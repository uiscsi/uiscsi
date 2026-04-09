// Package conformance_test contains IOL-inspired conformance tests
// exercising the public uiscsi API against an in-process mock target.
// All tests run automatically without manual SAN setup.
package conformance_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/uiscsi/uiscsi"
	testutil "github.com/uiscsi/uiscsi/test"
	"github.com/uiscsi/uiscsi/internal/login"
	"github.com/uiscsi/uiscsi/internal/pdu"
	"github.com/uiscsi/uiscsi/internal/transport"
)

// setupTarget creates a MockTarget with login and logout handlers.
func setupTarget(t *testing.T) *testutil.MockTarget {
	t.Helper()
	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })
	tgt.HandleLogin()
	tgt.HandleLogout()
	return tgt
}

// TestLogin_AuthNone verifies basic login with AuthMethod=None.
// IOL: Login Phase - Normal Login with No Authentication.
func TestLogin_AuthNone(t *testing.T) {
	tgt := setupTarget(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer sess.Close()

	// Session established successfully -- login passed.
}

// TestLogin_WithTarget verifies login with an explicit target IQN.
// IOL: Login Phase - Login with TargetName.
func TestLogin_WithTarget(t *testing.T) {
	tgt := setupTarget(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithTarget("iqn.2026-03.com.test:target"),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer sess.Close()
}

// TestLogin_InvalidAddress verifies that dialing an unreachable address
// returns a *TransportError with Op="dial".
func TestLogin_InvalidAddress(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := uiscsi.Dial(ctx, "192.0.2.1:59999") // RFC 5737 TEST-NET
	if err == nil {
		t.Fatal("expected error for unreachable address")
	}

	var te *uiscsi.TransportError
	if !errors.As(err, &te) {
		t.Fatalf("expected *TransportError, got %T: %v", err, err)
	}
	if te.Op != "dial" {
		t.Fatalf("Op: got %q, want %q", te.Op, "dial")
	}
}

// TestLogin_ContextCancel verifies that a canceled context returns an error.
func TestLogin_ContextCancel(t *testing.T) {
	tgt := setupTarget(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := uiscsi.Dial(ctx, tgt.Addr())
	if err == nil {
		t.Fatal("expected error for canceled context")
	}
}

// TestLogin_MultipleSessions verifies that two sessions to the same target
// both succeed and both close cleanly.
func TestLogin_MultipleSessions(t *testing.T) {
	tgt := setupTarget(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess1, err := uiscsi.Dial(ctx, tgt.Addr())
	if err != nil {
		t.Fatalf("Dial(1): %v", err)
	}

	sess2, err := uiscsi.Dial(ctx, tgt.Addr())
	if err != nil {
		t.Fatalf("Dial(2): %v", err)
	}

	if err := sess1.Close(); err != nil {
		t.Fatalf("Close(1): %v", err)
	}
	if err := sess2.Close(); err != nil {
		t.Fatalf("Close(2): %v", err)
	}
}

// TestLogin_NonAuthFailure_IsTransportError verifies that a login failure
// with StatusClass != 2 (non-auth) is wrapped as *TransportError, not *AuthError.
// This is the AUDIT-3 regression test.
func TestLogin_NonAuthFailure_IsTransportError(t *testing.T) {
	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	// Register a login handler that rejects with StatusClass=3 (target error),
	// NOT StatusClass=2 (auth failure).
	tgt.Handle(pdu.OpLoginReq, func(tc *testutil.TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		req := decoded.(*pdu.LoginReq)
		resp := &pdu.LoginResp{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: req.InitiatorTaskTag,
			},
			CSG:           req.CSG,
			NSG:           req.CSG,
			VersionMax:    0x00,
			VersionActive: 0x00,
			ISID:          req.ISID,
			StatSN:        tc.NextStatSN(),
			ExpCmdSN:      req.CmdSN,
			MaxCmdSN:      req.CmdSN,
			StatusClass:   3, // Target error, NOT auth
			StatusDetail:  0,
			Data:          login.EncodeTextKV(nil),
		}
		return tc.SendPDU(resp)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = uiscsi.Dial(ctx, tgt.Addr())
	if err == nil {
		t.Fatal("expected error for login rejection with StatusClass=3")
	}

	// Must be TransportError, NOT AuthError.
	var te *uiscsi.TransportError
	if !errors.As(err, &te) {
		t.Fatalf("expected *TransportError, got %T: %v", err, err)
	}
	if te.Op != "login" {
		t.Fatalf("TransportError.Op = %q, want %q", te.Op, "login")
	}

	// Must NOT be AuthError.
	var ae *uiscsi.AuthError
	if errors.As(err, &ae) {
		t.Fatalf("non-auth login failure should NOT be *AuthError, but got: %v", ae)
	}
}

// TestExecute_CDBTooLong verifies that Execute rejects CDBs exceeding 16 bytes
// before any network I/O. This is the AUDIT-10 regression test.
func TestExecute_CDBTooLong(t *testing.T) {
	tgt := setupTarget(t)
	tgt.HandleNOPOut()
	tgt.HandleSCSIRead(0, make([]byte, 512))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer sess.Close()

	// 17-byte CDB should be rejected immediately.
	cdb := make([]byte, 17)
	_, err = sess.Execute(ctx, 0, cdb)
	if err == nil {
		t.Fatal("expected error for CDB > 16 bytes")
	}
	if !strings.Contains(err.Error(), "exceeds maximum 16 bytes") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// Empty CDB should also be rejected.
	_, err = sess.Execute(ctx, 0, nil)
	if err == nil {
		t.Fatal("expected error for empty CDB")
	}
	if !strings.Contains(err.Error(), "empty CDB") {
		t.Fatalf("unexpected error message: %v", err)
	}
}
