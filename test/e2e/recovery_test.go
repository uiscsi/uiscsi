//go:build e2e

package e2e_test

import (
	"context"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/rkujawa/uiscsi"
	"github.com/rkujawa/uiscsi/test/lio"
)

// TestErrorRecovery_ConnectionDrop verifies that the uiscsi library
// reconnects after a TCP connection drop (ERL 0 session-level reconnection).
// The test uses `ss -K` to kill the TCP socket from the kernel side, then
// verifies the session recovers and can issue commands again.
func TestErrorRecovery_ConnectionDrop(t *testing.T) {
	lio.RequireRoot(t)
	lio.RequireModules(t)

	tgt, cleanup := lio.Setup(t, lio.Config{
		TargetSuffix: "recovery",
		InitiatorIQN: initiatorIQN,
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr,
		uiscsi.WithTarget(tgt.IQN),
		uiscsi.WithInitiatorName(initiatorIQN),
		uiscsi.WithMaxReconnectAttempts(3),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer sess.Close()

	// Verify session is functional.
	if _, err := sess.Inquiry(ctx, 0); err != nil {
		t.Fatalf("Inquiry before connection drop: %v", err)
	}
	t.Log("Session functional before connection drop")

	// Kill the TCP connection using ss -K.
	// This requires root (which we have) and kills the specific TCP socket
	// matching the target port on loopback.
	killCmd := exec.CommandContext(ctx, "ss", "-K",
		"dst", "127.0.0.1",
		"dport", "=", strconv.Itoa(tgt.Port),
	)
	out, err := killCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ss -K failed: %v\noutput: %s", err, out)
	}
	t.Logf("TCP connection killed via ss -K (port %d)", tgt.Port)

	// Wait briefly for the library to detect the drop.
	time.Sleep(500 * time.Millisecond)

	// After the connection drop, the library should reconnect (ERL 0).
	// Use a generous timeout since reconnect involves re-login.
	reconnCtx, reconnCancel := context.WithTimeout(ctx, 30*time.Second)
	defer reconnCancel()

	// Retry Inquiry -- the first attempt may trigger the reconnect,
	// subsequent attempts should succeed once reconnected.
	var lastErr error
	for attempt := range 10 {
		inq, err := sess.Inquiry(reconnCtx, 0)
		if err == nil {
			t.Logf("Inquiry succeeded after reconnect (attempt %d), VendorID=%q", attempt+1, inq.VendorID)
			return
		}
		lastErr = err
		t.Logf("Inquiry attempt %d after drop: %v", attempt+1, err)
		time.Sleep(time.Duration(500+attempt*500) * time.Millisecond)
	}
	t.Fatalf("session did not recover after connection drop: %v", lastErr)
}
