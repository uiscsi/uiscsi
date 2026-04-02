//go:build e2e

package e2e_test

import (
	"context"
	"encoding/binary"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rkujawa/uiscsi"
	"github.com/rkujawa/uiscsi/test/lio"
)

// TestTMF_LUNReset verifies that a LUN Reset task management function
// executes successfully against a real LIO target and that the session
// remains functional afterward.
func TestTMF_LUNReset(t *testing.T) {
	lio.RequireRoot(t)
	lio.RequireModules(t)

	tgt, cleanup := lio.Setup(t, lio.Config{
		TargetSuffix: "tmf",
		InitiatorIQN: initiatorIQN,
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr,
		uiscsi.WithTarget(tgt.IQN),
		uiscsi.WithInitiatorName(initiatorIQN),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer sess.Close()

	// Verify session is functional before TMF.
	if _, err := sess.Inquiry(ctx, 0); err != nil {
		t.Fatalf("Inquiry before LUNReset: %v", err)
	}

	// Send LUN Reset.
	result, err := sess.LUNReset(ctx, 0)
	if err != nil {
		t.Fatalf("LUNReset: %v", err)
	}
	t.Logf("LUNReset response code: %d", result.Response)

	// TMF response 0 = "Function Complete" per RFC 7143 Section 11.6.1.
	if result.Response != 0 {
		t.Errorf("LUNReset response: got %d, want 0 (Function Complete)", result.Response)
	}

	// Verify session still works after LUN Reset.
	inq, err := sess.Inquiry(ctx, 0)
	if err != nil {
		t.Fatalf("Inquiry after LUNReset: %v", err)
	}
	t.Logf("Session functional after LUNReset, VendorID=%q", inq.VendorID)
}

// TestTMF_AbortTask verifies that ABORT TASK TMF can be sent during a
// concurrent long-running SCSI command. It captures the Initiator Task Tag
// of an in-flight SCSI command via the PDU hook and then sends an AbortTask
// TMF targeting that ITT. Both "Function Complete" (0) and "Task Does Not
// Exist" (5) are accepted since the command may complete before the abort
// arrives at the target.
func TestTMF_AbortTask(t *testing.T) {
	lio.RequireRoot(t)
	lio.RequireModules(t)

	tgt, cleanup := lio.Setup(t, lio.Config{
		TargetSuffix: "tmf-abort",
		InitiatorIQN: initiatorIQN,
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Set up ITT capture via PDU hook. We capture the ITT of the first
	// outgoing SCSI Command PDU (opcode 0x01). sync.Once + channel
	// ensures the hook fires exactly once and the main goroutine can
	// synchronize on the capture.
	var capturedITT uint32
	var ittOnce sync.Once
	ittCh := make(chan struct{})

	hook := func(dir uiscsi.PDUDirection, data []byte) {
		if dir == uiscsi.PDUSend && len(data) >= 48 {
			opcode := data[0] & 0x3F
			if opcode == 0x01 { // SCSI Command opcode
				itt := binary.BigEndian.Uint32(data[16:20])
				ittOnce.Do(func() {
					atomic.StoreUint32(&capturedITT, itt)
					close(ittCh)
				})
			}
		}
	}

	sess, err := uiscsi.Dial(ctx, tgt.Addr,
		uiscsi.WithTarget(tgt.IQN),
		uiscsi.WithInitiatorName(initiatorIQN),
		uiscsi.WithPDUHook(hook),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer sess.Close()

	// Start a concurrent large read in a goroutine. 256 blocks at 512B
	// each = 128KB to create a long-running command that gives us time
	// to capture the ITT and send an AbortTask.
	errCh := make(chan error, 1)
	go func() {
		_, err := sess.ReadBlocks(ctx, 0, 0, 256, 512)
		errCh <- err
	}()

	// Wait for ITT capture with timeout.
	select {
	case <-ittCh:
		// ITT captured successfully.
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for SCSI Command PDU hook")
	}
	itt := atomic.LoadUint32(&capturedITT)
	t.Logf("Captured SCSI Command ITT: 0x%08X", itt)

	// Send AbortTask targeting the captured ITT.
	result, err := sess.AbortTask(ctx, itt)
	if err != nil {
		t.Fatalf("AbortTask: %v", err)
	}

	// Accept both valid TMF responses per D-06:
	//   Response 0 = "Function Complete"
	//   Response 5 = "Task Does Not Exist" (command may have completed)
	if result.Response != 0 && result.Response != 5 {
		t.Errorf("AbortTask response: got %d, want 0 (Function Complete) or 5 (Task Does Not Exist)", result.Response)
	}
	t.Logf("AbortTask response code: %d", result.Response)

	// Drain the concurrent read goroutine. It may error (aborted) or
	// succeed (completed before abort arrived). Both are acceptable.
	select {
	case readErr := <-errCh:
		t.Logf("Concurrent read result: %v", readErr)
	case <-time.After(10 * time.Second):
		t.Log("Concurrent read still in progress after AbortTask (expected)")
	}

	// Verify session still functional after abort.
	inq, err := sess.Inquiry(ctx, 0)
	if err != nil {
		t.Fatalf("Inquiry after AbortTask: %v", err)
	}
	t.Logf("Session functional after AbortTask, VendorID=%q", inq.VendorID)
}

// TestTMF_TargetWarmReset verifies that TARGET WARM RESET TMF executes
// and the test handles the expected session drop gracefully. After a warm
// reset, the target drops the session, so the test re-establishes a new
// session to confirm the target is still alive.
func TestTMF_TargetWarmReset(t *testing.T) {
	lio.RequireRoot(t)
	lio.RequireModules(t)

	tgt, cleanup := lio.Setup(t, lio.Config{
		TargetSuffix: "tmf-warm",
		InitiatorIQN: initiatorIQN,
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr,
		uiscsi.WithTarget(tgt.IQN),
		uiscsi.WithInitiatorName(initiatorIQN),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer sess.Close()

	// Verify session functional before TMF.
	if _, err := sess.Inquiry(ctx, 0); err != nil {
		t.Fatalf("Inquiry before TargetWarmReset: %v", err)
	}

	// Send TargetWarmReset. Per RFC 7143 Section 11.5.1, the target may
	// drop the session in response to a warm reset, which means the TMF
	// response itself may not arrive cleanly.
	result, err := sess.TargetWarmReset(ctx)
	if err != nil {
		t.Logf("TargetWarmReset returned error (session may have been killed): %v", err)
		// This is expected behavior per RFC 7143 Section 11.5.1.
	} else {
		t.Logf("TargetWarmReset response code: %d", result.Response)
		if result.Response == 5 { // Not Supported
			t.Skip("TARGET WARM RESET not supported by LIO target")
		}
	}

	// Attempt a command on the same session (may fail if reset killed it).
	_, postErr := sess.Inquiry(ctx, 0)
	if postErr != nil {
		t.Logf("Post-reset Inquiry failed (session killed, expected): %v", postErr)
	}

	// Re-establish a new session to verify the target is still alive
	// after the warm reset.
	sess2, err := uiscsi.Dial(ctx, tgt.Addr,
		uiscsi.WithTarget(tgt.IQN),
		uiscsi.WithInitiatorName(initiatorIQN),
	)
	if err != nil {
		t.Fatalf("Re-Dial after warm reset: %v", err)
	}
	defer sess2.Close()

	inq, err := sess2.Inquiry(ctx, 0)
	if err != nil {
		t.Fatalf("Inquiry on new session after warm reset: %v", err)
	}
	t.Logf("Target alive after warm reset, VendorID=%q", inq.VendorID)
}
