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

// TestCmdSN_SequentialIncrement verifies that CmdSN increments by exactly 1
// for each non-immediate SCSI command on the wire.
// Conformance: CMDSEQ-01 (FFP #1.1)
func TestCmdSN_SequentialIncrement(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()
	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Immediate)
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
		uiscsi.WithKeepaliveInterval(30*time.Second), // avoid keepalive interference
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	// Send 5 TEST UNIT READY commands (non-immediate SCSI commands).
	for i := range 5 {
		if err := sess.TestUnitReady(ctx, 0); err != nil {
			t.Fatalf("TestUnitReady[%d]: %v", i, err)
		}
	}

	cmds := rec.Sent(pdu.OpSCSICommand)
	if len(cmds) < 5 {
		t.Fatalf("captured SCSI commands: got %d, want >= 5", len(cmds))
	}

	// Verify CmdSN increments by exactly 1 between consecutive commands.
	for i := 1; i < len(cmds); i++ {
		prev := cmds[i-1].Decoded.(*pdu.SCSICommand)
		curr := cmds[i].Decoded.(*pdu.SCSICommand)
		delta := curr.CmdSN - prev.CmdSN
		if delta != 1 {
			t.Errorf("CmdSN[%d]=%d, CmdSN[%d]=%d: delta=%d, want 1",
				i-1, prev.CmdSN, i, curr.CmdSN, delta)
		}
	}

	// Verify none of the SCSI commands have Immediate flag set.
	for i, c := range cmds {
		cmd := c.Decoded.(*pdu.SCSICommand)
		if cmd.Immediate {
			t.Errorf("SCSI command[%d] has Immediate=true, want false", i)
		}
	}
}

// TestCmdSN_ImmediateDelivery_NonTMF verifies that NOP-Out carries
// Immediate=true and does not advance CmdSN. SCSI commands before and after
// the NOP-Out must have a CmdSN delta of exactly 1.
// Conformance: CMDSEQ-02 (FFP #2.1)
func TestCmdSN_ImmediateDelivery_NonTMF(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()

	// Custom NOP-Out handler that uses SessionState for correct ExpCmdSN.
	// NOP-Out is Immediate so SessionState.Update with immediate=true
	// does NOT advance ExpCmdSN, unlike the default HandleNOPOut which
	// hardcodes CmdSN+1.
	tgt.Handle(pdu.OpNOPOut, func(tc *testutil.TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		req := decoded.(*pdu.NOPOut)
		expCmdSN, maxCmdSN := tgt.Session().Update(req.CmdSN, req.Immediate)
		resp := &pdu.NOPIn{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: req.InitiatorTaskTag,
			},
			TargetTransferTag: 0xFFFFFFFF,
			StatSN:            tc.NextStatSN(),
			ExpCmdSN:          expCmdSN,
			MaxCmdSN:          maxCmdSN,
			Data:              req.Data,
		}
		return tc.SendPDU(resp)
	})

	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Immediate)
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

		// After first SCSI response, send a solicited NOP-In from target
		// (TTT != 0xFFFFFFFF) to force the initiator to reply with NOP-Out.
		if callCount == 0 {
			nopIn := &pdu.NOPIn{
				Header: pdu.Header{
					Final:            true,
					InitiatorTaskTag: 0xFFFFFFFF, // target-initiated
				},
				TargetTransferTag: 0x00000001, // non-reserved TTT solicits NOP-Out response
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
	time.Sleep(100 * time.Millisecond)

	// Second SCSI command.
	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady[1]: %v", err)
	}

	// Verify at least one NOP-Out was captured with Immediate=true.
	nops := rec.Sent(pdu.OpNOPOut)
	if len(nops) < 1 {
		t.Fatalf("captured NOP-Out: got %d, want >= 1", len(nops))
	}
	for i, n := range nops {
		nopOut := n.Decoded.(*pdu.NOPOut)
		if !nopOut.Immediate {
			t.Errorf("NOP-Out[%d] has Immediate=false, want true", i)
		}
	}

	// Verify the two SCSI commands have CmdSN delta=1 despite intervening NOP-Out.
	cmds := rec.Sent(pdu.OpSCSICommand)
	if len(cmds) < 2 {
		t.Fatalf("captured SCSI commands: got %d, want >= 2", len(cmds))
	}
	first := cmds[0].Decoded.(*pdu.SCSICommand)
	second := cmds[1].Decoded.(*pdu.SCSICommand)
	delta := second.CmdSN - first.CmdSN
	if delta != 1 {
		t.Errorf("CmdSN gap after NOP-Out: first=%d, second=%d, delta=%d, want 1",
			first.CmdSN, second.CmdSN, delta)
	}
}

// TestCmdSN_ImmediateDelivery_TMF verifies that TMF (LUN Reset) carries
// Immediate=true and does not advance CmdSN. SCSI commands before and after
// the TMF must have a CmdSN delta of exactly 1.
// Conformance: CMDSEQ-03 (FFP #2.2)
func TestCmdSN_ImmediateDelivery_TMF(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()
	tgt.HandleTMF()
	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Immediate)
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

	// First SCSI command: CmdSN=N.
	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady[0]: %v", err)
	}

	// LUN Reset (TMF): should be Immediate, does not advance CmdSN.
	if _, err := sess.LUNReset(ctx, 0); err != nil {
		t.Fatalf("LUNReset: %v", err)
	}

	// Second SCSI command: should be CmdSN=N+1 (not N+2).
	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady[1]: %v", err)
	}

	// Verify TMF was captured with Immediate=true.
	tmfs := rec.Sent(pdu.OpTaskMgmtReq)
	if len(tmfs) < 1 {
		t.Fatalf("captured TaskMgmtReq: got %d, want >= 1", len(tmfs))
	}
	for i, tm := range tmfs {
		tmfReq := tm.Decoded.(*pdu.TaskMgmtReq)
		if !tmfReq.Immediate {
			t.Errorf("TaskMgmtReq[%d] has Immediate=false, want true", i)
		}
	}

	// Verify the two SCSI commands have CmdSN delta=1 despite intervening TMF.
	cmds := rec.Sent(pdu.OpSCSICommand)
	if len(cmds) < 2 {
		t.Fatalf("captured SCSI commands: got %d, want >= 2", len(cmds))
	}
	first := cmds[0].Decoded.(*pdu.SCSICommand)
	second := cmds[1].Decoded.(*pdu.SCSICommand)
	delta := second.CmdSN - first.CmdSN
	if delta != 1 {
		t.Errorf("CmdSN gap after TMF: first=%d, second=%d, delta=%d, want 1",
			first.CmdSN, second.CmdSN, delta)
	}
}
