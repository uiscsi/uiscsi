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

// TestSession_LogoutAfterAsyncEvent1 verifies that the initiator sends a
// LogoutReq within the Parameter3 deadline after receiving an AsyncMsg with
// event code 1 (target requests logout). Per RFC 7143 Section 11.9.1 and
// FFP #14.1, the initiator MUST complete logout within Parameter3 seconds.
// Conformance: SESS-01
func TestSession_LogoutAfterAsyncEvent1(t *testing.T) {
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

	// HandleSCSIFunc: on first call, send SCSI response then inject AsyncMsg code 1.
	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)

		// Always send SCSI response first.
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

		// On first call, inject AsyncMsg code 1 with Parameter3=2 (2-second deadline).
		if callCount == 0 {
			if err := tgt.SendAsyncMsg(tc, 1, testutil.AsyncParams{Parameter3: 2}); err != nil {
				return err
			}
			close(asyncInjected)
		}
		return nil
	})

	// Track async events received by the initiator.
	asyncReceived := make(chan struct{}, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithPDUHook(rec.Hook()),
		uiscsi.WithKeepaliveInterval(30*time.Second),
		uiscsi.WithAsyncHandler(func(_ context.Context, evt uiscsi.AsyncEvent) {
			if evt.EventCode == 1 {
				select {
				case asyncReceived <- struct{}{}:
				default:
				}
			}
		}),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	// Do NOT use t.Cleanup(sess.Close) here — the session will be closed by
	// the async logout handler, and calling Close again is harmless but we
	// want to test the behavioral side effect.

	// First SCSI command triggers the AsyncMsg injection.
	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady: %v", err)
	}

	// Wait for async injection to complete.
	select {
	case <-asyncInjected:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for async injection")
	}

	// Wait for the async handler callback to fire.
	select {
	case <-asyncReceived:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for async event callback")
	}

	// Wait up to 3 seconds (Parameter3=2s + 1s buffer) for the LogoutReq to appear.
	deadline := time.After(3 * time.Second)
	var logoutReqs []pducapture.CapturedPDU
	for {
		logoutReqs = rec.Sent(pdu.OpLogoutReq)
		if len(logoutReqs) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("no LogoutReq sent within Parameter3 deadline + buffer")
		case <-time.After(100 * time.Millisecond):
		}
	}

	// Validate the LogoutReq wire fields.
	logout := logoutReqs[0].Decoded.(*pdu.LogoutReq)

	// ReasonCode = 0 (close session) per handleTargetRequestedLogout.
	if logout.ReasonCode != 0 {
		t.Errorf("ReasonCode: got %d, want 0 (close session)", logout.ReasonCode)
	}

	// Final = true.
	if !logout.Header.Final {
		t.Error("Final: got false, want true")
	}

	// ITT is valid (non-reserved).
	if logout.Header.InitiatorTaskTag == 0xFFFFFFFF {
		t.Error("ITT: got 0xFFFFFFFF, want valid task tag")
	}

	// DataSegmentLen = 0.
	if logout.Header.DataSegmentLen != 0 {
		t.Errorf("DataSegmentLen: got %d, want 0", logout.Header.DataSegmentLen)
	}

	// TotalAHSLength = 0.
	if logout.Header.TotalAHSLength != 0 {
		t.Errorf("TotalAHSLength: got %d, want 0", logout.Header.TotalAHSLength)
	}

	// CmdSN is valid (non-zero, incremented from SCSI command).
	if logout.CmdSN == 0 {
		t.Error("CmdSN: got 0, want non-zero")
	}

	// ExpStatSN is present.
	if logout.ExpStatSN == 0 {
		t.Error("ExpStatSN: got 0, want non-zero")
	}

	// Behavioral side effect: session should be closed after logout.
	// Wait a bit for the session teardown to complete.
	time.Sleep(500 * time.Millisecond)

	// Attempting a command on the closed session should fail.
	cmdErr := sess.TestUnitReady(ctx, 0)
	if cmdErr == nil {
		t.Error("expected error after async-triggered logout, got nil")
	}
}

// TestSession_CleanLogout verifies the standard voluntary logout exchange.
// The initiator sends LogoutReq with correct fields and receives LogoutResp.
// Per RFC 7143 Section 11.14/11.15 and FFP #17.1.
// Conformance: SESS-06
func TestSession_CleanLogout(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	// HandleSCSIFunc: standard response.
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
		uiscsi.WithKeepaliveInterval(30*time.Second),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	// Send a SCSI command to establish sequencing baseline.
	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady: %v", err)
	}

	// Record CmdSN from the SCSI command.
	scsiCmds := rec.Sent(pdu.OpSCSICommand)
	if len(scsiCmds) == 0 {
		t.Fatal("no SCSI commands captured")
	}
	lastSCSICmdSN := scsiCmds[len(scsiCmds)-1].Decoded.(*pdu.SCSICommand).CmdSN

	// Perform explicit voluntary logout.
	if err := sess.Logout(ctx); err != nil {
		t.Fatalf("Logout: %v", err)
	}

	// Verify exactly one LogoutReq was sent.
	logoutReqs := rec.Sent(pdu.OpLogoutReq)
	if len(logoutReqs) != 1 {
		t.Fatalf("LogoutReq count: got %d, want 1", len(logoutReqs))
	}

	logout := logoutReqs[0].Decoded.(*pdu.LogoutReq)

	// ReasonCode = 0 (close session).
	if logout.ReasonCode != 0 {
		t.Errorf("ReasonCode: got %d, want 0", logout.ReasonCode)
	}

	// Final = true.
	if !logout.Header.Final {
		t.Error("Final: got false, want true")
	}

	// ITT is valid (non-reserved).
	if logout.Header.InitiatorTaskTag == 0xFFFFFFFF {
		t.Error("ITT: got 0xFFFFFFFF, want valid task tag")
	}

	// CmdSN is valid — should be lastSCSICmdSN+1 since Logout advances CmdSN.
	expectedCmdSN := lastSCSICmdSN + 1
	if logout.CmdSN != expectedCmdSN {
		t.Errorf("CmdSN: got %d, want %d (lastSCSI+1)", logout.CmdSN, expectedCmdSN)
	}

	// ExpStatSN present.
	if logout.ExpStatSN == 0 {
		t.Error("ExpStatSN: got 0, want non-zero")
	}

	// DataSegmentLen = 0.
	if logout.Header.DataSegmentLen != 0 {
		t.Errorf("DataSegmentLen: got %d, want 0", logout.Header.DataSegmentLen)
	}
}
