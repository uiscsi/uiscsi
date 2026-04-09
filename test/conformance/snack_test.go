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

// TestSNACK_DataSNGap verifies that the initiator sends a Data/R2T SNACK
// (Type=0) with correct BegRun and RunLength when a DataSN gap is detected
// in the Data-In stream. The target skips DataSN=1, causing a gap. The
// initiator must SNACK for the missing PDU, then the target retransmits it.
// Conformance: SNACK-01 (FFP #13.1)
func TestSNACK_DataSNGap(t *testing.T) {
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

		// Send DataSN=0 (512 bytes at offset 0).
		din0 := &pdu.DataIn{
			Header: pdu.Header{
				Final:            false,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
				DataSegmentLen:   512,
			},
			DataSN:       0,
			BufferOffset: 0,
			ExpCmdSN:     expCmdSN,
			MaxCmdSN:     maxCmdSN,
			Data:         totalData[0:512],
		}
		if err := tc.SendPDU(din0); err != nil {
			return err
		}

		// SKIP DataSN=1 -- creating the gap.

		// Send DataSN=2 (512 bytes at offset 1024).
		din2 := &pdu.DataIn{
			Header: pdu.Header{
				Final:            false,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
				DataSegmentLen:   512,
			},
			DataSN:       2,
			BufferOffset: 1024,
			ExpCmdSN:     expCmdSN,
			MaxCmdSN:     maxCmdSN,
			Data:         totalData[1024:1536],
		}
		if err := tc.SendPDU(din2); err != nil {
			return err
		}

		// Wait for the SNACK to propagate from the initiator.
		time.Sleep(100 * time.Millisecond)

		// Retransmit DataSN=1 (512 bytes at offset 512) -- the gap fill.
		din1 := &pdu.DataIn{
			Header: pdu.Header{
				Final:            false,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
				DataSegmentLen:   512,
			},
			DataSN:       1,
			BufferOffset: 512,
			ExpCmdSN:     expCmdSN,
			MaxCmdSN:     maxCmdSN,
			Data:         totalData[512:1024],
		}
		if err := tc.SendPDU(din1); err != nil {
			return err
		}

		// Send final DataSN=3 with status (no data, but BufferOffset must
		// match the expected next offset for the initiator's validation).
		din3 := &pdu.DataIn{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
			},
			DataSN:       3,
			BufferOffset: 1536,
			HasStatus:    true,
			Status:       0x00,
			StatSN:       tc.NextStatSN(),
			ExpCmdSN:     expCmdSN,
			MaxCmdSN:     maxCmdSN,
		}
		if err := tc.SendPDU(din3); err != nil {
			return err
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
		t.Fatalf("ReadBlocks: %v", err)
	}
	if len(data) != 1536 {
		t.Fatalf("ReadBlocks returned %d bytes, want 1536", len(data))
	}

	// Allow time for SNACK to propagate.
	time.Sleep(100 * time.Millisecond)

	// Verify at least one Data/R2T SNACK (Type=0) was sent.
	snacks := rec.Sent(pdu.OpSNACKReq)
	if len(snacks) == 0 {
		t.Fatalf("no SNACK PDUs captured, want at least 1 Data/R2T SNACK")
	}

	found := false
	for _, cap := range snacks {
		snack := cap.Decoded.(*pdu.SNACKReq)
		if snack.Type == 0 { // Data/R2T SNACK
			found = true
			if snack.BegRun != 1 {
				t.Errorf("Data/R2T SNACK BegRun=%d, want 1 (the missing DataSN)", snack.BegRun)
			}
			if snack.RunLength != 1 {
				t.Errorf("Data/R2T SNACK RunLength=%d, want 1 (one missing PDU)", snack.RunLength)
			}
			break
		}
	}
	if !found {
		t.Errorf("no Data/R2T SNACK (Type=0) found among %d SNACK PDUs", len(snacks))
		for i, cap := range snacks {
			snack := cap.Decoded.(*pdu.SNACKReq)
			t.Logf("  SNACK[%d]: Type=%d BegRun=%d RunLength=%d", i, snack.Type, snack.BegRun, snack.RunLength)
		}
	}
}

// TestSNACK_DataACKWireFields verifies that the initiator sends a DataACK
// SNACK (Type=2) with correct wire fields when the target sets the A-bit
// (Acknowledge) on a Data-In PDU with a specific TargetTransferTag.
// This extends Phase 14 DATA-07 (TestDataIn_ABitDataACK) with deeper
// field assertions per D-03.
// Conformance: SNACK-02 (FFP #13.2)
func TestSNACK_DataACKWireFields(t *testing.T) {
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
		totalData[i] = byte(i % 251)
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
			// Set A-bit on 2nd PDU (DataSN=1) with specific TTT for assertion.
			if i == 1 {
				din.Acknowledge = true
				din.TargetTransferTag = 0x00000042
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
		t.Fatalf("ReadBlocks: %v", err)
	}
	if len(data) != 1536 {
		t.Fatalf("ReadBlocks returned %d bytes, want 1536", len(data))
	}

	// Allow time for SNACK to propagate.
	time.Sleep(100 * time.Millisecond)

	// Verify at least one DataACK SNACK (Type=2) was sent.
	snacks := rec.Sent(pdu.OpSNACKReq)
	if len(snacks) == 0 {
		t.Fatalf("no SNACK PDUs captured, want at least 1 DataACK SNACK")
	}

	found := false
	for _, cap := range snacks {
		snack := cap.Decoded.(*pdu.SNACKReq)
		if snack.Type == 2 { // DataACK
			found = true

			// Type field: must be exactly 2 (DataACK, not DataR2T or Status).
			if snack.Type != 2 {
				t.Errorf("DataACK SNACK Type=%d, want 2", snack.Type)
			}

			// BegRun field: DataSN following the A-bit PDU at DataSN=1.
			if snack.BegRun != 2 {
				t.Errorf("DataACK SNACK BegRun=%d, want 2", snack.BegRun)
			}

			// RunLength field: DataACK acknowledges all up to BegRun, per RFC 7143 Section 11.16.1.
			if snack.RunLength != 0 {
				t.Errorf("DataACK SNACK RunLength=%d, want 0", snack.RunLength)
			}

			// TargetTransferTag field: echoed from the A-bit Data-In PDU.
			if snack.TargetTransferTag != 0x00000042 {
				t.Errorf("DataACK SNACK TargetTransferTag=0x%08x, want 0x00000042", snack.TargetTransferTag)
			}

			// ITT field: must match the command's ITT (non-zero, valid task tag).
			if snack.InitiatorTaskTag == 0xFFFFFFFF {
				t.Errorf("DataACK SNACK InitiatorTaskTag=0xFFFFFFFF (reserved), want valid task tag")
			}

			break
		}
	}
	if !found {
		t.Errorf("no DataACK SNACK (Type=2) found among %d SNACK PDUs", len(snacks))
		for i, cap := range snacks {
			snack := cap.Decoded.(*pdu.SNACKReq)
			t.Logf("  SNACK[%d]: Type=%d BegRun=%d RunLength=%d TTT=0x%08x",
				i, snack.Type, snack.BegRun, snack.RunLength, snack.TargetTransferTag)
		}
	}
}
