package conformance_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/uiscsi/uiscsi"
	"github.com/uiscsi/uiscsi/internal/login"
	"github.com/uiscsi/uiscsi/internal/pdu"
	testutil "github.com/uiscsi/uiscsi/test"
	"github.com/uiscsi/uiscsi/test/pducapture"
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

// TestAsync_SessionDrop verifies that after receiving AsyncMsg code 3
// (session drop notification), the session is terminated and subsequent
// commands return an error.
// Conformance: ASYNC-03 (FFP #20.3).
func TestAsync_SessionDrop(t *testing.T) {
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
			// AsyncEvent 3: session drop. Time2Wait=0, Time2Retain=0.
			if err := tgt.SendAsyncMsg(tc, 3, testutil.AsyncParams{
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

	// Verify async handler callback fires with EventCode=3.
	select {
	case evt := <-asyncReceived:
		if evt.EventCode != 3 {
			t.Fatalf("async EventCode: got %d, want 3", evt.EventCode)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for async event callback")
	}

	// Allow time for session cancellation.
	time.Sleep(500 * time.Millisecond)

	// Session should be terminated. Commands must fail.
	cmdCtx, cmdCancel := context.WithTimeout(ctx, 3*time.Second)
	defer cmdCancel()

	cmdErr := sess.TestUnitReady(cmdCtx, 0)
	if cmdErr == nil {
		t.Error("expected error after session drop async event, got nil")
	}
}

// TestAsync_NegotiationRequest verifies that after receiving AsyncMsg code 4
// (negotiation request), the initiator sends a TextReq within Parameter3
// seconds containing operational parameters (MaxRecvDataSegmentLength,
// MaxBurstLength, FirstBurstLength). After successful renegotiation, the
// session remains operational.
// Conformance: ASYNC-04 (FFP #20.4).
func TestAsync_NegotiationRequest(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()
	tgt.HandleText()

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
			// AsyncEvent 4: negotiation request. 3-second deadline.
			if err := tgt.SendAsyncMsg(tc, 4, testutil.AsyncParams{Parameter3: 3}); err != nil {
				return err
			}
			close(asyncInjected)
		}
		return nil
	})

	asyncReceived := make(chan uiscsi.AsyncEvent, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
	t.Cleanup(func() { sess.Close() })

	// Record CmdSN from the initial SCSI command.
	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady (initial): %v", err)
	}

	scsiCmds := rec.Sent(pdu.OpSCSICommand)
	if len(scsiCmds) == 0 {
		t.Fatal("no SCSI commands captured")
	}
	initialCmdSN := scsiCmds[0].Decoded.(*pdu.SCSICommand).CmdSN

	select {
	case <-asyncInjected:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for async injection")
	}

	// Verify async handler callback fires with EventCode=4.
	select {
	case evt := <-asyncReceived:
		if evt.EventCode != 4 {
			t.Fatalf("async EventCode: got %d, want 4", evt.EventCode)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for async event callback")
	}

	// Wait for TextReq to appear on wire (within Parameter3=3s + buffer).
	deadline := time.After(5 * time.Second)
	var textReqs []pducapture.CapturedPDU
	for {
		textReqs = rec.Sent(pdu.OpTextReq)
		if len(textReqs) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("no TextReq sent within Parameter3 deadline + buffer")
		case <-time.After(100 * time.Millisecond):
		}
	}

	// Validate TextReq wire fields.
	textReq := textReqs[0].Decoded.(*pdu.TextReq)

	// Final = true (single text exchange).
	if !textReq.Header.Final {
		t.Error("TextReq Final: got false, want true")
	}

	// ITT must be valid (not reserved).
	if textReq.Header.InitiatorTaskTag == 0xFFFFFFFF {
		t.Error("TextReq ITT: got 0xFFFFFFFF, want valid task tag")
	}

	// TTT = 0xFFFFFFFF (initial request, not continuation).
	if textReq.TargetTransferTag != 0xFFFFFFFF {
		t.Errorf("TextReq TTT: got 0x%08X, want 0xFFFFFFFF", textReq.TargetTransferTag)
	}

	// CmdSN must be valid (incremented from initial SCSI command).
	if textReq.CmdSN <= initialCmdSN {
		t.Errorf("TextReq CmdSN: got %d, want > %d (initial)", textReq.CmdSN, initialCmdSN)
	}

	// Verify the data segment carries renegotiation parameters.
	// TextReq raw bytes: BHS (48) + data segment.
	rawData := textReqs[0].Raw
	if len(rawData) <= pdu.BHSLength {
		t.Fatal("TextReq has no data segment")
	}
	dataSegment := rawData[pdu.BHSLength:]
	kvs := login.DecodeTextKV(dataSegment)
	kvMap := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		kvMap[kv.Key] = kv.Value
	}

	requiredKeys := []string{"MaxRecvDataSegmentLength", "MaxBurstLength", "FirstBurstLength"}
	for _, key := range requiredKeys {
		if _, ok := kvMap[key]; !ok {
			t.Errorf("TextReq data segment missing key %q; got keys: %s",
				key, formatKeys(kvMap))
		}
	}

	// After renegotiation, session should remain operational.
	// Allow renegotiation to complete.
	time.Sleep(1 * time.Second)

	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady (post-renegotiation): %v", err)
	}
}

// formatKeys returns a comma-separated list of map keys for diagnostic output.
func formatKeys(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}
