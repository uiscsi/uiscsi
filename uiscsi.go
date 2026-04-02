// Package uiscsi provides a pure-userspace iSCSI initiator library.
// It handles TCP connection, login negotiation, and SCSI command transport
// over iSCSI PDUs entirely in userspace.
//
// Basic usage:
//
//	sess, err := uiscsi.Dial(ctx, "192.168.1.100:3260",
//	    uiscsi.WithTarget("iqn.2026-03.com.example:storage"),
//	)
//	if err != nil { ... }
//	defer sess.Close()
//
//	data, err := sess.ReadBlocks(ctx, 0, 0, 1, 512)
package uiscsi

import (
	"context"
	"errors"
	"net"

	"github.com/rkujawa/uiscsi/internal/login"
	"github.com/rkujawa/uiscsi/internal/session"
	"github.com/rkujawa/uiscsi/internal/transport"
)

// normalizePortal ensures addr has an explicit port. If no port is present,
// the iSCSI default port 3260 (RFC 7143 Section 4.1) is appended.
func normalizePortal(addr string) string {
	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		// No port present — append default.
		return net.JoinHostPort(addr, "3260")
	}
	return addr
}

// Dial connects to an iSCSI target at addr, performs login negotiation, and
// returns a Session ready for SCSI commands. The caller must call
// Session.Close when done.
func Dial(ctx context.Context, addr string, opts ...Option) (*Session, error) {
	addr = normalizePortal(addr)
	cfg := &dialConfig{}
	for _, o := range opts {
		o(cfg)
	}

	// Step 1: TCP connect.
	tc, err := transport.Dial(ctx, addr)
	if err != nil {
		return nil, &TransportError{Op: "dial", Err: err}
	}

	// Step 2: iSCSI login.
	params, err := login.Login(ctx, tc, cfg.loginOpts...)
	if err != nil {
		tc.Close()
		// Check for auth failure: LoginError with StatusClass == 2.
		var le *login.LoginError
		if errors.As(err, &le) && le.StatusClass == 2 {
			return nil, wrapAuthError(err)
		}
		return nil, wrapAuthError(err)
	}

	// Step 3: Build session options including reconnect info for ERL 0.
	allSessionOpts := make([]session.SessionOption, 0, len(cfg.sessionOpts)+1)
	allSessionOpts = append(allSessionOpts, session.WithReconnectInfo(addr, cfg.loginOpts...))
	allSessionOpts = append(allSessionOpts, cfg.sessionOpts...)

	// Step 4: Create session.
	s := session.NewSession(tc, *params, allSessionOpts...)

	return &Session{s: s}, nil
}

// Discover performs a SendTargets discovery against the iSCSI target at addr.
// It dials, performs a Discovery session login, issues SendTargets, logs out,
// and returns the discovered targets.
func Discover(ctx context.Context, addr string, opts ...Option) ([]Target, error) {
	addr = normalizePortal(addr)
	cfg := &dialConfig{}
	for _, o := range opts {
		o(cfg)
	}

	targets, err := session.Discover(ctx, addr, cfg.loginOpts...)
	if err != nil {
		return nil, &TransportError{Op: "discover", Err: err}
	}

	result := make([]Target, len(targets))
	for i, dt := range targets {
		result[i] = convertTarget(dt)
	}
	return result, nil
}
