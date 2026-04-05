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

// TestAsync_LogoutRequest verifies that after receiving AsyncMsg code 1
// (target requests logout), the initiator:
//   1. Sends a LogoutReq within the Parameter3 deadline
//   2. Rejects new commands after the async event is received
//
// This test focuses on TIMING and NO-NEW-COMMANDS behavior. Wire field
// validation of the LogoutReq itself is covered by SESS-01 in session_test.go.
// Conformance: ASYNC-01 (FFP #20.1).
func TestAsync_LogoutRequest(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	asyncInjected := make(chan struct{})

	// HandleSCSIFunc: on first SCSI command, respond then inject AsyncMsg code 1.
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
		if err := tc.SendPDU(resp); err != nil {
			return err
		}

		if callCount == 0 {
			// Parameter3=1 means initiator has 1 second to logout.
			if err := tgt.SendAsyncMsg(tc, 1, testutil.AsyncParams{Parameter3: 1}); err != nil {
				return err
			}
			close(asyncInjected)
		}
		return nil
	})

	// Track async events received by the initiator.
	asyncReceived := make(chan uiscsi.AsyncEvent, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithPDUHook(rec.Hook()),
		uiscsi.WithKeepaliveInterval(30*time.Second),
		uiscsi.WithAsyncHandler(func(_ context.Context, evt uiscsi.AsyncEvent) {
			select {
			case asyncReceived <- evt:
			default:
			}
		}),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	// Trigger async injection via first SCSI command.
	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady: %v", err)
	}

	// Wait for async injection to complete on the target side.
	select {
	case <-asyncInjected:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for async injection")
	}

	// Verify async handler callback fires with EventCode=1.
	select {
	case evt := <-asyncReceived:
		if evt.EventCode != 1 {
			t.Fatalf("async EventCode: got %d, want 1", evt.EventCode)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for async event callback")
	}

	// Verify LogoutReq appears on wire within Parameter3 (1s) + buffer.
	deadline := time.After(3 * time.Second)
	for {
		logoutReqs := rec.Sent(pdu.OpLogoutReq)
		if len(logoutReqs) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("no LogoutReq sent within Parameter3 deadline + buffer")
		case <-time.After(100 * time.Millisecond):
		}
	}

	// Wait for session teardown to complete.
	time.Sleep(500 * time.Millisecond)

	// NO-NEW-COMMANDS: after async-triggered logout, commands must fail.
	cmdErr := sess.TestUnitReady(ctx, 0)
	if cmdErr == nil {
		t.Error("expected error after async-triggered logout, got nil")
	}
}

// TestAsync_ConnectionDrop verifies that after receiving AsyncMsg code 2
// (connection drop notification), the session surfaces an error on subsequent
// commands. The initiator either triggers reconnect or sets session error state.
// Conformance: ASYNC-02 (FFP #20.2).
func TestAsync_ConnectionDrop(t *testing.T) {
	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	asyncInjected := make(chan struct{})

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
		if err := tc.SendPDU(resp); err != nil {
			return err
		}

		if callCount == 0 {
			// AsyncEvent 2: connection drop. CID=0, Time2Wait=0, Time2Retain=0.
			if err := tgt.SendAsyncMsg(tc, 2, testutil.AsyncParams{
				Parameter1: 0,
				Parameter2: 0,
				Parameter3: 0,
			}); err != nil {
				return err
			}
			close(asyncInjected)
		}
		return nil
	})

	asyncReceived := make(chan uiscsi.AsyncEvent, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithKeepaliveInterval(30*time.Second),
		uiscsi.WithMaxReconnectAttempts(0), // disable reconnect to get clean error
		uiscsi.WithAsyncHandler(func(_ context.Context, evt uiscsi.AsyncEvent) {
			select {
			case asyncReceived <- evt:
			default:
			}
		}),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	// Trigger async injection.
	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady: %v", err)
	}

	select {
	case <-asyncInjected:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for async injection")
	}

	// Verify async handler callback fires with EventCode=2.
	select {
	case evt := <-asyncReceived:
		if evt.EventCode != 2 {
			t.Fatalf("async EventCode: got %d, want 2", evt.EventCode)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for async event callback")
	}

	// Allow time for session error processing or reconnect attempt.
	time.Sleep(1 * time.Second)

	// After connection drop event, the session should be in error state.
	// Attempting a command should fail.
	cmdCtx, cmdCancel := context.WithTimeout(ctx, 3*time.Second)
	defer cmdCancel()

	cmdErr := sess.TestUnitReady(cmdCtx, 0)
	if cmdErr == nil {
		t.Error("expected error after connection drop async event, got nil")
	}
}
