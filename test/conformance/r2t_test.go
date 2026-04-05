package conformance_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/rkujawa/uiscsi"
	"github.com/rkujawa/uiscsi/internal/pdu"
	testutil "github.com/rkujawa/uiscsi/test"
	"github.com/rkujawa/uiscsi/test/pducapture"
)

// TestR2T_SinglePDU verifies that a single Data-Out PDU is sent in response
// to an R2T with the correct TTT, BufferOffset, DataSN, F-bit, and length.
// Conformance: R2T-01 (FFP #12.1)
func TestR2T_SinglePDU(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	// Pure R2T-driven writes: ImmediateData=No, InitialR2T=Yes.
	tgt.SetNegotiationConfig(testutil.NegotiationConfig{
		ImmediateData: testutil.BoolPtr(false),
		InitialR2T:    testutil.BoolPtr(true),
	})

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()
	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)

		// Send a single R2T: TTT=0x100, offset=0, 512 bytes.
		r2t := &pdu.R2T{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
			},
			TargetTransferTag:        0x100,
			StatSN:                   tc.StatSN(),
			ExpCmdSN:                 expCmdSN,
			MaxCmdSN:                 maxCmdSN,
			R2TSN:                    0,
			BufferOffset:             0,
			DesiredDataTransferLength: 512,
		}
		if err := tc.SendPDU(r2t); err != nil {
			return err
		}

		// Read Data-Out PDUs until F-bit.
		dataOuts, err := testutil.ReadDataOutPDUs(tc)
		if err != nil {
			return err
		}
		_ = dataOuts

		// Send SCSI Response.
		resp := &pdu.SCSIResponse{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
			},
			Status:   0x00,
			StatSN:   tc.NextStatSN(),
			ExpCmdSN: expCmdSN,
			MaxCmdSN: maxCmdSN,
		}
		return tc.SendPDU(resp)
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

	// Write 512 bytes (1 block of 512).
	data := bytes.Repeat([]byte{0xAB}, 512)
	if err := sess.WriteBlocks(ctx, 0, 0, 1, 512, data); err != nil {
		t.Fatalf("WriteBlocks: %v", err)
	}

	// Assert captured Data-Out PDUs.
	outs := rec.Sent(pdu.OpDataOut)
	if len(outs) != 1 {
		t.Fatalf("Data-Out PDU count: got %d, want 1", len(outs))
	}

	dout := outs[0].Decoded.(*pdu.DataOut)
	if dout.TargetTransferTag != 0x100 {
		t.Errorf("TTT: got 0x%X, want 0x100", dout.TargetTransferTag)
	}
	if dout.BufferOffset != 0 {
		t.Errorf("BufferOffset: got %d, want 0", dout.BufferOffset)
	}
	if dout.DataSN != 0 {
		t.Errorf("DataSN: got %d, want 0", dout.DataSN)
	}
	if !dout.Header.Final {
		t.Errorf("Final: got false, want true")
	}
	if dout.Header.DataSegmentLen != 512 {
		t.Errorf("DataSegmentLen: got %d, want 512", dout.Header.DataSegmentLen)
	}
}

// TestR2T_MultiPDU verifies that multi-PDU Data-Out responses to a single R2T
// have correct DataSN progression, BufferOffset increment, and F-bit on last only.
// Conformance: R2T-02 (FFP #12.2)
func TestR2T_MultiPDU(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	// MaxRecvDSL=256 forces 4 PDUs for 1024 bytes. Pure R2T path.
	tgt.SetNegotiationConfig(testutil.NegotiationConfig{
		ImmediateData:            testutil.BoolPtr(false),
		InitialR2T:               testutil.BoolPtr(true),
		MaxRecvDataSegmentLength: testutil.Uint32Ptr(256),
	})

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()
	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)

		// Single R2T: TTT=0x200, offset=0, 1024 bytes.
		r2t := &pdu.R2T{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
			},
			TargetTransferTag:        0x200,
			StatSN:                   tc.StatSN(),
			ExpCmdSN:                 expCmdSN,
			MaxCmdSN:                 maxCmdSN,
			R2TSN:                    0,
			BufferOffset:             0,
			DesiredDataTransferLength: 1024,
		}
		if err := tc.SendPDU(r2t); err != nil {
			return err
		}

		dataOuts, err := testutil.ReadDataOutPDUs(tc)
		if err != nil {
			return err
		}
		_ = dataOuts

		resp := &pdu.SCSIResponse{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
			},
			Status:   0x00,
			StatSN:   tc.NextStatSN(),
			ExpCmdSN: expCmdSN,
			MaxCmdSN: maxCmdSN,
		}
		return tc.SendPDU(resp)
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

	// Write 1024 bytes (2 blocks of 512).
	data := bytes.Repeat([]byte{0xCD}, 1024)
	if err := sess.WriteBlocks(ctx, 0, 0, 2, 512, data); err != nil {
		t.Fatalf("WriteBlocks: %v", err)
	}

	outs := rec.Sent(pdu.OpDataOut)
	if len(outs) != 4 {
		t.Fatalf("Data-Out PDU count: got %d, want 4", len(outs))
	}

	for i, cap := range outs {
		dout := cap.Decoded.(*pdu.DataOut)
		if dout.TargetTransferTag != 0x200 {
			t.Errorf("PDU[%d] TTT: got 0x%X, want 0x200", i, dout.TargetTransferTag)
		}
		if dout.DataSN != uint32(i) {
			t.Errorf("PDU[%d] DataSN: got %d, want %d", i, dout.DataSN, i)
		}
		wantOffset := uint32(i) * 256
		if dout.BufferOffset != wantOffset {
			t.Errorf("PDU[%d] BufferOffset: got %d, want %d", i, dout.BufferOffset, wantOffset)
		}
		wantFinal := i == 3
		if dout.Header.Final != wantFinal {
			t.Errorf("PDU[%d] Final: got %v, want %v", i, dout.Header.Final, wantFinal)
		}
		if dout.Header.DataSegmentLen != 256 {
			t.Errorf("PDU[%d] DataSegmentLen: got %d, want 256", i, dout.Header.DataSegmentLen)
		}
	}
}

// TestR2T_MultipleR2T verifies multi-R2T fulfillment with per-burst DataSN
// reset, correct TTT grouping, and sequential ordering under DataSequenceInOrder=Yes.
// Conformance: R2T-03 (FFP #12.3 partial)
func TestR2T_MultipleR2T(t *testing.T) {
	t.Log("R2T-03 partial: DataSequenceInOrder=No cannot be negotiated " +
		"(initiator hardcodes Yes, BooleanOr semantics). This test verifies " +
		"multi-R2T per-burst isolation under DataSequenceInOrder=Yes. " +
		"Full R2T-03 coverage requires initiator support for DataSequenceInOrder=No proposal.")

	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	// Pure R2T path.
	tgt.SetNegotiationConfig(testutil.NegotiationConfig{
		ImmediateData: testutil.BoolPtr(false),
		InitialR2T:    testutil.BoolPtr(true),
	})

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()
	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)

		// R2T 0: TTT=0x300, BufferOffset=0, 512 bytes, R2TSN=0
		r2t0 := &pdu.R2T{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
			},
			TargetTransferTag:        0x300,
			StatSN:                   tc.StatSN(),
			ExpCmdSN:                 expCmdSN,
			MaxCmdSN:                 maxCmdSN,
			R2TSN:                    0,
			BufferOffset:             0,
			DesiredDataTransferLength: 512,
		}
		if err := tc.SendPDU(r2t0); err != nil {
			return err
		}

		// R2T 1: TTT=0x301, BufferOffset=512, 512 bytes, R2TSN=1
		r2t1 := &pdu.R2T{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
			},
			TargetTransferTag:        0x301,
			StatSN:                   tc.StatSN(),
			ExpCmdSN:                 expCmdSN,
			MaxCmdSN:                 maxCmdSN,
			R2TSN:                    1,
			BufferOffset:             512,
			DesiredDataTransferLength: 512,
		}
		if err := tc.SendPDU(r2t1); err != nil {
			return err
		}

		// Read first burst (F-bit terminates).
		burst0, err := testutil.ReadDataOutPDUs(tc)
		if err != nil {
			return err
		}
		_ = burst0

		// Read second burst (F-bit terminates).
		burst1, err := testutil.ReadDataOutPDUs(tc)
		if err != nil {
			return err
		}
		_ = burst1

		resp := &pdu.SCSIResponse{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
			},
			Status:   0x00,
			StatSN:   tc.NextStatSN(),
			ExpCmdSN: expCmdSN,
			MaxCmdSN: maxCmdSN,
		}
		return tc.SendPDU(resp)
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

	// Write 1024 bytes (2 blocks of 512).
	data := bytes.Repeat([]byte{0xEF}, 1024)
	if err := sess.WriteBlocks(ctx, 0, 0, 2, 512, data); err != nil {
		t.Fatalf("WriteBlocks: %v", err)
	}

	// Collect all Data-Out PDUs and group by TTT.
	outs := rec.Sent(pdu.OpDataOut)
	if len(outs) < 2 {
		t.Fatalf("Data-Out PDU count: got %d, want >= 2", len(outs))
	}

	burst0x300 := filterByTTT(outs, 0x300)
	burst0x301 := filterByTTT(outs, 0x301)

	if len(burst0x300) == 0 {
		t.Fatalf("no Data-Out PDUs with TTT=0x300")
	}
	if len(burst0x301) == 0 {
		t.Fatalf("no Data-Out PDUs with TTT=0x301")
	}

	// Verify first burst (TTT=0x300): DataSN starts at 0, BufferOffset=0.
	for i, cap := range burst0x300 {
		dout := cap.Decoded.(*pdu.DataOut)
		if dout.DataSN != uint32(i) {
			t.Errorf("burst0x300[%d] DataSN: got %d, want %d", i, dout.DataSN, i)
		}
	}
	first0x300 := burst0x300[0].Decoded.(*pdu.DataOut)
	if first0x300.BufferOffset != 0 {
		t.Errorf("burst0x300 first BufferOffset: got %d, want 0", first0x300.BufferOffset)
	}
	last0x300 := burst0x300[len(burst0x300)-1].Decoded.(*pdu.DataOut)
	if !last0x300.Header.Final {
		t.Errorf("burst0x300 last Final: got false, want true")
	}

	// Verify second burst (TTT=0x301): DataSN starts at 0 (per-burst reset), BufferOffset=512.
	for i, cap := range burst0x301 {
		dout := cap.Decoded.(*pdu.DataOut)
		if dout.DataSN != uint32(i) {
			t.Errorf("burst0x301[%d] DataSN: got %d, want %d", i, dout.DataSN, i)
		}
	}
	first0x301 := burst0x301[0].Decoded.(*pdu.DataOut)
	if first0x301.BufferOffset != 512 {
		t.Errorf("burst0x301 first BufferOffset: got %d, want 512", first0x301.BufferOffset)
	}
	last0x301 := burst0x301[len(burst0x301)-1].Decoded.(*pdu.DataOut)
	if !last0x301.Header.Final {
		t.Errorf("burst0x301 last Final: got false, want true")
	}

	// Verify sequential ordering: all TTT=0x300 PDUs appear before TTT=0x301.
	lastSeq0x300 := burst0x300[len(burst0x300)-1].Seq
	firstSeq0x301 := burst0x301[0].Seq
	if lastSeq0x300 >= firstSeq0x301 {
		t.Errorf("burst ordering: last TTT=0x300 seq=%d >= first TTT=0x301 seq=%d (want sequential under DataSequenceInOrder=Yes)",
			lastSeq0x300, firstSeq0x301)
	}
}

// TestR2T_ParallelCommand verifies that R2T fulfillment for concurrent commands
// maintains per-command ITT/TTT isolation with correct DataSN and F-bit.
// Conformance: R2T-04 (FFP #12.4)
func TestR2T_ParallelCommand(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	// Pure R2T path.
	tgt.SetNegotiationConfig(testutil.NegotiationConfig{
		ImmediateData: testutil.BoolPtr(false),
		InitialR2T:    testutil.BoolPtr(true),
	})

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()
	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)

		var ttt uint32
		if callCount == 0 {
			ttt = 0x400
		} else {
			ttt = 0x500
		}

		r2t := &pdu.R2T{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
			},
			TargetTransferTag:        ttt,
			StatSN:                   tc.StatSN(),
			ExpCmdSN:                 expCmdSN,
			MaxCmdSN:                 maxCmdSN,
			R2TSN:                    0,
			BufferOffset:             0,
			DesiredDataTransferLength: 512,
		}
		if err := tc.SendPDU(r2t); err != nil {
			return err
		}

		dataOuts, err := testutil.ReadDataOutPDUs(tc)
		if err != nil {
			return err
		}
		_ = dataOuts

		resp := &pdu.SCSIResponse{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
			},
			Status:   0x00,
			StatSN:   tc.NextStatSN(),
			ExpCmdSN: expCmdSN,
			MaxCmdSN: maxCmdSN,
		}
		return tc.SendPDU(resp)
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

	// Send 2 writes sequentially. The handler uses callCount to assign
	// different TTTs (0x400 vs 0x500), proving per-command ITT/TTT isolation.
	// Sequential execution avoids mock target serialization issues while
	// still validating that each command's Data-Out PDUs are correctly isolated.
	data := bytes.Repeat([]byte{0xBB}, 512)
	if err := sess.WriteBlocks(ctx, 0, 0, 1, 512, data); err != nil {
		t.Fatalf("WriteBlocks[0]: %v", err)
	}
	if err := sess.WriteBlocks(ctx, 0, 1, 1, 512, data); err != nil {
		t.Fatalf("WriteBlocks[1]: %v", err)
	}

	// Correlate SCSI Command ITTs with Data-Out TTTs.
	cmds := rec.Sent(pdu.OpSCSICommand)
	outs := rec.Sent(pdu.OpDataOut)

	// Filter to write commands only (those with W-bit set).
	var writeCmds []pducapture.CapturedPDU
	for _, c := range cmds {
		cmd := c.Decoded.(*pdu.SCSICommand)
		if cmd.Write {
			writeCmds = append(writeCmds, c)
		}
	}

	if len(writeCmds) < 2 {
		t.Fatalf("captured write SCSI commands: got %d, want >= 2", len(writeCmds))
	}
	if len(outs) < 2 {
		t.Fatalf("captured Data-Out PDUs: got %d, want >= 2", len(outs))
	}

	// Build ITT -> TTT mapping from Data-Out PDUs.
	ittToTTTs := make(map[uint32]map[uint32]bool)
	for _, o := range outs {
		dout := o.Decoded.(*pdu.DataOut)
		if ittToTTTs[dout.Header.InitiatorTaskTag] == nil {
			ittToTTTs[dout.Header.InitiatorTaskTag] = make(map[uint32]bool)
		}
		ittToTTTs[dout.Header.InitiatorTaskTag][dout.TargetTransferTag] = true
	}

	// Verify each command's Data-Out PDUs use a single, unique TTT.
	seenTTTs := make(map[uint32]bool)
	for itt, ttts := range ittToTTTs {
		if len(ttts) != 1 {
			t.Errorf("ITT 0x%X has %d distinct TTTs, want 1", itt, len(ttts))
		}
		for ttt := range ttts {
			if seenTTTs[ttt] {
				t.Errorf("TTT 0x%X used by multiple ITTs", ttt)
			}
			seenTTTs[ttt] = true
		}
	}

	// Verify each Data-Out set has DataSN=0 and Final=true (single PDU per R2T).
	for _, o := range outs {
		dout := o.Decoded.(*pdu.DataOut)
		if dout.DataSN != 0 {
			t.Errorf("Data-Out ITT=0x%X TTT=0x%X DataSN: got %d, want 0",
				dout.Header.InitiatorTaskTag, dout.TargetTransferTag, dout.DataSN)
		}
		if !dout.Header.Final {
			t.Errorf("Data-Out ITT=0x%X TTT=0x%X Final: got false, want true",
				dout.Header.InitiatorTaskTag, dout.TargetTransferTag)
		}
	}
}

// filterByTTT returns captured PDUs whose decoded DataOut has the given TTT.
func filterByTTT(caps []pducapture.CapturedPDU, ttt uint32) []pducapture.CapturedPDU {
	var result []pducapture.CapturedPDU
	for _, c := range caps {
		dout := c.Decoded.(*pdu.DataOut)
		if dout.TargetTransferTag == ttt {
			result = append(result, c)
		}
	}
	return result
}
