//go:build e2e

package e2e_test

import (
	"context"
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

// Note on AbortTask: AbortTask requires an active task tag to abort.
// In synchronous tests, all tasks complete before we can abort them.
// Testing AbortTask meaningfully requires a long-running concurrent command.
// The LUNReset test above validates the TMF path end-to-end. AbortTask
// uses the same TMF infrastructure internally (same PDU type, same
// response handling), so LUNReset coverage provides confidence in the
// TMF mechanism.
