// Package uiscsi provides a pure-userspace iSCSI initiator library.
// It handles TCP connection, login negotiation, and SCSI command transport
// over iSCSI PDUs entirely in userspace.
//
// # Quick Start
//
//	sess, err := uiscsi.Dial(ctx, "192.168.1.100:3260",
//	    uiscsi.WithTarget("iqn.2026-03.com.example:storage"),
//	)
//	if err != nil { ... }
//	defer sess.Close()
//
//	data, err := sess.SCSI().ReadBlocks(ctx, 0, 0, 1, 512)
//
// # API Tiers
//
// uiscsi provides three tiers of SCSI command access, each with different
// error handling. Choose based on your use case:
//
// ## Tier 1: Typed SCSI Methods (recommended for common operations)
//
// Access via [Session.SCSI]. Returns parsed Go types. CHECK CONDITION is
// automatically converted to [*SCSIError].
//
//	data, err := sess.SCSI().ReadBlocks(ctx, lun, lba, blocks, blockSize)
//	inq, err := sess.SCSI().Inquiry(ctx, lun)
//	err := sess.SCSI().ModeSelect6(ctx, lun, modeData)
//
// ## Tier 2: Buffered Raw CDB (for device-specific commands)
//
// Access via [Session.Raw]. Returns raw status + sense bytes. The caller
// interprets SCSI status. Use [CheckStatus] or [ParseSenseData] for convenience.
//
//	result, err := sess.Raw().Execute(ctx, lun, cdb, uiscsi.WithDataIn(256))
//	if err := uiscsi.CheckStatus(result.Status, result.SenseData); err != nil { ... }
//
// ## Tier 3: Streaming Raw CDB (for high-throughput sequential I/O)
//
// Access via [Session.Raw]. Response data delivered as [io.Reader] with
// bounded memory (~64KB). Critical for tape drives and large block transfers.
//
//	sr, err := sess.Raw().StreamExecute(ctx, lun, cdb, uiscsi.WithDataIn(blockSize))
//	_, err = io.Copy(dst, sr.Data)
//	status, sense, err := sr.Wait()
//
// # Performance Tuning
//
// The default MaxRecvDataSegmentLength is 8192 bytes (8KB per PDU). For
// high-throughput workloads (tape, large block I/O), increase it with
// [WithMaxRecvDataSegmentLength]:
//
//	sess, err := uiscsi.Dial(ctx, addr,
//	    uiscsi.WithTarget(iqn),
//	    uiscsi.WithMaxRecvDataSegmentLength(262144), // 256KB per PDU
//	)
//
// This reduces per-PDU overhead and improves streaming throughput. With
// StreamExecute, the bounded-memory window scales with MRDSL: 8 chunks
// × MRDSL bytes (e.g., 8 × 256KB = 2MB in-flight with 256KB MRDSL).
//
// # Other API Groups
//
// Task management: [Session.TMF] — AbortTask, LUNReset, TargetWarmReset, etc.
//
// Protocol operations: [Session.Protocol] — Logout, SendExpStatSNConfirmation.
package uiscsi

import (
	"context"
	"errors"
	"net"

	"github.com/uiscsi/uiscsi/internal/login"
	"github.com/uiscsi/uiscsi/internal/session"
	"github.com/uiscsi/uiscsi/internal/transport"
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
		_ = tc.Close()
		// Check for auth failure: LoginError with StatusClass == 2.
		var le *login.LoginError
		if errors.As(err, &le) && le.StatusClass == 2 {
			return nil, wrapAuthError(err)
		}
		return nil, &TransportError{Op: "login", Err: err}
	}

	// Step 3: Build session options including reconnect info for ERL 0.
	allSessionOpts := make([]session.SessionOption, 0, len(cfg.sessionOpts)+1)
	allSessionOpts = append(allSessionOpts, session.WithReconnectInfo(addr, cfg.loginOpts...))
	allSessionOpts = append(allSessionOpts, cfg.sessionOpts...)

	// Step 4: Create session.
	s := session.NewSession(tc, *params, allSessionOpts...)

	sess := &Session{s: s}
	sess.initOps()
	return sess, nil
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
