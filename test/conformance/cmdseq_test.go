package conformance_test

import (
	"context"
	"testing"
	"time"

	"github.com/rkujawa/uiscsi"
	"github.com/rkujawa/uiscsi/internal/pdu"
	testutil "github.com/rkujawa/uiscsi/test"
	"github.com/rkujawa/uiscsi/test/pducapture"
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
		if cmd.Header.Immediate {
			t.Errorf("SCSI command[%d] has Immediate=true, want false", i)
		}
	}
}
