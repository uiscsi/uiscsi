package conformance_test

import (
	"context"
	"testing"
	"time"

	"github.com/uiscsi/uiscsi"
	"github.com/uiscsi/uiscsi/internal/pdu"
	testutil "github.com/uiscsi/uiscsi/test"
	"github.com/uiscsi/uiscsi/test/pducapture"
)

// TestDataIn_StatusInFinal verifies that the final Data-In PDU in a multi-PDU
// read carries both the S-bit (HasStatus) and F-bit (Final), while intermediate
// PDUs carry neither. The initiator must accept status inline in the final
// Data-In without requiring a separate SCSI Response PDU.
// Conformance: DATA-06 (FFP #10.1)
func TestDataIn_StatusInFinal(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	// 1536 bytes total, 512 bytes per PDU = 3 Data-In PDUs.
	totalData := make([]byte, 1536)
	for i := range totalData {
		totalData[i] = byte(i % 251) // deterministic pattern
	}

	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)

		// Send 3 Data-In PDUs of 512 bytes each.
		for i := range 3 {
			offset := uint32(i * 512)
			isFinal := i == 2

			din := &pdu.DataIn{
				Header: pdu.Header{
					Final:            isFinal,
					InitiatorTaskTag: cmd.InitiatorTaskTag,
					DataSegmentLen:   512,
				},
				DataSN:       uint32(i),
				BufferOffset: offset,
				ExpCmdSN:     expCmdSN,
				MaxCmdSN:     maxCmdSN,
				Data:         totalData[offset : offset+512],
			}
			if isFinal {
				din.HasStatus = true
				din.Status = 0x00
				din.StatSN = tc.NextStatSN()
			}
			if err := tc.SendPDU(din); err != nil {
				return err
			}
		}
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithPDUHook(rec.Hook()),
		uiscsi.WithKeepaliveInterval(30*time.Second),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	data, err := sess.ReadBlocks(ctx, 0, 0, 3, 512)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(data) != 1536 {
		t.Fatalf("Read returned %d bytes, want 1536", len(data))
	}

	// Verify Data-In PDU fields from captured traffic.
	dins := rec.Received(pdu.OpDataIn)
	if len(dins) != 3 {
		t.Fatalf("captured Data-In PDUs: got %d, want 3", len(dins))
	}

	for i, cap := range dins {
		din := cap.Decoded.(*pdu.DataIn)
		isFinal := i == 2

		if din.Header.Final != isFinal {
			t.Errorf("DataIn[%d] Final=%v, want %v", i, din.Header.Final, isFinal)
		}
		if din.HasStatus != isFinal {
			t.Errorf("DataIn[%d] HasStatus=%v, want %v", i, din.HasStatus, isFinal)
		}
		if isFinal && din.Status != 0x00 {
			t.Errorf("DataIn[%d] Status=0x%02x, want 0x00", i, din.Status)
		}
	}
}

// TestDataIn_ABitDataACK verifies that when the target sets the A-bit
// (Acknowledge) on a Data-In PDU, the initiator responds with a DataACK
// SNACK (Type=2) at ERL >= 1. Per RFC 7143 Section 11.7.2.
// Conformance: DATA-07 (FFP #10.2)
func TestDataIn_ABitDataACK(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	// Configure ERL=1 on target side.
	tgt.SetNegotiationConfig(testutil.NegotiationConfig{
		ErrorRecoveryLevel: testutil.Uint32Ptr(1),
	})
	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	totalData := make([]byte, 1536)
	for i := range totalData {
		totalData[i] = byte(i % 197)
	}

	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)

		for i := range 3 {
			offset := uint32(i * 512)
			isFinal := i == 2

			din := &pdu.DataIn{
				Header: pdu.Header{
					Final:            isFinal,
					InitiatorTaskTag: cmd.InitiatorTaskTag,
					DataSegmentLen:   512,
				},
				DataSN:       uint32(i),
				BufferOffset: offset,
				ExpCmdSN:     expCmdSN,
				MaxCmdSN:     maxCmdSN,
				Data:         totalData[offset : offset+512],
			}
			// Set A-bit on 2nd PDU (DataSN=1).
			if i == 1 {
				din.Acknowledge = true
				din.TargetTransferTag = 0x00000001
			}
			if isFinal {
				din.HasStatus = true
				din.Status = 0x00
				din.StatSN = tc.NextStatSN()
			}
			if err := tc.SendPDU(din); err != nil {
				return err
			}
		}
		return nil
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

	data, err := sess.ReadBlocks(ctx, 0, 0, 3, 512)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(data) != 1536 {
		t.Fatalf("Read returned %d bytes, want 1536", len(data))
	}

	// Allow time for SNACK to propagate.
	time.Sleep(100 * time.Millisecond)

	// Verify at least one SNACK was sent with Type=DataACK (2).
	snacks := rec.Sent(pdu.OpSNACKReq)
	if len(snacks) == 0 {
		t.Fatalf("no SNACK PDUs captured, want at least 1 DataACK SNACK")
	}

	found := false
	for _, cap := range snacks {
		snack := cap.Decoded.(*pdu.SNACKReq)
		if snack.Type == 2 { // DataACK
			found = true
			// BegRun should be DataSN following the A-bit PDU (DataSN=1),
			// so BegRun should be 2.
			if snack.BegRun != 2 {
				t.Errorf("DataACK SNACK BegRun=%d, want 2", snack.BegRun)
			}
			break
		}
	}
	if !found {
		t.Errorf("no DataACK SNACK (Type=2) found among %d SNACK PDUs", len(snacks))
		for i, cap := range snacks {
			snack := cap.Decoded.(*pdu.SNACKReq)
			t.Logf("  SNACK[%d]: Type=%d BegRun=%d RunLength=%d", i, snack.Type, snack.BegRun, snack.RunLength)
		}
	}
}

// TestDataIn_ZeroLength verifies that the initiator correctly handles a
// Data-In PDU with DataSegmentLength=0 (empty data segment). This can occur
// when the final Data-In carries only status with no data payload.
// Conformance: DATA-09 (FFP #11.1.2)
func TestDataIn_ZeroLength(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	readData := make([]byte, 512)
	for i := range readData {
		readData[i] = byte(i % 127)
	}

	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)

		// First PDU: 512 bytes of data, not final.
		din1 := &pdu.DataIn{
			Header: pdu.Header{
				Final:            false,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
				DataSegmentLen:   512,
			},
			DataSN:       0,
			BufferOffset: 0,
			ExpCmdSN:     expCmdSN,
			MaxCmdSN:     maxCmdSN,
			Data:         readData,
		}
		if err := tc.SendPDU(din1); err != nil {
			return err
		}

		// Second PDU: zero-length data, Final+Status.
		din2 := &pdu.DataIn{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
				DataSegmentLen:   0,
			},
			DataSN:       1,
			BufferOffset: 512,
			HasStatus:    true,
			Status:       0x00,
			StatSN:       tc.NextStatSN(),
			ExpCmdSN:     expCmdSN,
			MaxCmdSN:     maxCmdSN,
			Data:         nil,
		}
		return tc.SendPDU(din2)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithPDUHook(rec.Hook()),
		uiscsi.WithKeepaliveInterval(30*time.Second),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	data, err := sess.ReadBlocks(ctx, 0, 0, 1, 512)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(data) != 512 {
		t.Fatalf("Read returned %d bytes, want 512", len(data))
	}

	// Verify 2 Data-In PDUs were received (one with data, one zero-length).
	dins := rec.Received(pdu.OpDataIn)
	if len(dins) != 2 {
		t.Fatalf("captured Data-In PDUs: got %d, want 2", len(dins))
	}

	// Second PDU should have zero-length data segment on the wire.
	lastDin := dins[1].Decoded.(*pdu.DataIn)
	if lastDin.Header.DataSegmentLen != 0 {
		t.Errorf("final Data-In DataSegmentLen=%d, want 0", lastDin.Header.DataSegmentLen)
	}
	if !lastDin.HasStatus {
		t.Errorf("final Data-In HasStatus=false, want true")
	}
}

// TestDataIn_EDTL verifies that the sum of all Data-In DataSegmentLength
// values matches the Expected Data Transfer Length (EDTL) from the SCSI
// Command, and that Read() returns exactly EDTL bytes.
// Conformance: DATA-14 (FFP #16.6)
func TestDataIn_EDTL(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	// 1024 bytes total, 256 bytes per PDU = 4 Data-In PDUs.
	totalData := make([]byte, 1024)
	for i := range totalData {
		totalData[i] = byte(i % 179)
	}

	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)

		for i := range 4 {
			offset := uint32(i * 256)
			isFinal := i == 3

			din := &pdu.DataIn{
				Header: pdu.Header{
					Final:            isFinal,
					InitiatorTaskTag: cmd.InitiatorTaskTag,
					DataSegmentLen:   256,
				},
				DataSN:       uint32(i),
				BufferOffset: offset,
				ExpCmdSN:     expCmdSN,
				MaxCmdSN:     maxCmdSN,
				Data:         totalData[offset : offset+256],
			}
			if isFinal {
				din.HasStatus = true
				din.Status = 0x00
				din.StatSN = tc.NextStatSN()
			}
			if err := tc.SendPDU(din); err != nil {
				return err
			}
		}
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithPDUHook(rec.Hook()),
		uiscsi.WithKeepaliveInterval(30*time.Second),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	data, err := sess.ReadBlocks(ctx, 0, 0, 2, 512)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(data) != 1024 {
		t.Fatalf("Read returned %d bytes, want 1024", len(data))
	}

	// Verify EDTL from the SCSI Command PDU.
	cmds := rec.Sent(pdu.OpSCSICommand)
	if len(cmds) == 0 {
		t.Fatal("no SCSI Command PDUs captured")
	}
	scsiCmd := cmds[0].Decoded.(*pdu.SCSICommand)
	edtl := scsiCmd.ExpectedDataTransferLength

	// Sum all Data-In data segment lengths.
	dins := rec.Received(pdu.OpDataIn)
	if len(dins) != 4 {
		t.Fatalf("captured Data-In PDUs: got %d, want 4", len(dins))
	}

	var totalRecv uint32
	for _, cap := range dins {
		din := cap.Decoded.(*pdu.DataIn)
		totalRecv += din.Header.DataSegmentLen
	}

	if totalRecv != edtl {
		t.Errorf("sum of Data-In DataSegmentLen=%d, want EDTL=%d", totalRecv, edtl)
	}
}
