package conformance_test

import (
	"context"
	"testing"
	"time"

	"github.com/uiscsi/uiscsi"
	"github.com/uiscsi/uiscsi/internal/pdu"
	"github.com/uiscsi/uiscsi/internal/transport"
	testutil "github.com/uiscsi/uiscsi/test"
	"github.com/uiscsi/uiscsi/test/pducapture"
)

// TestRetry_RejectCallerReissue tests the initiator's behavior after receiving
// a Reject PDU at ERL=0 (caller-reissue path).
//
// At ERL=0, the initiator does not perform same-connection retry. A Reject
// cancels the in-flight task; the caller receives an error and must re-issue
// a new command with new ITT and CmdSN.
//
// This test verifies: Reject -> task cancelled -> caller re-issues ->
// new command succeeds with new ITT/CmdSN but same CDB (READ(10) for same LBA).
func TestRetry_RejectCallerReissue(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	// ERL=0: no same-connection retry, Reject cancels task.
	tgt.SetNegotiationConfig(testutil.NegotiationConfig{
		ErrorRecoveryLevel: testutil.Uint32Ptr(0),
	})
	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)

		if callCount == 0 {
			// First command: send a Reject PDU with Reason=0x09 (Invalid PDU Field).
			// The data segment contains the complete BHS of the rejected command.
			rejectedBHS, err := cmd.MarshalBHS()
			if err != nil {
				return err
			}

			reject := &pdu.Reject{
				Header: pdu.Header{
					Final:            true,
					InitiatorTaskTag: 0xFFFFFFFF,
					DataSegmentLen:   uint32(len(rejectedBHS)),
				},
				Reason:   0x09, // Invalid PDU Field
				StatSN:   tc.NextStatSN(),
				ExpCmdSN: expCmdSN,
				MaxCmdSN: maxCmdSN,
				Data:     rejectedBHS[:],
			}
			return tc.SendPDU(reject)
		}

		// callCount >= 1: respond normally with Data-In (HasStatus=true).
		data := make([]byte, 512)
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
			Data:         data,
		}
		return tc.SendPDU(din)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithPDUHook(rec.Hook()),
		uiscsi.WithKeepaliveInterval(30*time.Second),
		uiscsi.WithOperationalOverrides(map[string]string{
			"ErrorRecoveryLevel": "0",
		}),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	// First ReadBlocks should fail (Reject cancels the task at ERL=0).
	_, firstErr := sess.ReadBlocks(ctx, 0, 0, 1, 512)
	if firstErr == nil {
		t.Fatal("expected first ReadBlocks to fail after Reject at ERL=0")
	}
	t.Logf("first ReadBlocks error (expected): %v", firstErr)

	// Allow async processing to settle.
	time.Sleep(200 * time.Millisecond)

	// Second ReadBlocks should succeed (caller re-issues with new ITT/CmdSN).
	data, secondErr := sess.ReadBlocks(ctx, 0, 0, 1, 512)
	if secondErr != nil {
		t.Fatalf("second ReadBlocks should succeed, got: %v", secondErr)
	}
	if len(data) != 512 {
		t.Fatalf("second ReadBlocks returned %d bytes, want 512", len(data))
	}

	// Verify pducapture: at least 2 SCSI Command PDUs were sent.
	cmds := rec.Sent(pdu.OpSCSICommand)
	if len(cmds) < 2 {
		t.Fatalf("captured SCSI commands: got %d, want >= 2", len(cmds))
	}

	first := cmds[0].Decoded.(*pdu.SCSICommand)
	second := cmds[1].Decoded.(*pdu.SCSICommand)

	// Assert CmdSN[1] > CmdSN[0] (NEW CmdSN, not original).
	if second.CmdSN <= first.CmdSN {
		t.Fatalf("CmdSN not incremented: first=%d, second=%d (want second > first)",
			first.CmdSN, second.CmdSN)
	}
	t.Logf("CmdSN: first=%d, second=%d (incremented as expected)", first.CmdSN, second.CmdSN)

	// Assert ITT[1] != ITT[0] (NEW ITT, not original).
	if second.InitiatorTaskTag == first.InitiatorTaskTag {
		t.Fatalf("ITT not changed: first=0x%08X, second=0x%08X (want different)",
			first.InitiatorTaskTag, second.InitiatorTaskTag)
	}
	t.Logf("ITT: first=0x%08X, second=0x%08X (different as expected)",
		first.InitiatorTaskTag, second.InitiatorTaskTag)

	// Assert both have the same CDB bytes (both are READ(10) for LBA 0, 1 block).
	if len(first.CDB) != len(second.CDB) {
		t.Fatalf("CDB length mismatch: first=%d, second=%d", len(first.CDB), len(second.CDB))
	}
	for i := range first.CDB {
		if first.CDB[i] != second.CDB[i] {
			t.Fatalf("CDB byte %d differs: first=0x%02X, second=0x%02X",
				i, first.CDB[i], second.CDB[i])
		}
	}
	t.Logf("CDB: identical (both READ(10) for same LBA/block count)")
}

// TestRetry_SameConnectionRetry tests same-connection retry after Reject at
// ERL=1 (CMDSEQ-07 / FFP #4.1).
//
// Per RFC 7143 Section 6.2.1, when the target rejects a command at ERL>=1,
// the initiator MUST retry on the same connection with the original ITT,
// CDB, and CmdSN. This test verifies all three fields are identical on
// the retried command.
func TestRetry_SameConnectionRetry(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	// ERL=1 required for same-connection retry.
	tgt.SetNegotiationConfig(testutil.NegotiationConfig{
		ErrorRecoveryLevel: testutil.Uint32Ptr(1),
	})
	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	// Register a SNACK handler to drain any SNACK PDUs.
	tgt.Handle(pdu.OpSNACKReq, func(tc *testutil.TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		return nil // silently consume
	})

	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)

		if callCount == 0 {
			// First command: send a Reject PDU (Reason=0x09).
			rejectedBHS, err := cmd.MarshalBHS()
			if err != nil {
				return err
			}
			reject := &pdu.Reject{
				Header: pdu.Header{
					Final:            true,
					InitiatorTaskTag: 0xFFFFFFFF,
					DataSegmentLen:   uint32(len(rejectedBHS)),
				},
				Reason:   0x09,
				StatSN:   tc.NextStatSN(),
				ExpCmdSN: expCmdSN,
				MaxCmdSN: maxCmdSN,
				Data:     rejectedBHS[:],
			}
			return tc.SendPDU(reject)
		}

		// callCount >= 1: respond normally with Data-In (HasStatus=true).
		data := make([]byte, 512)
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
			Data:         data,
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

	// ReadBlocks should succeed — the initiator retries internally after Reject.
	data, err := sess.ReadBlocks(ctx, 0, 0, 1, 512)
	if err != nil {
		t.Fatalf("ReadBlocks should succeed after same-connection retry, got: %v", err)
	}
	if len(data) != 512 {
		t.Fatalf("ReadBlocks returned %d bytes, want 512", len(data))
	}

	// Verify pducapture: exactly 2 SCSI Command PDUs were sent.
	cmds := rec.Sent(pdu.OpSCSICommand)
	if len(cmds) < 2 {
		t.Fatalf("captured SCSI commands: got %d, want >= 2", len(cmds))
	}

	first := cmds[0].Decoded.(*pdu.SCSICommand)
	second := cmds[1].Decoded.(*pdu.SCSICommand)

	// Assert ITT[1] == ITT[0] (SAME ITT — same-connection retry).
	if second.InitiatorTaskTag != first.InitiatorTaskTag {
		t.Fatalf("ITT changed on retry: first=0x%08X, second=0x%08X (want same)",
			first.InitiatorTaskTag, second.InitiatorTaskTag)
	}
	t.Logf("ITT: first=0x%08X, second=0x%08X (same -- correct)", first.InitiatorTaskTag, second.InitiatorTaskTag)

	// Assert CmdSN[1] == CmdSN[0] (SAME CmdSN — same-connection retry).
	if second.CmdSN != first.CmdSN {
		t.Fatalf("CmdSN changed on retry: first=%d, second=%d (want same)",
			first.CmdSN, second.CmdSN)
	}
	t.Logf("CmdSN: first=%d, second=%d (same -- correct)", first.CmdSN, second.CmdSN)

	// Assert CDB identical (same READ(10) command).
	if len(first.CDB) != len(second.CDB) {
		t.Fatalf("CDB length mismatch: first=%d, second=%d", len(first.CDB), len(second.CDB))
	}
	for i := range first.CDB {
		if first.CDB[i] != second.CDB[i] {
			t.Fatalf("CDB byte %d differs: first=0x%02X, second=0x%02X", i, first.CDB[i], second.CDB[i])
		}
	}
	t.Logf("CDB: identical (both READ(10) for same LBA/block count)")
}

// TestRetry_ExpStatSNGap tests the initiator's behavior when the target
// sends a SCSI response with a jumped StatSN, followed by a tail loss
// scenario where a command receives partial Data-In but never the final
// response (CMDSEQ-08 / FFP #5.1).
//
// Strategy: The first command gets a normal response with StatSN jumped
// by 5 (creating a gap in ExpStatSN tracking). The second command receives
// one partial Data-In (no status) to start the SNACK timer, then never
// receives a final response. The SNACK timer fires after the configured
// timeout, sending a Status SNACK (Type=1) to request the missing status.
//
// The test asserts that a Status SNACK PDU is sent on the wire. If not,
// it fails explicitly documenting the gap in ERL=1 status recovery.
func TestRetry_ExpStatSNGap(t *testing.T) {
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

	// Register SNACK handler that consumes all SNACK PDUs silently.
	// The test verifies SNACK presence via pducapture, not handler logic.
	tgt.Handle(pdu.OpSNACKReq, func(tc *testutil.TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		snack := decoded.(*pdu.SNACKReq)
		t.Logf("SNACK received: Type=%d, BegRun=%d, RunLength=%d",
			snack.Type, snack.BegRun, snack.RunLength)
		return nil
	})

	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)

		if callCount == 0 {
			// First command: advance StatSN by 5 extra calls to create a gap.
			// The initiator's updateStatSN will jump to the new value.
			for range 5 {
				tc.NextStatSN() // skip 5 StatSN values
			}

			// Send normal response with the jumped StatSN.
			data := make([]byte, 512)
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
				StatSN:       tc.NextStatSN(), // this is now 5 ahead
				ExpCmdSN:     expCmdSN,
				MaxCmdSN:     maxCmdSN,
				Data:         data,
			}
			return tc.SendPDU(din)
		}

		if callCount == 1 {
			// Second command: send one partial Data-In (no status) to start
			// the SNACK timer, then do NOT respond further. This creates a
			// tail loss scenario where the SNACK timer fires.
			partialData := make([]byte, 256)
			din := &pdu.DataIn{
				Header: pdu.Header{
					InitiatorTaskTag: cmd.InitiatorTaskTag,
					DataSegmentLen:   256,
				},
				DataSN:       0,
				BufferOffset: 0,
				ExpCmdSN:     expCmdSN,
				MaxCmdSN:     maxCmdSN,
				Data:         partialData,
			}
			// Send partial Data-In, then return without sending final.
			// The SNACK timer will fire after snackTimeout.
			return tc.SendPDU(din)
		}

		// callCount >= 2: respond normally (cleanup).
		data := make([]byte, 512)
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
			Data:         data,
		}
		return tc.SendPDU(din)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithPDUHook(rec.Hook()),
		uiscsi.WithKeepaliveInterval(30*time.Second),
		uiscsi.WithSNACKTimeout(500*time.Millisecond), // short timeout for test speed
		uiscsi.WithOperationalOverrides(map[string]string{
			"ErrorRecoveryLevel": "1",
		}),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	// First ReadBlocks: should succeed despite jumped StatSN.
	data, err := sess.ReadBlocks(ctx, 0, 0, 1, 512)
	if err != nil {
		t.Fatalf("first ReadBlocks: %v", err)
	}
	if len(data) != 512 {
		t.Fatalf("first ReadBlocks returned %d bytes, want 512", len(data))
	}
	t.Log("first ReadBlocks succeeded (StatSN jumped by 5)")

	// Second ReadBlocks in a goroutine with short timeout. This will hang
	// (only partial Data-In, no final response), triggering the SNACK timer.
	readCtx, readCancel := context.WithTimeout(ctx, 5*time.Second)
	defer readCancel()

	doneCh := make(chan error, 1)
	go func() {
		_, err := sess.ReadBlocks(readCtx, 0, 0, 1, 512)
		doneCh <- err
	}()

	// Wait for the SNACK timer to fire. With 500ms timeout, wait up to 3s.
	time.Sleep(2 * time.Second)

	// Check pducapture for Status SNACK PDUs.
	snacks := rec.Sent(pdu.OpSNACKReq)
	var statusSNACKFound bool
	for _, s := range snacks {
		snack := s.Decoded.(*pdu.SNACKReq)
		if snack.Type == 1 { // Status SNACK
			statusSNACKFound = true
			t.Logf("Status SNACK found: Type=%d, BegRun=%d, RunLength=%d, ExpStatSN=%d",
				snack.Type, snack.BegRun, snack.RunLength, snack.ExpStatSN)
			break
		}
	}

	if !statusSNACKFound {
		t.Fatal("CMDSEQ-08: no Status SNACK sent after StatSN gap -- " +
			"ERL=1 status recovery via SNACK timer is not functioning for tail loss scenario")
	}

	// Clean up: cancel the second ReadBlocks.
	readCancel()
	<-doneCh
}
