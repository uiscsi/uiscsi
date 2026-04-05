package conformance_test

import (
	"context"
	"testing"
	"time"

	"github.com/rkujawa/uiscsi"
	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
	testutil "github.com/rkujawa/uiscsi/test"
	"github.com/rkujawa/uiscsi/test/pducapture"
)

// TestERL2_ConnectionReassignment verifies that after a TCP connection drop
// with ERL=2 negotiated, the initiator performs ERL 2 connection replacement:
// dials a new connection with the same ISID+TSIH, performs login, and sends
// Logout(reasonCode=2) on the new connection to signal connection recovery
// for the old CID.
//
// The discriminating signal between ERL 2 and ERL 0 is Logout(reasonCode=2)
// on the wire. ERL 0 reconnect does NOT send Logout(reasonCode=2).
//
// Conformance: SESS-07 (FFP #7.1 -- ERL 2 connection replacement).
func TestERL2_ConnectionReassignment(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.SetNegotiationConfig(testutil.NegotiationConfig{
		ErrorRecoveryLevel: testutil.Uint32Ptr(2),
	})
	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	// Register TMF handler: respond with Function Complete for all TMFs.
	// For TASK REASSIGN (Function=14), after responding to the TMF, also
	// send a Data-In with the reassigned task's NEW ITT. In ERL 2, the
	// target resumes data delivery for reassigned tasks. Since connreplace
	// re-registers the task under a new ITT, we track the mapping from
	// the TMF's ReferencedTaskTag to the new ITT that the initiator
	// allocated (which it sends as a SCSI Command after reassignment).
	tgt.Handle(pdu.OpTaskMgmtReq, func(tc *testutil.TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		tmf := decoded.(*pdu.TaskMgmtReq)
		expCmdSN, maxCmdSN := tgt.Session().Update(tmf.CmdSN, tmf.Header.Immediate)
		resp := &pdu.TaskMgmtResp{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: tmf.InitiatorTaskTag,
			},
			Response: 0x00, // Function Complete
			StatSN:   tc.NextStatSN(),
			ExpCmdSN: expCmdSN,
			MaxCmdSN: maxCmdSN,
		}
		return tc.SendPDU(resp)
	})

	// HandleSCSIFunc:
	// callCount==0: drop connection without responding (simulate TCP failure).
	// callCount>=1: respond normally with Data-In (status + 512 bytes).
	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
		if callCount == 0 {
			// Drop the connection to trigger ERL 2 recovery.
			tc.Close()
			return nil
		}
		// Respond normally.
		din := &pdu.DataIn{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
				DataSegmentLen:   512,
			},
			HasStatus: true,
			Status:    0x00,
			StatSN:    tc.NextStatSN(),
			ExpCmdSN:  expCmdSN,
			MaxCmdSN:  maxCmdSN,
			DataSN:    0,
			Data:      make([]byte, 512),
		}
		return tc.SendPDU(din)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithPDUHook(rec.Hook()),
		uiscsi.WithKeepaliveInterval(30*time.Second),
		uiscsi.WithOperationalOverrides(map[string]string{
			"ErrorRecoveryLevel": "2",
		}),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	// Issue ReadBlocks in a goroutine. It will block while connection
	// drops and ERL 2 recovery happens. After task reassignment, the
	// task loop waits for data from the target; we give it time to
	// observe the ERL 2 protocol signals (Logout reasonCode=2, TMF).
	type readResult struct {
		data []byte
		err  error
	}
	readCh := make(chan readResult, 1)
	go func() {
		data, err := sess.ReadBlocks(ctx, 0, 0, 1, 512)
		readCh <- readResult{data, err}
	}()

	// Wait for the ERL 2 recovery to complete. The recovery happens
	// asynchronously: connection drop -> replaceConnection -> login ->
	// Logout(reasonCode=2) -> TMF TASK REASSIGN. We poll for the
	// Logout(reasonCode=2) signal rather than relying on ReadBlocks
	// completion, since task data resumption after reassignment depends
	// on target-side re-send behavior.
	var foundReasonCode2 bool
	deadline := time.After(10 * time.Second)
	for !foundReasonCode2 {
		select {
		case <-deadline:
			t.Fatal("SESS-07: timed out waiting for Logout(reasonCode=2)")
		case <-time.After(100 * time.Millisecond):
		}
		logouts := rec.Sent(pdu.OpLogoutReq)
		for _, cap := range logouts {
			lr, ok := cap.Decoded.(*pdu.LogoutReq)
			if ok && lr.ReasonCode == 2 {
				foundReasonCode2 = true
				break
			}
		}
	}

	// KEY DISCRIMINATING ASSERTION: Logout(reasonCode=2) was captured.
	// ERL 0 reconnect does NOT send Logout(reasonCode=2). This proves
	// the ERL 2 path was taken.
	if !foundReasonCode2 {
		t.Fatal("SESS-07: no Logout(reasonCode=2) on wire -- ERL 2 path was not taken; likely fell back to ERL 0")
	}

	// Note: Login PDUs are exchanged over the raw transport.Conn before
	// session pumps start, so they are not captured by WithPDUHook.
	// The second login (reconnect) is confirmed by the successful
	// Logout(reasonCode=2) which can only happen after a successful
	// login on the new connection.

	// The ReadBlocks may or may not complete depending on whether the
	// target resends data after task reassignment. For SESS-07, the
	// key assertion is the Logout(reasonCode=2) signal. Cancel context
	// to unblock any pending operations.
	cancel()
	<-readCh
}

// TestERL2_TaskReassign verifies that after ERL 2 connection replacement,
// the initiator sends TMF TASK REASSIGN (Function=14) for each in-flight
// task, with ReferencedTaskTag set to the original ITT of the interrupted
// SCSI command.
//
// Conformance: SESS-08 (FFP #19.5 -- ERL 2 task reassignment).
func TestERL2_TaskReassign(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.SetNegotiationConfig(testutil.NegotiationConfig{
		ErrorRecoveryLevel: testutil.Uint32Ptr(2),
	})
	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	// Track the original ITT from the first SCSI command, and whether
	// a TASK REASSIGN TMF was received with the correct ReferencedTaskTag.
	type tmfCapture struct {
		function          uint8
		referencedTaskTag uint32
	}
	tmfCh := make(chan tmfCapture, 4)

	// Register TMF handler: record TMF details and respond Function Complete.
	tgt.Handle(pdu.OpTaskMgmtReq, func(tc *testutil.TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		tmf := decoded.(*pdu.TaskMgmtReq)
		expCmdSN, maxCmdSN := tgt.Session().Update(tmf.CmdSN, tmf.Header.Immediate)

		// Send capture to test goroutine (non-blocking).
		select {
		case tmfCh <- tmfCapture{function: tmf.Function, referencedTaskTag: tmf.ReferencedTaskTag}:
		default:
		}

		resp := &pdu.TaskMgmtResp{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: tmf.InitiatorTaskTag,
			},
			Response: 0x00, // Function Complete
			StatSN:   tc.NextStatSN(),
			ExpCmdSN: expCmdSN,
			MaxCmdSN: maxCmdSN,
		}
		return tc.SendPDU(resp)
	})

	// HandleSCSIFunc:
	// callCount==0: record the original ITT, then close connection.
	// callCount>=1: respond normally.
	var originalITT uint32
	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
		if callCount == 0 {
			originalITT = cmd.InitiatorTaskTag
			tc.Close()
			return nil
		}
		din := &pdu.DataIn{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
				DataSegmentLen:   512,
			},
			HasStatus: true,
			Status:    0x00,
			StatSN:    tc.NextStatSN(),
			ExpCmdSN:  expCmdSN,
			MaxCmdSN:  maxCmdSN,
			DataSN:    0,
			Data:      make([]byte, 512),
		}
		return tc.SendPDU(din)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithPDUHook(rec.Hook()),
		uiscsi.WithKeepaliveInterval(30*time.Second),
		uiscsi.WithOperationalOverrides(map[string]string{
			"ErrorRecoveryLevel": "2",
		}),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	// Issue ReadBlocks in a goroutine to create an in-flight task.
	type readResult struct {
		data []byte
		err  error
	}
	readCh := make(chan readResult, 1)
	go func() {
		data, err := sess.ReadBlocks(ctx, 0, 0, 1, 512)
		readCh <- readResult{data, err}
	}()

	// Wait for the TMF TASK REASSIGN to be received by the target.
	var foundTaskReassign bool
	var tmfRefTag uint32
	deadline := time.After(10 * time.Second)
	for !foundTaskReassign {
		select {
		case <-deadline:
			t.Fatal("SESS-08: timed out waiting for TMF TASK REASSIGN")
		case cap := <-tmfCh:
			if cap.function == 14 { // TASK REASSIGN
				foundTaskReassign = true
				tmfRefTag = cap.referencedTaskTag
			}
		case <-time.After(100 * time.Millisecond):
		}
	}

	// Verify TMF TASK REASSIGN was sent (Function=14).
	if !foundTaskReassign {
		t.Fatal("SESS-08: no TMF TASK REASSIGN (Function=14) received by target")
	}

	// Verify ReferencedTaskTag matches the original SCSI Command ITT.
	if tmfRefTag != originalITT {
		t.Fatalf("SESS-08: TMF ReferencedTaskTag=0x%08X, want original ITT=0x%08X",
			tmfRefTag, originalITT)
	}

	// Also verify via pducapture: at least one sent TMF has Function=14.
	tmfPDUs := rec.Sent(pdu.OpTaskMgmtReq)
	var foundInCapture bool
	for _, cap := range tmfPDUs {
		tmf, ok := cap.Decoded.(*pdu.TaskMgmtReq)
		if ok && tmf.Function == 14 {
			foundInCapture = true
			// Cross-check ReferencedTaskTag from pducapture.
			if tmf.ReferencedTaskTag != originalITT {
				t.Fatalf("SESS-08: pducapture TMF ReferencedTaskTag=0x%08X, want 0x%08X",
					tmf.ReferencedTaskTag, originalITT)
			}
			break
		}
	}
	if !foundInCapture {
		t.Fatal("SESS-08: TMF TASK REASSIGN (Function=14) not found in pducapture sent PDUs")
	}

	// Clean up: cancel context to unblock pending operations.
	cancel()
	<-readCh
}
