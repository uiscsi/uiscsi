package conformance_test

import (
	"context"
	"testing"
	"time"

	"github.com/uiscsi/uiscsi"
	testutil "github.com/uiscsi/uiscsi/test"
)

// setupTMFTarget creates a MockTarget with login, logout, and TMF handlers.
func setupTMFTarget(t *testing.T) (*testutil.MockTarget, *uiscsi.Session) {
	t.Helper()
	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()
	tgt.HandleTMF()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	return tgt, sess
}

// TestTMF_AbortTask tests ABORT TASK management function.
// IOL: Task Management - ABORT TASK.
func TestTMF_AbortTask(t *testing.T) {
	_, sess := setupTMFTarget(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// AbortTask with a dummy task tag.
	result, err := sess.AbortTask(ctx, 0x12345678)
	if err != nil {
		t.Fatalf("AbortTask: %v", err)
	}
	if result.Response != 0 {
		t.Fatalf("Response: got %d, want 0 (function complete)", result.Response)
	}
}

// TestTMF_LUNReset tests LUN RESET management function.
// IOL: Task Management - LOGICAL UNIT RESET.
func TestTMF_LUNReset(t *testing.T) {
	_, sess := setupTMFTarget(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := sess.LUNReset(ctx, 0)
	if err != nil {
		t.Fatalf("LUNReset: %v", err)
	}
	if result.Response != 0 {
		t.Fatalf("Response: got %d, want 0 (function complete)", result.Response)
	}
}

// TestTMF_TargetWarmReset tests TARGET WARM RESET management function.
// IOL: Task Management - TARGET WARM RESET.
func TestTMF_TargetWarmReset(t *testing.T) {
	_, sess := setupTMFTarget(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := sess.TargetWarmReset(ctx)
	if err != nil {
		t.Fatalf("TargetWarmReset: %v", err)
	}
	if result.Response != 0 {
		t.Fatalf("Response: got %d, want 0 (function complete)", result.Response)
	}
}
