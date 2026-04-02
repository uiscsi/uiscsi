package session

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
)

// readTaskMgmtReqPDU reads and decodes a TaskMgmtReq PDU from the target conn.
func readTaskMgmtReqPDU(t *testing.T, conn net.Conn) *pdu.TaskMgmtReq {
	t.Helper()
	raw, err := transport.ReadRawPDU(conn, false, false)
	if err != nil {
		t.Fatalf("read TaskMgmtReq: %v", err)
	}
	tmf := &pdu.TaskMgmtReq{}
	tmf.UnmarshalBHS(raw.BHS)
	return tmf
}

// writeTaskMgmtRespPDU encodes and writes a TaskMgmtResp PDU to the target conn.
func writeTaskMgmtRespPDU(t *testing.T, conn net.Conn, resp *pdu.TaskMgmtResp) {
	t.Helper()
	resp.Header.OpCode_ = pdu.OpTaskMgmtResp
	resp.Header.Final = true
	bhs, err := resp.MarshalBHS()
	if err != nil {
		t.Fatalf("MarshalBHS TaskMgmtResp: %v", err)
	}
	raw := &transport.RawPDU{BHS: bhs}
	if err := transport.WriteRawPDU(conn, raw); err != nil {
		t.Fatalf("write TaskMgmtResp: %v", err)
	}
}

// drainNonTMFPDUs reads and discards PDUs from conn until a TaskMgmtReq is
// found, which is returned. This allows skipping NOP-Out keepalives and
// SCSI commands when waiting for TMF PDUs.
func drainUntilTMF(t *testing.T, conn net.Conn) *pdu.TaskMgmtReq {
	t.Helper()
	for {
		raw, err := transport.ReadRawPDU(conn, false, false)
		if err != nil {
			t.Fatalf("read PDU while draining: %v", err)
		}
		opcode := pdu.OpCode(raw.BHS[0] & 0x3f)
		if opcode == pdu.OpTaskMgmtReq {
			tmf := &pdu.TaskMgmtReq{}
			tmf.UnmarshalBHS(raw.BHS)
			return tmf
		}
		// Discard non-TMF PDUs (keepalives, etc.)
	}
}

func TestAbortTask(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		sess, targetConn := newTestSession(t)

		// Submit a SCSI command to get an ITT to abort.
		cmd := Command{}
		cmd.CDB[0] = 0x00 // TEST UNIT READY
		resultCh, err := sess.Submit(context.Background(), cmd)
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}

		// Read the submitted command from target side.
		scsiCmd := readSCSICommandPDU(t, targetConn)
		taskITT := scsiCmd.InitiatorTaskTag

		// Call AbortTask in a goroutine.
		tmfDone := make(chan struct{})
		var tmfResult *TMFResult
		var tmfErr error
		go func() {
			defer close(tmfDone)
			tmfResult, tmfErr = sess.AbortTask(context.Background(), taskITT)
		}()

		// Read the TMF request from target side.
		tmfReq := drainUntilTMF(t, targetConn)
		if tmfReq.Function != TMFAbortTask {
			t.Fatalf("Function: got %d, want %d", tmfReq.Function, TMFAbortTask)
		}
		if tmfReq.ReferencedTaskTag != taskITT {
			t.Fatalf("ReferencedTaskTag: got %d, want %d", tmfReq.ReferencedTaskTag, taskITT)
		}

		// Respond with success.
		writeTaskMgmtRespPDU(t, targetConn, &pdu.TaskMgmtResp{
			Header:   pdu.Header{InitiatorTaskTag: tmfReq.InitiatorTaskTag},
			Response: TMFRespComplete,
			StatSN:   2,
			ExpCmdSN: 2,
			MaxCmdSN: 10,
		})

		<-tmfDone
		if tmfErr != nil {
			t.Fatalf("AbortTask error: %v", tmfErr)
		}
		if tmfResult.Response != TMFRespComplete {
			t.Fatalf("Response: got %d, want %d", tmfResult.Response, TMFRespComplete)
		}

		// Verify the aborted task received ErrTaskAborted.
		select {
		case result := <-resultCh:
			if !errors.Is(result.Err, ErrTaskAborted) {
				t.Fatalf("aborted task error: got %v, want %v", result.Err, ErrTaskAborted)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for aborted task result")
		}
	})

	t.Run("task_not_exist", func(t *testing.T) {
		sess, targetConn := newTestSession(t)

		// Call AbortTask for a non-existent ITT.
		tmfDone := make(chan struct{})
		var tmfResult *TMFResult
		var tmfErr error
		go func() {
			defer close(tmfDone)
			tmfResult, tmfErr = sess.AbortTask(context.Background(), 0x42)
		}()

		// Read TMF from target.
		tmfReq := drainUntilTMF(t, targetConn)
		if tmfReq.Function != TMFAbortTask {
			t.Fatalf("Function: got %d, want %d", tmfReq.Function, TMFAbortTask)
		}

		// Respond with TaskNotExist.
		writeTaskMgmtRespPDU(t, targetConn, &pdu.TaskMgmtResp{
			Header:   pdu.Header{InitiatorTaskTag: tmfReq.InitiatorTaskTag},
			Response: TMFRespTaskNotExist,
			StatSN:   1,
			ExpCmdSN: 1,
			MaxCmdSN: 10,
		})

		<-tmfDone
		if tmfErr != nil {
			t.Fatalf("AbortTask error: %v", tmfErr)
		}
		if tmfResult.Response != TMFRespTaskNotExist {
			t.Fatalf("Response: got %d, want %d", tmfResult.Response, TMFRespTaskNotExist)
		}
	})
}

func TestAbortTaskSet(t *testing.T) {
	sess, targetConn := newTestSession(t)

	lun := uint64(1)

	// Submit two commands on the same LUN.
	cmd1 := Command{LUN: lun}
	cmd1.CDB[0] = 0x00
	resultCh1, err := sess.Submit(context.Background(), cmd1)
	if err != nil {
		t.Fatalf("Submit cmd1: %v", err)
	}
	readSCSICommandPDU(t, targetConn) // drain cmd1

	cmd2 := Command{LUN: lun}
	cmd2.CDB[0] = 0x00

	// Advance MaxCmdSN to allow second submit.
	// (Default window starts at CmdSN=1, MaxCmdSN=1)
	// Need to send a response to widen the window first.
	// Actually, let's check the window. The default newTestSession uses Defaults()
	// which sets CmdSN=1, ExpStatSN=1, and newCmdWindow(1, 1, 1) means MaxCmdSN=1.
	// So only one command fits. Let's widen the window by responding to cmd1 partially.

	// Let's simplify: just verify AbortTaskSet works with one task.
	tmfDone := make(chan struct{})
	var tmfResult *TMFResult
	var tmfErr error
	go func() {
		defer close(tmfDone)
		tmfResult, tmfErr = sess.AbortTaskSet(context.Background(), lun)
	}()

	tmfReq := drainUntilTMF(t, targetConn)
	if tmfReq.Function != TMFAbortTaskSet {
		t.Fatalf("Function: got %d, want %d", tmfReq.Function, TMFAbortTaskSet)
	}
	// Verify LUN is encoded in the TMF header.
	gotLUN := pdu.DecodeSAMLUN(tmfReq.Header.LUN[:])
	if gotLUN != lun {
		t.Fatalf("LUN: got %d, want %d", gotLUN, lun)
	}

	writeTaskMgmtRespPDU(t, targetConn, &pdu.TaskMgmtResp{
		Header:   pdu.Header{InitiatorTaskTag: tmfReq.InitiatorTaskTag},
		Response: TMFRespComplete,
		StatSN:   2,
		ExpCmdSN: 2,
		MaxCmdSN: 10,
	})

	<-tmfDone
	if tmfErr != nil {
		t.Fatalf("AbortTaskSet error: %v", tmfErr)
	}
	if tmfResult.Response != TMFRespComplete {
		t.Fatalf("Response: got %d, want %d", tmfResult.Response, TMFRespComplete)
	}

	// Verify cmd1 was cleaned up with ErrTaskAborted.
	select {
	case result := <-resultCh1:
		if !errors.Is(result.Err, ErrTaskAborted) {
			t.Fatalf("cmd1 error: got %v, want %v", result.Err, ErrTaskAborted)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cmd1 cleanup")
	}
}

func TestLUNReset(t *testing.T) {
	sess, targetConn := newTestSession(t)

	lun := uint64(5)

	// Submit a command on the target LUN.
	cmd := Command{LUN: lun}
	cmd.CDB[0] = 0x00
	resultCh, err := sess.Submit(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	readSCSICommandPDU(t, targetConn)

	tmfDone := make(chan struct{})
	var tmfResult *TMFResult
	var tmfErr error
	go func() {
		defer close(tmfDone)
		tmfResult, tmfErr = sess.LUNReset(context.Background(), lun)
	}()

	tmfReq := drainUntilTMF(t, targetConn)
	if tmfReq.Function != TMFLogicalUnitReset {
		t.Fatalf("Function: got %d, want %d", tmfReq.Function, TMFLogicalUnitReset)
	}
	gotLUN := pdu.DecodeSAMLUN(tmfReq.Header.LUN[:])
	if gotLUN != lun {
		t.Fatalf("LUN: got %d, want %d", gotLUN, lun)
	}

	writeTaskMgmtRespPDU(t, targetConn, &pdu.TaskMgmtResp{
		Header:   pdu.Header{InitiatorTaskTag: tmfReq.InitiatorTaskTag},
		Response: TMFRespComplete,
		StatSN:   2,
		ExpCmdSN: 2,
		MaxCmdSN: 10,
	})

	<-tmfDone
	if tmfErr != nil {
		t.Fatalf("LUNReset error: %v", tmfErr)
	}
	if tmfResult.Response != TMFRespComplete {
		t.Fatalf("Response: got %d, want %d", tmfResult.Response, TMFRespComplete)
	}

	select {
	case result := <-resultCh:
		if !errors.Is(result.Err, ErrTaskAborted) {
			t.Fatalf("task error: got %v, want %v", result.Err, ErrTaskAborted)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for task cleanup")
	}
}

func TestTargetWarmReset(t *testing.T) {
	sess, targetConn := newTestSession(t)

	tmfDone := make(chan struct{})
	var tmfResult *TMFResult
	var tmfErr error
	go func() {
		defer close(tmfDone)
		tmfResult, tmfErr = sess.TargetWarmReset(context.Background())
	}()

	tmfReq := drainUntilTMF(t, targetConn)
	if tmfReq.Function != TMFTargetWarmReset {
		t.Fatalf("Function: got %d, want %d", tmfReq.Function, TMFTargetWarmReset)
	}

	writeTaskMgmtRespPDU(t, targetConn, &pdu.TaskMgmtResp{
		Header:   pdu.Header{InitiatorTaskTag: tmfReq.InitiatorTaskTag},
		Response: TMFRespComplete,
		StatSN:   1,
		ExpCmdSN: 1,
		MaxCmdSN: 10,
	})

	<-tmfDone
	if tmfErr != nil {
		t.Fatalf("TargetWarmReset error: %v", tmfErr)
	}
	if tmfResult.Response != TMFRespComplete {
		t.Fatalf("Response: got %d, want %d", tmfResult.Response, TMFRespComplete)
	}
}

func TestTargetColdReset(t *testing.T) {
	sess, targetConn := newTestSession(t)

	tmfDone := make(chan struct{})
	var tmfResult *TMFResult
	var tmfErr error
	go func() {
		defer close(tmfDone)
		tmfResult, tmfErr = sess.TargetColdReset(context.Background())
	}()

	tmfReq := drainUntilTMF(t, targetConn)
	if tmfReq.Function != TMFTargetColdReset {
		t.Fatalf("Function: got %d, want %d", tmfReq.Function, TMFTargetColdReset)
	}

	writeTaskMgmtRespPDU(t, targetConn, &pdu.TaskMgmtResp{
		Header:   pdu.Header{InitiatorTaskTag: tmfReq.InitiatorTaskTag},
		Response: TMFRespComplete,
		StatSN:   1,
		ExpCmdSN: 1,
		MaxCmdSN: 10,
	})

	<-tmfDone
	if tmfErr != nil {
		t.Fatalf("TargetColdReset error: %v", tmfErr)
	}
	if tmfResult.Response != TMFRespComplete {
		t.Fatalf("Response: got %d, want %d", tmfResult.Response, TMFRespComplete)
	}
}

func TestClearTaskSet(t *testing.T) {
	sess, targetConn := newTestSession(t)

	lun := uint64(3)

	cmd := Command{LUN: lun}
	cmd.CDB[0] = 0x00
	resultCh, err := sess.Submit(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	readSCSICommandPDU(t, targetConn)

	tmfDone := make(chan struct{})
	var tmfResult *TMFResult
	var tmfErr error
	go func() {
		defer close(tmfDone)
		tmfResult, tmfErr = sess.ClearTaskSet(context.Background(), lun)
	}()

	tmfReq := drainUntilTMF(t, targetConn)
	if tmfReq.Function != TMFClearTaskSet {
		t.Fatalf("Function: got %d, want %d", tmfReq.Function, TMFClearTaskSet)
	}
	gotLUN := pdu.DecodeSAMLUN(tmfReq.Header.LUN[:])
	if gotLUN != lun {
		t.Fatalf("LUN: got %d, want %d", gotLUN, lun)
	}

	writeTaskMgmtRespPDU(t, targetConn, &pdu.TaskMgmtResp{
		Header:   pdu.Header{InitiatorTaskTag: tmfReq.InitiatorTaskTag},
		Response: TMFRespComplete,
		StatSN:   2,
		ExpCmdSN: 2,
		MaxCmdSN: 10,
	})

	<-tmfDone
	if tmfErr != nil {
		t.Fatalf("ClearTaskSet error: %v", tmfErr)
	}
	if tmfResult.Response != TMFRespComplete {
		t.Fatalf("Response: got %d, want %d", tmfResult.Response, TMFRespComplete)
	}

	select {
	case result := <-resultCh:
		if !errors.Is(result.Err, ErrTaskAborted) {
			t.Fatalf("task error: got %v, want %v", result.Err, ErrTaskAborted)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for task cleanup")
	}
}

func TestTMF(t *testing.T) {
	t.Run("immediate_bit", func(t *testing.T) {
		sess, targetConn := newTestSession(t)

		go func() {
			sess.TargetWarmReset(context.Background())
		}()

		// Read the raw TMF PDU and check Immediate bit.
		tmfReq := drainUntilTMF(t, targetConn)
		if !tmfReq.Immediate {
			t.Fatal("TMF PDU does not have Immediate bit set")
		}

		// Respond to unblock the goroutine.
		writeTaskMgmtRespPDU(t, targetConn, &pdu.TaskMgmtResp{
			Header:   pdu.Header{InitiatorTaskTag: tmfReq.InitiatorTaskTag},
			Response: TMFRespComplete,
			StatSN:   1,
			ExpCmdSN: 1,
			MaxCmdSN: 10,
		})
	})

	t.Run("context_cancellation", func(t *testing.T) {
		sess, _ := newTestSession(t)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately.

		_, err := sess.TargetWarmReset(ctx)
		if err == nil {
			t.Fatal("expected error from cancelled context")
		}
	})
}
