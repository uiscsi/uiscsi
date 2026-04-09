//go:build e2e

package e2e_test

import (
	"context"
	"testing"
	"time"

	"github.com/uiscsi/uiscsi"
	"github.com/uiscsi/uiscsi/test/lio"
)

// TestCHAP verifies one-way CHAP authentication against a real LIO target
// with ACL-level CHAP credentials configured.
func TestCHAP(t *testing.T) {
	lio.RequireRoot(t)
	lio.RequireModules(t)

	tgt, cleanup := lio.Setup(t, lio.Config{
		TargetSuffix: "chap",
		InitiatorIQN: initiatorIQN,
		CHAPUser:     "e2e-user",
		CHAPPassword: "e2e-secret-pass",
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr,
		uiscsi.WithTarget(tgt.IQN),
		uiscsi.WithInitiatorName(initiatorIQN),
		uiscsi.WithCHAP("e2e-user", "e2e-secret-pass"),
	)
	if err != nil {
		t.Fatalf("Dial with CHAP: %v", err)
	}
	defer sess.Close()

	// Verify session is functional.
	inq, err := sess.Inquiry(ctx, 0)
	if err != nil {
		t.Fatalf("Inquiry after CHAP login: %v", err)
	}
	t.Logf("CHAP login OK, Inquiry VendorID=%q", inq.VendorID)
}

// TestCHAPMutual verifies mutual (bidirectional) CHAP authentication against
// a real LIO target with both initiator and target credentials configured.
func TestCHAPMutual(t *testing.T) {
	lio.RequireRoot(t)
	lio.RequireModules(t)

	tgt, cleanup := lio.Setup(t, lio.Config{
		TargetSuffix:   "chap-mutual",
		InitiatorIQN:   initiatorIQN,
		CHAPUser:       "e2e-user",
		CHAPPassword:   "e2e-secret-pass",
		MutualUser:     "e2e-target",
		MutualPassword: "e2e-target-pass",
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr,
		uiscsi.WithTarget(tgt.IQN),
		uiscsi.WithInitiatorName(initiatorIQN),
		uiscsi.WithMutualCHAP("e2e-user", "e2e-secret-pass", "e2e-target-pass"),
	)
	if err != nil {
		t.Fatalf("Dial with mutual CHAP: %v", err)
	}
	defer sess.Close()

	// Verify session is functional.
	inq, err := sess.Inquiry(ctx, 0)
	if err != nil {
		t.Fatalf("Inquiry after mutual CHAP login: %v", err)
	}
	t.Logf("Mutual CHAP login OK, Inquiry VendorID=%q", inq.VendorID)
}

// TestCHAPBadPassword verifies that CHAP authentication fails with an
// incorrect password, ensuring the target rejects bad credentials.
func TestCHAPBadPassword(t *testing.T) {
	lio.RequireRoot(t)
	lio.RequireModules(t)

	tgt, cleanup := lio.Setup(t, lio.Config{
		TargetSuffix: "chap-bad",
		InitiatorIQN: initiatorIQN,
		CHAPUser:     "e2e-user",
		CHAPPassword: "e2e-secret-pass",
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr,
		uiscsi.WithTarget(tgt.IQN),
		uiscsi.WithInitiatorName(initiatorIQN),
		uiscsi.WithCHAP("e2e-user", "wrong-password-here"),
	)
	if err == nil {
		sess.Close()
		t.Fatal("Dial with wrong CHAP password should have failed, but succeeded")
	}
	t.Logf("CHAP bad password correctly rejected: %v", err)
}
