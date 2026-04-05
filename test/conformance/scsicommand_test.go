package conformance_test

import (
	"context"
	"testing"
	"time"

	"github.com/rkujawa/uiscsi/internal/pdu"
	testutil "github.com/rkujawa/uiscsi/test"
)

// TestSCSICommand_ImmediateDataMatrix verifies SCSI Command PDU wire fields
// across the ImmediateData/InitialR2T parameter matrix.
// Conformance: SCSI-01 (FFP #16.1.1), SCSI-02 (FFP #16.1.2),
// SCSI-04 (FFP #16.2.2), SCSI-05 (FFP #16.2.3)
func TestSCSICommand_ImmediateDataMatrix(t *testing.T) {
	// SCSI-01 (FFP #16.1.1): ImmediateData=Yes, all data fits in immediate
	// segment (EDTL = MaxRecvDSL = FirstBurstLength).
	t.Run("ImmediateData=Yes", func(t *testing.T) {
		negCfg := testutil.NegotiationConfig{
			ImmediateData:            testutil.BoolPtr(true),
			InitialR2T:               testutil.BoolPtr(false),
			MaxRecvDataSegmentLength: testutil.Uint32Ptr(512),
			FirstBurstLength:         testutil.Uint32Ptr(512),
		}
		setup := newWriteTestSetup(t, negCfg)

		setup.Target.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
			expCmdSN, maxCmdSN := setup.Target.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
			// ImmediateData=Yes, EDTL=FBL=MaxRecvDSL=512: all data in immediate.
			// No unsolicited Data-Out expected (FBL exhausted by immediate).
			return sendSCSIResponse(tc, cmd, expCmdSN, maxCmdSN)
		})

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		sess := setup.dialWithOverrides(t, ctx, map[string]string{
			"ImmediateData": "Yes",
			"InitialR2T":    "No",
		})

		data := make([]byte, 512)
		if err := sess.WriteBlocks(ctx, 0, 0, 1, 512, data); err != nil {
			t.Fatalf("WriteBlocks: %v", err)
		}

		cmds := setup.Recorder.Sent(pdu.OpSCSICommand)
		if len(cmds) < 1 {
			t.Fatalf("captured SCSI Command PDUs: got %d, want >= 1", len(cmds))
		}
		scsiCmd := cmds[0].Decoded.(*pdu.SCSICommand)

		if !scsiCmd.Write {
			t.Error("SCSI Command Write=false, want true")
		}
		if scsiCmd.DataSegmentLen != 512 {
			t.Errorf("SCSI Command DataSegmentLen=%d, want 512", scsiCmd.DataSegmentLen)
		}
		if scsiCmd.ExpectedDataTransferLength != 512 {
			t.Errorf("SCSI Command EDTL=%d, want 512", scsiCmd.ExpectedDataTransferLength)
		}
		if !scsiCmd.Header.Final {
			t.Error("SCSI Command Final=false, want true")
		}
	})

	// SCSI-02 (FFP #16.1.2): ImmediateData=No, InitialR2T=Yes.
	// No immediate data in SCSI Command; target sends R2T for all data.
	t.Run("ImmediateData=No", func(t *testing.T) {
		negCfg := testutil.NegotiationConfig{
			ImmediateData:            testutil.BoolPtr(false),
			InitialR2T:               testutil.BoolPtr(true),
			MaxRecvDataSegmentLength: testutil.Uint32Ptr(512),
		}
		setup := newWriteTestSetup(t, negCfg)

		setup.Target.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
			expCmdSN, maxCmdSN := setup.Target.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
			// Send R2T for full EDTL, consume Data-Out, send response.
			_, maxCmdSN, err := sendR2TAndConsume(tc, setup.Target, cmd, 0x00000001, 0, cmd.ExpectedDataTransferLength)
			if err != nil {
				return err
			}
			return sendSCSIResponse(tc, cmd, expCmdSN, maxCmdSN)
		})

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		sess := setup.dialWithOverrides(t, ctx, map[string]string{
			"ImmediateData": "No",
			"InitialR2T":    "Yes",
		})

		data := make([]byte, 512)
		if err := sess.WriteBlocks(ctx, 0, 0, 1, 512, data); err != nil {
			t.Fatalf("WriteBlocks: %v", err)
		}

		cmds := setup.Recorder.Sent(pdu.OpSCSICommand)
		if len(cmds) < 1 {
			t.Fatalf("captured SCSI Command PDUs: got %d, want >= 1", len(cmds))
		}
		scsiCmd := cmds[0].Decoded.(*pdu.SCSICommand)

		if !scsiCmd.Write {
			t.Error("SCSI Command Write=false, want true")
		}
		if scsiCmd.DataSegmentLen != 0 {
			t.Errorf("SCSI Command DataSegmentLen=%d, want 0", scsiCmd.DataSegmentLen)
		}
		if scsiCmd.ExpectedDataTransferLength != 512 {
			t.Errorf("SCSI Command EDTL=%d, want 512", scsiCmd.ExpectedDataTransferLength)
		}
		if !scsiCmd.Header.Final {
			t.Error("SCSI Command Final=false, want true (InitialR2T=Yes means no unsolicited)")
		}
	})

	// SCSI-04 (FFP #16.2.2): ImmediateData=No, InitialR2T=Yes.
	// Verify no unsolicited Data-Out (TTT=0xFFFFFFFF) is sent at all.
	t.Run("ImmediateData=No/InitialR2T=Yes/no-unsolicited", func(t *testing.T) {
		negCfg := testutil.NegotiationConfig{
			ImmediateData:            testutil.BoolPtr(false),
			InitialR2T:               testutil.BoolPtr(true),
			MaxRecvDataSegmentLength: testutil.Uint32Ptr(512),
		}
		setup := newWriteTestSetup(t, negCfg)

		setup.Target.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
			expCmdSN, maxCmdSN := setup.Target.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
			// Send R2T for full EDTL, consume Data-Out, send response.
			_, maxCmdSN, err := sendR2TAndConsume(tc, setup.Target, cmd, 0x00000001, 0, cmd.ExpectedDataTransferLength)
			if err != nil {
				return err
			}
			return sendSCSIResponse(tc, cmd, expCmdSN, maxCmdSN)
		})

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		sess := setup.dialWithOverrides(t, ctx, map[string]string{
			"ImmediateData": "No",
			"InitialR2T":    "Yes",
		})

		data := make([]byte, 1024)
		if err := sess.WriteBlocks(ctx, 0, 0, 2, 512, data); err != nil {
			t.Fatalf("WriteBlocks: %v", err)
		}

		cmds := setup.Recorder.Sent(pdu.OpSCSICommand)
		if len(cmds) < 1 {
			t.Fatalf("captured SCSI Command PDUs: got %d, want >= 1", len(cmds))
		}
		scsiCmd := cmds[0].Decoded.(*pdu.SCSICommand)

		if scsiCmd.DataSegmentLen != 0 {
			t.Errorf("SCSI Command DataSegmentLen=%d, want 0", scsiCmd.DataSegmentLen)
		}

		// Verify no unsolicited Data-Out PDUs (TTT=0xFFFFFFFF).
		douts := setup.Recorder.Sent(pdu.OpDataOut)
		for i, c := range douts {
			dout := c.Decoded.(*pdu.DataOut)
			if dout.TargetTransferTag == 0xFFFFFFFF {
				t.Errorf("DataOut[%d] has TTT=0xFFFFFFFF (unsolicited), want none", i)
			}
		}
	})

	// SCSI-05 (FFP #16.2.3): ImmediateData=No, InitialR2T=No.
	// No immediate data in SCSI Command, but unsolicited Data-Out
	// PDUs (TTT=0xFFFFFFFF) ARE sent since InitialR2T=No.
	t.Run("ImmediateData=No/InitialR2T=No/no-immediate", func(t *testing.T) {
		negCfg := testutil.NegotiationConfig{
			ImmediateData:            testutil.BoolPtr(false),
			InitialR2T:               testutil.BoolPtr(false),
			FirstBurstLength:         testutil.Uint32Ptr(1024),
			MaxRecvDataSegmentLength: testutil.Uint32Ptr(512),
		}
		setup := newWriteTestSetup(t, negCfg)

		setup.Target.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
			expCmdSN, maxCmdSN := setup.Target.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
			// Read unsolicited Data-Out until F-bit.
			if _, err := testutil.ReadDataOutPDUs(tc); err != nil {
				return err
			}
			return sendSCSIResponse(tc, cmd, expCmdSN, maxCmdSN)
		})

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		sess := setup.dialWithOverrides(t, ctx, map[string]string{
			"ImmediateData": "No",
			"InitialR2T":    "No",
		})

		// Use EDTL=1024 to match FBL=1024 so unsolicited can complete.
		data := make([]byte, 1024)
		if err := sess.WriteBlocks(ctx, 0, 0, 2, 512, data); err != nil {
			t.Fatalf("WriteBlocks: %v", err)
		}

		cmds := setup.Recorder.Sent(pdu.OpSCSICommand)
		if len(cmds) < 1 {
			t.Fatalf("captured SCSI Command PDUs: got %d, want >= 1", len(cmds))
		}
		scsiCmd := cmds[0].Decoded.(*pdu.SCSICommand)

		if scsiCmd.DataSegmentLen != 0 {
			t.Errorf("SCSI Command DataSegmentLen=%d, want 0 (no immediate data)", scsiCmd.DataSegmentLen)
		}

		// Verify unsolicited Data-Out PDUs (TTT=0xFFFFFFFF) exist.
		douts := setup.Recorder.Sent(pdu.OpDataOut)
		var unsolicitedCount int
		for _, c := range douts {
			dout := c.Decoded.(*pdu.DataOut)
			if dout.TargetTransferTag == 0xFFFFFFFF {
				unsolicitedCount++
			}
		}
		if unsolicitedCount == 0 {
			t.Error("expected unsolicited Data-Out (TTT=0xFFFFFFFF), got none")
		}
	})
}
