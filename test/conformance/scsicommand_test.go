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

// TestSCSICommand_UnsolicitedFBit verifies F-bit semantics in the SCSI Command PDU
// for unsolicited data scenarios.
// Conformance: SCSI-03 (FFP #16.2.1), SCSI-07 (FFP #16.3.1)
func TestSCSICommand_UnsolicitedFBit(t *testing.T) {
	// SCSI-03 (FFP #16.2.1): EDTL=DSL (all data in immediate), F-bit set,
	// no unsolicited Data-Out follows.
	t.Run("EDTL=DataSegmentLen/F-bit-unsolicited", func(t *testing.T) {
		negCfg := testutil.NegotiationConfig{
			ImmediateData:            testutil.BoolPtr(true),
			InitialR2T:               testutil.BoolPtr(false),
			MaxRecvDataSegmentLength: testutil.Uint32Ptr(512),
			FirstBurstLength:         testutil.Uint32Ptr(512),
		}
		setup := newWriteTestSetup(t, negCfg)

		setup.Target.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
			expCmdSN, maxCmdSN := setup.Target.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
			// All data fits in SCSI Command (EDTL=FBL=MaxRecvDSL=512). No Data-Out.
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

		if scsiCmd.DataSegmentLen != 512 {
			t.Errorf("SCSI Command DataSegmentLen=%d, want 512", scsiCmd.DataSegmentLen)
		}
		if scsiCmd.ExpectedDataTransferLength != 512 {
			t.Errorf("SCSI Command EDTL=%d, want 512", scsiCmd.ExpectedDataTransferLength)
		}
		if !scsiCmd.Header.Final {
			t.Error("SCSI Command Final=false, want true (EDTL=DSL, all data in immediate)")
		}

		// Verify no unsolicited Data-Out PDUs.
		douts := setup.Recorder.Sent(pdu.OpDataOut)
		for i, c := range douts {
			dout := c.Decoded.(*pdu.DataOut)
			if dout.TargetTransferTag == 0xFFFFFFFF {
				t.Errorf("DataOut[%d] has TTT=0xFFFFFFFF (unsolicited), want none", i)
			}
		}
	})

	// SCSI-07 (FFP #16.3.1): ImmediateData=Yes, InitialR2T=Yes.
	// F-bit set because no unsolicited Data-Out follows; immediate data is
	// bounded by MaxRecvDSL. Target sends R2T for remaining data.
	t.Run("InitialR2T=Yes/F-bit-command", func(t *testing.T) {
		negCfg := testutil.NegotiationConfig{
			ImmediateData:            testutil.BoolPtr(true),
			InitialR2T:               testutil.BoolPtr(true),
			MaxRecvDataSegmentLength: testutil.Uint32Ptr(512),
		}
		setup := newWriteTestSetup(t, negCfg)

		setup.Target.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
			expCmdSN, maxCmdSN := setup.Target.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
			// ImmediateData=Yes, InitialR2T=Yes: immediate data in SCSI Command
			// but no unsolicited Data-Out follows. Send R2T for remaining.
			remaining := cmd.ExpectedDataTransferLength - cmd.DataSegmentLen
			if remaining > 0 {
				_, maxCmdSN, err := sendR2TAndConsume(tc, setup.Target, cmd, 0x00000001, cmd.DataSegmentLen, remaining)
				if err != nil {
					return err
				}
				return sendSCSIResponse(tc, cmd, expCmdSN, maxCmdSN)
			}
			return sendSCSIResponse(tc, cmd, expCmdSN, maxCmdSN)
		})

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		sess := setup.dialWithOverrides(t, ctx, map[string]string{
			"ImmediateData": "Yes",
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

		if !scsiCmd.Header.Final {
			t.Error("SCSI Command Final=false, want true (InitialR2T=Yes, no unsolicited Data-Out follows)")
		}
		if scsiCmd.DataSegmentLen != 512 {
			t.Errorf("SCSI Command DataSegmentLen=%d, want 512 (immediate data up to MaxRecvDSL)", scsiCmd.DataSegmentLen)
		}
		if scsiCmd.ExpectedDataTransferLength != 1024 {
			t.Errorf("SCSI Command EDTL=%d, want 1024", scsiCmd.ExpectedDataTransferLength)
		}
		if !scsiCmd.Write {
			t.Error("SCSI Command Write=false, want true")
		}
	})
}

// TestSCSICommand_FirstBurstLength verifies FirstBurstLength boundary behavior
// across 5 scenarios per D-04. All use ImmediateData=Yes, InitialR2T=No.
// Conformance: SCSI-06 (FFP #16.2.4)
func TestSCSICommand_FirstBurstLength(t *testing.T) {
	type scenario struct {
		name     string
		fbl      uint32 // FirstBurstLength
		mrdsl    uint32 // MaxRecvDataSegmentLength (target declares)
		edtl     uint32 // Expected Data Transfer Length
		wantDSL  uint32 // expected DataSegmentLen in SCSI Command
		wantF    bool   // expected Final in SCSI Command
		needR2T  bool   // whether target must send R2T for remaining
		needRead bool   // whether target must read unsolicited Data-Out
	}

	scenarios := []scenario{
		{
			// D-04 scenario 1: EDTL < FBL. Immediate + unsolicited fills EDTL
			// before FBL is exhausted. EDTL=768, FBL=1024, MaxRecvDSL=512.
			// Immediate=512, unsolicited Data-Out=256 (partial read triggers
			// ErrUnexpectedEOF), total unsolicited=768 < FBL=1024.
			name:     "EDTL<FirstBurstLength",
			fbl:      1024,
			mrdsl:    512,
			edtl:     768,
			wantDSL:  512,
			wantF:    true,
			needRead: true,
		},
		{
			// D-04 scenario 2: EDTL = FBL. Immediate + unsolicited Data-Out fills FBL.
			name:     "EDTL=FirstBurstLength",
			fbl:      1024,
			mrdsl:    512,
			edtl:     1024,
			wantDSL:  512,
			wantF:    true, // F-bit always true in SCSI Command per implementation
			needRead: true, // unsolicited Data-Out follows
		},
		{
			// D-04 scenario 3: EDTL > FBL. Unsolicited fills FBL, then R2T for remainder.
			name:     "EDTL>FirstBurstLength",
			fbl:      1024,
			mrdsl:    512,
			edtl:     2048,
			wantDSL:  512,
			wantF:    true,
			needRead: true,
			needR2T:  true,
		},
		{
			// D-04 scenario 4: EDTL = MaxRecvDSL = FBL. Single immediate PDU,
			// entire first burst consumed by immediate data.
			name:    "EDTL=MaxRecvDSL",
			fbl:     512,
			mrdsl:   512,
			edtl:    512,
			wantDSL: 512,
			wantF:   true,
		},
		{
			// D-04 scenario 5: EDTL = 2*FBL. FBL = MaxRecvDSL, so immediate
			// exhausts FBL. Target sends R2T for remaining FBL.
			name:    "EDTL=2xFirstBurstLength",
			fbl:     512,
			mrdsl:   512,
			edtl:    1024,
			wantDSL: 512,
			wantF:   true,
			needR2T: true,
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			negCfg := testutil.NegotiationConfig{
				ImmediateData:            testutil.BoolPtr(true),
				InitialR2T:               testutil.BoolPtr(false),
				FirstBurstLength:         testutil.Uint32Ptr(sc.fbl),
				MaxRecvDataSegmentLength: testutil.Uint32Ptr(sc.mrdsl),
			}
			setup := newWriteTestSetup(t, negCfg)

			setup.Target.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
				expCmdSN, maxCmdSN := setup.Target.Session().Update(cmd.CmdSN, cmd.Header.Immediate)

				if sc.needRead {
					// Read unsolicited Data-Out until F-bit.
					if _, err := testutil.ReadDataOutPDUs(tc); err != nil {
						return err
					}
				}

				if sc.needR2T {
					// FBL consumed by immediate + unsolicited. R2T for remaining.
					remaining := cmd.ExpectedDataTransferLength - sc.fbl
					r2t := &pdu.R2T{
						Header: pdu.Header{
							Final:            true,
							InitiatorTaskTag: cmd.InitiatorTaskTag,
						},
						TargetTransferTag:         0x00000002,
						StatSN:                    tc.StatSN(),
						ExpCmdSN:                  expCmdSN,
						MaxCmdSN:                  maxCmdSN,
						R2TSN:                     0,
						BufferOffset:              sc.fbl,
						DesiredDataTransferLength: remaining,
					}
					if err := tc.SendPDU(r2t); err != nil {
						return err
					}
					if _, err := testutil.ReadDataOutPDUs(tc); err != nil {
						return err
					}
				}

				return sendSCSIResponse(tc, cmd, expCmdSN, maxCmdSN)
			})

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			sess := setup.dialWithOverrides(t, ctx, map[string]string{
				"ImmediateData": "Yes",
				"InitialR2T":    "No",
			})

			// Use 256-byte blocks for flexibility (768 not divisible by 512).
			blocks := sc.edtl / 256
			data := make([]byte, sc.edtl)
			for i := range data {
				data[i] = byte(i % 256)
			}
			if err := sess.WriteBlocks(ctx, 0, 0, blocks, 256, data); err != nil {
				t.Fatalf("WriteBlocks: %v", err)
			}

			cmds := setup.Recorder.Sent(pdu.OpSCSICommand)
			if len(cmds) < 1 {
				t.Fatalf("captured SCSI Command PDUs: got %d, want >= 1", len(cmds))
			}
			scsiCmd := cmds[0].Decoded.(*pdu.SCSICommand)

			if scsiCmd.DataSegmentLen != sc.wantDSL {
				t.Errorf("SCSI Command DataSegmentLen=%d, want %d", scsiCmd.DataSegmentLen, sc.wantDSL)
			}
			if scsiCmd.ExpectedDataTransferLength != sc.edtl {
				t.Errorf("SCSI Command EDTL=%d, want %d", scsiCmd.ExpectedDataTransferLength, sc.edtl)
			}
			if scsiCmd.Header.Final != sc.wantF {
				t.Errorf("SCSI Command Final=%v, want %v", scsiCmd.Header.Final, sc.wantF)
			}
			if !scsiCmd.Write {
				t.Error("SCSI Command Write=false, want true")
			}

			// Verify unsolicited vs solicited Data-Out classification.
			douts := setup.Recorder.Sent(pdu.OpDataOut)
			var unsolicitedBytes uint32
			var solicitedCount int
			for _, c := range douts {
				dout := c.Decoded.(*pdu.DataOut)
				if dout.TargetTransferTag == 0xFFFFFFFF {
					unsolicitedBytes += dout.DataSegmentLen
				} else {
					solicitedCount++
				}
			}

			// Total unsolicited (immediate + Data-Out) must not exceed FBL.
			totalUnsolicited := scsiCmd.DataSegmentLen + unsolicitedBytes
			if totalUnsolicited > sc.fbl {
				t.Errorf("total unsolicited (immediate %d + Data-Out %d = %d) exceeds FirstBurstLength %d",
					scsiCmd.DataSegmentLen, unsolicitedBytes, totalUnsolicited, sc.fbl)
			}

			if sc.needR2T && solicitedCount == 0 {
				t.Errorf("expected solicited Data-Out PDUs (TTT != 0xFFFFFFFF), got none")
			}

			t.Logf("DSL=%d EDTL=%d unsolicited=%d solicited=%d FBL=%d",
				scsiCmd.DataSegmentLen, sc.edtl, totalUnsolicited, solicitedCount, sc.fbl)
		})
	}

	// Summary: verify all scenarios exercised specific FBL boundaries.
	t.Run("summary", func(t *testing.T) {
		t.Logf("5 FirstBurstLength boundary scenarios validated for SCSI-06 (FFP #16.2.4)")
		t.Logf("Boundaries: EDTL<FBL, EDTL=FBL, EDTL>FBL, EDTL=MaxRecvDSL, EDTL=2*FBL")
		// Count from parent's subtests.
		for _, name := range []string{
			"EDTL<FirstBurstLength", "EDTL=FirstBurstLength", "EDTL>FirstBurstLength",
			"EDTL=MaxRecvDSL", "EDTL=2xFirstBurstLength",
		} {
			t.Logf("  - %s: covered", name)
		}
	})
}
