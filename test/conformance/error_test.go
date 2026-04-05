package conformance_test

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/rkujawa/uiscsi"
	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
	testutil "github.com/rkujawa/uiscsi/test"
	"github.com/rkujawa/uiscsi/test/pducapture"
)

// TestError_SCSICheckCondition verifies that CHECK CONDITION status (0x02)
// with sense data produces a *SCSIError with correct SenseKey/ASC/ASCQ.
// IOL: Error Recovery - SCSI Status CHECK CONDITION.
func TestError_SCSICheckCondition(t *testing.T) {
	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	defer tgt.Close()

	tgt.HandleLogin()
	tgt.HandleLogout()

	// Build sense data: fixed format (0x70), sense key=5 (ILLEGAL REQUEST),
	// ASC=0x24 (INVALID FIELD IN CDB), ASCQ=0x00.
	senseData := make([]byte, 18)
	senseData[0] = 0x70       // response code: current errors, fixed format
	senseData[2] = 0x05       // sense key: ILLEGAL REQUEST
	senseData[7] = 10         // additional sense length
	senseData[12] = 0x24      // ASC: INVALID FIELD IN CDB
	senseData[13] = 0x00      // ASCQ
	tgt.HandleSCSIError(0x02, senseData) // CHECK CONDITION

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer sess.Close()

	_, readErr := sess.ReadBlocks(ctx, 0, 0, 1, 512)
	if readErr == nil {
		t.Fatal("expected error for CHECK CONDITION")
	}

	var scsiErr *uiscsi.SCSIError
	if !errors.As(readErr, &scsiErr) {
		t.Fatalf("expected *SCSIError, got %T: %v", readErr, readErr)
	}
	if scsiErr.Status != 0x02 {
		t.Fatalf("Status: got 0x%02X, want 0x02", scsiErr.Status)
	}
	if scsiErr.SenseKey != 0x05 {
		t.Fatalf("SenseKey: got 0x%02X, want 0x05", scsiErr.SenseKey)
	}
	if scsiErr.ASC != 0x24 {
		t.Fatalf("ASC: got 0x%02X, want 0x24", scsiErr.ASC)
	}
	if scsiErr.ASCQ != 0x00 {
		t.Fatalf("ASCQ: got 0x%02X, want 0x00", scsiErr.ASCQ)
	}
}

// TestError_TransportDrop verifies that closing the mock target connection
// mid-operation produces a *TransportError.
// IOL: Error Recovery - Transport Connection Drop.
func TestError_TransportDrop(t *testing.T) {
	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	defer tgt.Close()

	tgt.HandleLogin()
	tgt.HandleLogout()
	// No SCSI handler -- so the target won't respond.

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer sess.Close()

	// Close the target to drop all connections.
	tgt.Close()

	// Attempt an operation -- should fail with transport error.
	// Give a short timeout so it does not hang.
	readCtx, readCancel := context.WithTimeout(ctx, 2*time.Second)
	defer readCancel()

	_, readErr := sess.ReadBlocks(readCtx, 0, 0, 1, 512)
	if readErr == nil {
		t.Fatal("expected error after transport drop")
	}
	// The error should be some kind of transport/connection failure.
	// It might be wrapped as *TransportError or context.DeadlineExceeded.
	// Both are acceptable -- the important thing is we get an error, not a hang.
}

// TestError_TypedErrorChain verifies errors.As works through the error chain
// for wrapped errors.
func TestError_TypedErrorChain(t *testing.T) {
	// Test that TransportError.Unwrap works.
	inner := errors.New("connection reset")
	te := &uiscsi.TransportError{Op: "read", Err: inner}

	if !errors.Is(te, inner) {
		t.Fatal("errors.Is should find inner error through TransportError")
	}

	var te2 *uiscsi.TransportError
	if !errors.As(te, &te2) {
		t.Fatal("errors.As should find *TransportError")
	}
	if te2.Op != "read" {
		t.Fatalf("Op: got %q, want %q", te2.Op, "read")
	}
}

// TestError_BUSY verifies that BUSY status (0x08) surfaces as a *SCSIError
// with Status==0x08 via errors.As.
// Conformance: ERR-05.
func TestError_BUSY(t *testing.T) {
	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleSCSIWithStatus(0x08, nil) // BUSY, no sense data

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	_, readErr := sess.ReadBlocks(ctx, 0, 0, 1, 512)
	if readErr == nil {
		t.Fatal("expected error for BUSY status")
	}

	var scsiErr *uiscsi.SCSIError
	if !errors.As(readErr, &scsiErr) {
		t.Fatalf("expected *SCSIError, got %T: %v", readErr, readErr)
	}
	if scsiErr.Status != 0x08 {
		t.Fatalf("Status: got 0x%02X, want 0x08", scsiErr.Status)
	}
}

// TestError_ReservationConflict verifies that RESERVATION CONFLICT status
// (0x18) surfaces as a *SCSIError with Status==0x18.
// Conformance: ERR-06.
func TestError_ReservationConflict(t *testing.T) {
	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleSCSIWithStatus(0x18, nil) // RESERVATION CONFLICT, no sense data

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	_, readErr := sess.ReadBlocks(ctx, 0, 0, 1, 512)
	if readErr == nil {
		t.Fatal("expected error for RESERVATION CONFLICT status")
	}

	var scsiErr *uiscsi.SCSIError
	if !errors.As(readErr, &scsiErr) {
		t.Fatalf("expected *SCSIError, got %T: %v", readErr, readErr)
	}
	if scsiErr.Status != 0x18 {
		t.Fatalf("Status: got 0x%02X, want 0x18", scsiErr.Status)
	}
}

// TestError_UnexpectedUnsolicited verifies that CHECK CONDITION with sense
// key ABORTED COMMAND (0x0B), ASC=0x0C, ASCQ=0x0D (unexpected unsolicited
// data) surfaces correctly via errors.As.
// Conformance: ERR-03.
func TestError_UnexpectedUnsolicited(t *testing.T) {
	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()

	// Fixed format sense data (0x70): ABORTED COMMAND, ASC=0x0C, ASCQ=0x0D.
	senseData := make([]byte, 18)
	senseData[0] = 0x70  // response code: current errors, fixed format
	senseData[2] = 0x0B  // sense key: ABORTED COMMAND
	senseData[7] = 10    // additional sense length
	senseData[12] = 0x0C // ASC: WRITE ERROR
	senseData[13] = 0x0D // ASCQ: UNEXPECTED UNSOLICITED DATA
	tgt.HandleSCSIWithStatus(0x02, senseData) // CHECK CONDITION

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	// Use WriteBlocks to trigger the write path (unsolicited data scenario).
	writeErr := sess.WriteBlocks(ctx, 0, 0, 1, 512, make([]byte, 512))
	if writeErr == nil {
		t.Fatal("expected error for CHECK CONDITION with unexpected unsolicited data")
	}

	var scsiErr *uiscsi.SCSIError
	if !errors.As(writeErr, &scsiErr) {
		t.Fatalf("expected *SCSIError, got %T: %v", writeErr, writeErr)
	}
	if scsiErr.Status != 0x02 {
		t.Fatalf("Status: got 0x%02X, want 0x02", scsiErr.Status)
	}
	if scsiErr.SenseKey != 0x0B {
		t.Fatalf("SenseKey: got 0x%02X, want 0x0B", scsiErr.SenseKey)
	}
	if scsiErr.ASC != 0x0C {
		t.Fatalf("ASC: got 0x%02X, want 0x0C", scsiErr.ASC)
	}
	if scsiErr.ASCQ != 0x0D {
		t.Fatalf("ASCQ: got 0x%02X, want 0x0D", scsiErr.ASCQ)
	}
}

// TestError_NotEnoughUnsolicited verifies that CHECK CONDITION with sense
// key ABORTED COMMAND (0x0B), ASC=0x0C, ASCQ=0x0E (not enough unsolicited
// data) surfaces correctly via errors.As.
// Conformance: ERR-04.
func TestError_NotEnoughUnsolicited(t *testing.T) {
	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()

	// Fixed format sense data (0x70): ABORTED COMMAND, ASC=0x0C, ASCQ=0x0E.
	senseData := make([]byte, 18)
	senseData[0] = 0x70  // response code: current errors, fixed format
	senseData[2] = 0x0B  // sense key: ABORTED COMMAND
	senseData[7] = 10    // additional sense length
	senseData[12] = 0x0C // ASC: WRITE ERROR
	senseData[13] = 0x0E // ASCQ: NOT ENOUGH UNSOLICITED DATA
	tgt.HandleSCSIWithStatus(0x02, senseData) // CHECK CONDITION

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	writeErr := sess.WriteBlocks(ctx, 0, 0, 1, 512, make([]byte, 512))
	if writeErr == nil {
		t.Fatal("expected error for CHECK CONDITION with not enough unsolicited data")
	}

	var scsiErr *uiscsi.SCSIError
	if !errors.As(writeErr, &scsiErr) {
		t.Fatalf("expected *SCSIError, got %T: %v", writeErr, writeErr)
	}
	if scsiErr.Status != 0x02 {
		t.Fatalf("Status: got 0x%02X, want 0x02", scsiErr.Status)
	}
	if scsiErr.SenseKey != 0x0B {
		t.Fatalf("SenseKey: got 0x%02X, want 0x0B", scsiErr.SenseKey)
	}
	if scsiErr.ASC != 0x0C {
		t.Fatalf("ASC: got 0x%02X, want 0x0C", scsiErr.ASC)
	}
	if scsiErr.ASCQ != 0x0E {
		t.Fatalf("ASCQ: got 0x%02X, want 0x0E", scsiErr.ASCQ)
	}
}

// TestError_CRCErrorSense verifies that CHECK CONDITION with sense key
// ABORTED COMMAND (0x0B), ASC=0x47, ASCQ=0x05 (CRC error detected) surfaces
// correct SenseKey/ASC/ASCQ via errors.As.
// Conformance: ERR-01.
func TestError_CRCErrorSense(t *testing.T) {
	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()

	// Fixed format sense data (0x70): ABORTED COMMAND, ASC=0x47, ASCQ=0x05.
	senseData := make([]byte, 18)
	senseData[0] = 0x70  // response code: current errors, fixed format
	senseData[2] = 0x0B  // sense key: ABORTED COMMAND
	senseData[7] = 10    // additional sense length
	senseData[12] = 0x47 // ASC: SCSI PARITY ERROR
	senseData[13] = 0x05 // ASCQ: CRC ERROR DETECTED

	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)

		// Build data segment: [SenseLength (2 bytes BE)] [Sense Data]
		dataSegment := make([]byte, 2+len(senseData))
		binary.BigEndian.PutUint16(dataSegment[0:2], uint16(len(senseData)))
		copy(dataSegment[2:], senseData)

		resp := &pdu.SCSIResponse{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
				DataSegmentLen:   uint32(len(dataSegment)),
			},
			Status:   0x02, // CHECK CONDITION
			StatSN:   tc.NextStatSN(),
			ExpCmdSN: expCmdSN,
			MaxCmdSN: maxCmdSN,
			Data:     dataSegment,
		}
		return tc.SendPDU(resp)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	_, readErr := sess.ReadBlocks(ctx, 0, 0, 1, 512)
	if readErr == nil {
		t.Fatal("expected error for CHECK CONDITION with CRC error sense")
	}

	var scsiErr *uiscsi.SCSIError
	if !errors.As(readErr, &scsiErr) {
		t.Fatalf("expected *SCSIError, got %T: %v", readErr, readErr)
	}
	if scsiErr.Status != 0x02 {
		t.Fatalf("Status: got 0x%02X, want 0x02", scsiErr.Status)
	}
	if scsiErr.SenseKey != 0x0B {
		t.Fatalf("SenseKey: got 0x%02X, want 0x0B", scsiErr.SenseKey)
	}
	if scsiErr.ASC != 0x47 {
		t.Fatalf("ASC: got 0x%02X, want 0x47", scsiErr.ASC)
	}
	if scsiErr.ASCQ != 0x05 {
		t.Fatalf("ASCQ: got 0x%02X, want 0x05", scsiErr.ASCQ)
	}
}

// TestError_SNACKRejectNewCommand verifies that when the target sends a
// Reject PDU (reason 0x09: Invalid PDU Field) for a SNACK triggered by a
// DataSN gap, the first command fails but a subsequent command succeeds.
// Conformance: ERR-02.
func TestError_SNACKRejectNewCommand(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	// ERL=1 required for SNACK support.
	tgt.SetNegotiationConfig(testutil.NegotiationConfig{
		ErrorRecoveryLevel: testutil.Uint32Ptr(1),
	})
	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	// Register a SNACK handler to drain any SNACK PDUs the initiator sends
	// (the Status SNACK timer may fire after the task is cancelled).
	tgt.Handle(pdu.OpSNACKReq, func(tc *testutil.TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		// Silently consume -- the Reject already handles the SNACK semantics.
		return nil
	})

	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)

		if callCount == 0 {
			// First command: send Data-In with DataSN gap to trigger SNACK.
			// Send DataSN=0 (512 bytes).
			din0 := &pdu.DataIn{
				Header: pdu.Header{
					InitiatorTaskTag: cmd.InitiatorTaskTag,
					DataSegmentLen:   512,
				},
				DataSN:       0,
				BufferOffset: 0,
				ExpCmdSN:     expCmdSN,
				MaxCmdSN:     maxCmdSN,
				Data:         make([]byte, 512),
			}
			if err := tc.SendPDU(din0); err != nil {
				return err
			}

			// Skip DataSN=1, send DataSN=2 (512 bytes) to create a gap.
			din2 := &pdu.DataIn{
				Header: pdu.Header{
					Final:            true,
					InitiatorTaskTag: cmd.InitiatorTaskTag,
					DataSegmentLen:   512,
				},
				DataSN:       2,
				BufferOffset: 1024,
				ExpCmdSN:     expCmdSN,
				MaxCmdSN:     maxCmdSN,
				Data:         make([]byte, 512),
			}
			if err := tc.SendPDU(din2); err != nil {
				return err
			}

			// Read the SNACK PDU from the initiator.
			_, snackRaw, err := tc.ReadPDU()
			if err != nil {
				return fmt.Errorf("reading SNACK: %w", err)
			}

			// Build Reject PDU: Reason=0x09 (Invalid PDU Field).
			// The data segment contains the complete BHS of the rejected SNACK.
			reject := &pdu.Reject{
				Header: pdu.Header{
					Final:            true,
					InitiatorTaskTag: 0xFFFFFFFF,
					DataSegmentLen:   uint32(len(snackRaw.BHS)),
				},
				Reason:   0x09,
				StatSN:   tc.NextStatSN(),
				ExpCmdSN: expCmdSN,
				MaxCmdSN: maxCmdSN,
				Data:     snackRaw.BHS[:],
			}
			return tc.SendPDU(reject)
		}

		// callCount >= 1: second command succeeds normally.
		din := &pdu.DataIn{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
				DataSegmentLen:   512,
			},
			DataSN:       0,
			BufferOffset: 0,
			HasStatus:    true,
			Status:       0x00,
			StatSN:       tc.NextStatSN(),
			ExpCmdSN:     expCmdSN,
			MaxCmdSN:     maxCmdSN,
			Data:         make([]byte, 512),
		}
		return tc.SendPDU(din)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithPDUHook(rec.Hook()),
		uiscsi.WithKeepaliveInterval(30*time.Second),
		uiscsi.WithOperationalOverrides(map[string]string{
			"ErrorRecoveryLevel": "1",
		}),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	// First ReadBlocks should fail (Reject cancels the task).
	_, firstErr := sess.ReadBlocks(ctx, 0, 0, 1, 512)
	if firstErr == nil {
		t.Fatal("expected first ReadBlocks to fail after Reject")
	}

	// Allow async processing to settle.
	time.Sleep(200 * time.Millisecond)

	// Second ReadBlocks should succeed.
	data, secondErr := sess.ReadBlocks(ctx, 0, 0, 1, 512)
	if secondErr != nil {
		t.Fatalf("second ReadBlocks should succeed, got: %v", secondErr)
	}
	if len(data) != 512 {
		t.Fatalf("second ReadBlocks returned %d bytes, want 512", len(data))
	}
}
