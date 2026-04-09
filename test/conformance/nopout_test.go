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

// TestNOPOut_PingResponse verifies that the initiator responds correctly to a
// target-initiated NOP-In (solicited ping). Full wire field validation per
// RFC 7143 Section 11.18 and FFP #15.1.
// Conformance: SESS-03
func TestNOPOut_PingResponse(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()

	const testTTT = uint32(0x00000042)
	testLUN := pdu.EncodeSAMLUN(1)

	// Custom NOP-Out handler that uses SessionState for correct ExpCmdSN.
	// NOP-Out response is Immediate, so SessionState.Update with immediate=true
	// does NOT advance ExpCmdSN.
	tgt.Handle(pdu.OpNOPOut, func(tc *testutil.TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		req := decoded.(*pdu.NOPOut)
		expCmdSN, maxCmdSN := tgt.Session().Update(req.CmdSN, req.Header.Immediate)
		resp := &pdu.NOPIn{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: req.Header.InitiatorTaskTag,
			},
			TargetTransferTag: 0xFFFFFFFF,
			StatSN:            tc.NextStatSN(),
			ExpCmdSN:          expCmdSN,
			MaxCmdSN:          maxCmdSN,
			Data:              req.Data,
		}
		return tc.SendPDU(resp)
	})

	// HandleSCSIFunc: on first call, inject a solicited NOP-In with known TTT and LUN.
	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)

		// Send SCSI response first.
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
		if err := tc.SendPDU(resp); err != nil {
			return err
		}

		// After first SCSI response, send solicited NOP-In with specific TTT and LUN.
		if callCount == 0 {
			nopIn := &pdu.NOPIn{
				Header: pdu.Header{
					Final:            true,
					InitiatorTaskTag: 0xFFFFFFFF, // target-initiated
					LUN:              testLUN,
				},
				TargetTransferTag: testTTT,
				StatSN:            tc.NextStatSN(),
				ExpCmdSN:          expCmdSN,
				MaxCmdSN:          maxCmdSN,
			}
			return tc.SendPDU(nopIn)
		}
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithPDUHook(rec.Hook()),
		uiscsi.WithKeepaliveInterval(30*time.Second), // avoid timer-based NOP-Out
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	// First SCSI command triggers NOP-In from target.
	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady[0]: %v", err)
	}

	// Allow time for NOP-Out response to propagate.
	time.Sleep(200 * time.Millisecond)

	// Second SCSI command to verify CmdSN is NOT incremented from the NOP-Out.
	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady[1]: %v", err)
	}

	// Find the NOP-Out that is a response (ITT=0xFFFFFFFF, TTT=testTTT).
	nops := rec.Sent(pdu.OpNOPOut)
	var responseNOP *pdu.NOPOut
	for _, n := range nops {
		nopOut := n.Decoded.(*pdu.NOPOut)
		if nopOut.Header.InitiatorTaskTag == 0xFFFFFFFF && nopOut.TargetTransferTag == testTTT {
			responseNOP = nopOut
			break
		}
	}
	if responseNOP == nil {
		t.Fatalf("no NOP-Out response found with ITT=0xFFFFFFFF and TTT=0x%08X; captured %d NOP-Outs",
			testTTT, len(nops))
	}

	// Verify all wire fields per FFP #15.1.

	// ITT = 0xFFFFFFFF (response, not new task)
	if responseNOP.Header.InitiatorTaskTag != 0xFFFFFFFF {
		t.Errorf("ITT: got 0x%08X, want 0xFFFFFFFF", responseNOP.Header.InitiatorTaskTag)
	}

	// TTT = testTTT (echoed from NOP-In)
	if responseNOP.TargetTransferTag != testTTT {
		t.Errorf("TTT: got 0x%08X, want 0x%08X", responseNOP.TargetTransferTag, testTTT)
	}

	// Immediate = true (I-bit set)
	if !responseNOP.Header.Immediate {
		t.Error("Immediate: got false, want true")
	}

	// Final = true (F-bit set)
	if !responseNOP.Header.Final {
		t.Error("Final: got false, want true")
	}

	// LUN = testLUN (echoed from NOP-In per RFC 7143 S11.18)
	if responseNOP.Header.LUN != testLUN {
		t.Errorf("LUN: got %v, want %v", responseNOP.Header.LUN, testLUN)
	}

	// CmdSN present (carried, not advanced).
	if responseNOP.CmdSN == 0 {
		t.Error("CmdSN: got 0, want non-zero (carried)")
	}

	// ExpStatSN present.
	if responseNOP.ExpStatSN == 0 {
		t.Error("ExpStatSN: got 0, want non-zero")
	}

	// Verify next SCSI command CmdSN is NOT incremented from the NOP-Out CmdSN.
	// The NOP-Out is Immediate so it does not consume a CmdSN slot.
	cmds := rec.Sent(pdu.OpSCSICommand)
	if len(cmds) < 2 {
		t.Fatalf("captured SCSI commands: got %d, want >= 2", len(cmds))
	}
	firstCmd := cmds[0].Decoded.(*pdu.SCSICommand)
	secondCmd := cmds[1].Decoded.(*pdu.SCSICommand)
	delta := secondCmd.CmdSN - firstCmd.CmdSN
	if delta != 1 {
		t.Errorf("CmdSN gap after NOP-Out: first=%d, second=%d, delta=%d, want 1",
			firstCmd.CmdSN, secondCmd.CmdSN, delta)
	}
}

// TestNOPOut_ExpStatSNConfirmation verifies that the initiator can send a
// NOP-Out ExpStatSN confirmation with ITT=0xFFFFFFFF, TTT=0xFFFFFFFF,
// Immediate=true, Final=true, and CmdSN not advanced.
// Per RFC 7143 Section 11.18 and FFP #15.3.
// Conformance: SESS-05
func TestNOPOut_ExpStatSNConfirmation(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()

	// Custom NOP-Out handler with SessionState for correct sequencing.
	tgt.Handle(pdu.OpNOPOut, func(tc *testutil.TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		req := decoded.(*pdu.NOPOut)
		// ExpStatSN confirmation has ITT=0xFFFFFFFF — no response expected.
		// Only respond to pings (ITT != 0xFFFFFFFF).
		if req.Header.InitiatorTaskTag == 0xFFFFFFFF {
			return nil
		}
		expCmdSN, maxCmdSN := tgt.Session().Update(req.CmdSN, req.Header.Immediate)
		resp := &pdu.NOPIn{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: req.Header.InitiatorTaskTag,
			},
			TargetTransferTag: 0xFFFFFFFF,
			StatSN:            tc.NextStatSN(),
			ExpCmdSN:          expCmdSN,
			MaxCmdSN:          maxCmdSN,
		}
		return tc.SendPDU(resp)
	})

	// HandleSCSIFunc: standard response for TestUnitReady.
	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
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
		uiscsi.WithKeepaliveInterval(30*time.Second), // long, to avoid auto pings
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	// Send a SCSI command first to establish CmdSN baseline.
	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady: %v", err)
	}

	// Record CmdSN of the SCSI command before confirmation.
	cmdsBeforeConfirm := rec.Sent(pdu.OpSCSICommand)
	if len(cmdsBeforeConfirm) == 0 {
		t.Fatal("no SCSI commands captured before confirmation")
	}
	cmdSNBefore := cmdsBeforeConfirm[len(cmdsBeforeConfirm)-1].Decoded.(*pdu.SCSICommand).CmdSN

	// Trigger ExpStatSN confirmation.
	if err := sess.SendExpStatSNConfirmation(); err != nil {
		t.Fatalf("SendExpStatSNConfirmation: %v", err)
	}

	// Allow time for NOP-Out to propagate.
	time.Sleep(200 * time.Millisecond)

	// Send another SCSI command to verify CmdSN was NOT advanced by the NOP-Out.
	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady[1]: %v", err)
	}

	// Find the NOP-Out with ITT=0xFFFFFFFF AND TTT=0xFFFFFFFF.
	nopouts := rec.Sent(pdu.OpNOPOut)
	var found *pdu.NOPOut
	for _, cap := range nopouts {
		nop := cap.Decoded.(*pdu.NOPOut)
		if nop.Header.InitiatorTaskTag == 0xFFFFFFFF && nop.TargetTransferTag == 0xFFFFFFFF {
			found = nop
			break
		}
	}
	if found == nil {
		t.Fatalf("no ExpStatSN confirmation NOP-Out found (ITT=TTT=0xFFFFFFFF); captured %d NOP-Outs", len(nopouts))
	}

	// Verify all wire fields per FFP #15.3.

	// I-bit = true (Immediate)
	if !found.Header.Immediate {
		t.Error("Immediate: got false, want true")
	}

	// F-bit = true (Final)
	if !found.Header.Final {
		t.Error("Final: got false, want true")
	}

	// ITT = 0xFFFFFFFF (no response expected)
	if found.Header.InitiatorTaskTag != 0xFFFFFFFF {
		t.Errorf("ITT: got 0x%08X, want 0xFFFFFFFF", found.Header.InitiatorTaskTag)
	}

	// TTT = 0xFFFFFFFF
	if found.TargetTransferTag != 0xFFFFFFFF {
		t.Errorf("TTT: got 0x%08X, want 0xFFFFFFFF", found.TargetTransferTag)
	}

	// ExpStatSN present (non-zero).
	if found.ExpStatSN == 0 {
		t.Error("ExpStatSN: got 0, want non-zero")
	}

	// CmdSN should NOT be advanced — next SCSI command CmdSN should be
	// exactly cmdSNBefore+1 (not cmdSNBefore+2).
	cmdsAfterConfirm := rec.Sent(pdu.OpSCSICommand)
	if len(cmdsAfterConfirm) < 2 {
		t.Fatalf("captured SCSI commands: got %d, want >= 2", len(cmdsAfterConfirm))
	}
	cmdSNAfter := cmdsAfterConfirm[len(cmdsAfterConfirm)-1].Decoded.(*pdu.SCSICommand).CmdSN
	delta := cmdSNAfter - cmdSNBefore
	if delta != 1 {
		t.Errorf("CmdSN gap after ExpStatSN confirmation: before=%d, after=%d, delta=%d, want 1",
			cmdSNBefore, cmdSNAfter, delta)
	}
}

// TestNOPOut_PingRequest verifies that the initiator sends keepalive NOP-Out
// pings with correct wire fields when keepalive interval triggers.
// Conformance: SESS-04 (FFP #15.2)
func TestNOPOut_PingRequest(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()

	// Custom NOP-Out handler with SessionState for correct sequencing.
	tgt.Handle(pdu.OpNOPOut, func(tc *testutil.TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		req := decoded.(*pdu.NOPOut)
		expCmdSN, maxCmdSN := tgt.Session().Update(req.CmdSN, req.Header.Immediate)
		resp := &pdu.NOPIn{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: req.Header.InitiatorTaskTag,
			},
			TargetTransferTag: 0xFFFFFFFF,
			StatSN:            tc.NextStatSN(),
			ExpCmdSN:          expCmdSN,
			MaxCmdSN:          maxCmdSN,
			Data:              req.Data,
		}
		return tc.SendPDU(resp)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithPDUHook(rec.Hook()),
		uiscsi.WithKeepaliveInterval(2*time.Second), // trigger keepalive within test timeout
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	// Wait for at least one keepalive NOP-Out to be sent.
	time.Sleep(3 * time.Second)

	// Find initiator-originated NOP-Out (ITT != 0xFFFFFFFF, TTT = 0xFFFFFFFF).
	nops := rec.Sent(pdu.OpNOPOut)
	var pingNOP *pdu.NOPOut
	for _, n := range nops {
		nopOut := n.Decoded.(*pdu.NOPOut)
		if nopOut.Header.InitiatorTaskTag != 0xFFFFFFFF && nopOut.TargetTransferTag == 0xFFFFFFFF {
			pingNOP = nopOut
			break
		}
	}
	if pingNOP == nil {
		t.Fatalf("no initiator-originated NOP-Out found (ITT!=0xFFFFFFFF, TTT=0xFFFFFFFF); captured %d NOP-Outs",
			len(nops))
	}

	// ITT != 0xFFFFFFFF (valid task tag, initiator-originated)
	if pingNOP.Header.InitiatorTaskTag == 0xFFFFFFFF {
		t.Error("ITT: got 0xFFFFFFFF, want valid task tag for initiator-originated ping")
	}

	// TTT = 0xFFFFFFFF (initiator-originated)
	if pingNOP.TargetTransferTag != 0xFFFFFFFF {
		t.Errorf("TTT: got 0x%08X, want 0xFFFFFFFF", pingNOP.TargetTransferTag)
	}

	// I-bit = true (immediate)
	if !pingNOP.Header.Immediate {
		t.Error("Immediate: got false, want true")
	}

	// Final = true
	if !pingNOP.Header.Final {
		t.Error("Final: got false, want true")
	}

	// LUN = all zeros (initiator-originated, no specific LUN)
	var zeroLUN [8]byte
	if pingNOP.Header.LUN != zeroLUN {
		t.Errorf("LUN: got %v, want all zeros", pingNOP.Header.LUN)
	}
}
