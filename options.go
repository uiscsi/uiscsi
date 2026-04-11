package uiscsi

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"time"

	"github.com/uiscsi/uiscsi/internal/login"
	"github.com/uiscsi/uiscsi/internal/session"
	"github.com/uiscsi/uiscsi/internal/transport"
)

// Option configures a Dial or Discover call via the functional options pattern.
type Option func(*dialConfig)

// dialConfig holds the accumulated options for Dial/Discover.
type dialConfig struct {
	loginOpts   []login.LoginOption
	sessionOpts []session.SessionOption
	dialTimeout time.Duration
}

// WithTarget sets the target IQN for login.
func WithTarget(iqn string) Option {
	return func(c *dialConfig) {
		c.loginOpts = append(c.loginOpts, login.WithTarget(iqn))
	}
}

// WithDialTimeout sets the TCP connection timeout for the initial dial.
// This is independent of the context deadline — context controls the
// overall operation, while dial timeout controls only the TCP handshake.
// Default is 0 (no explicit timeout beyond context).
func WithDialTimeout(d time.Duration) Option {
	return func(c *dialConfig) {
		c.dialTimeout = d
	}
}

// WithCHAP enables CHAP authentication for the iSCSI login exchange.
//
// # Security
//
// Without WithCHAP, login is unauthenticated — any initiator knowing the
// target IQN can connect. CHAP authenticates the login phase only; the
// iSCSI data phase is always transmitted in cleartext regardless of CHAP
// usage. For environments where data confidentiality is required, protect
// the iSCSI network with IPsec (RFC 7143 Section 8).
//
// CHAP secrets are accepted in memory and never written to disk by this
// library. Use [WithMutualCHAP] for bidirectional authentication.
func WithCHAP(user, secret string) Option {
	return func(c *dialConfig) {
		c.loginOpts = append(c.loginOpts, login.WithCHAP(user, secret))
	}
}

// WithMutualCHAP enables mutual CHAP authentication.
//
// deadcode: retained — part of the public iSCSI initiator API for external consumers.
func WithMutualCHAP(user, secret, targetSecret string) Option {
	return func(c *dialConfig) {
		c.loginOpts = append(c.loginOpts, login.WithMutualCHAP(user, secret, targetSecret))
	}
}

// WithInitiatorName sets the initiator IQN.
func WithInitiatorName(iqn string) Option {
	return func(c *dialConfig) {
		c.loginOpts = append(c.loginOpts, login.WithInitiatorName(iqn))
	}
}

// WithHeaderDigest sets header digest preferences.
//
// deadcode: retained — part of the public iSCSI initiator API for external consumers.
func WithHeaderDigest(prefs ...string) Option {
	return func(c *dialConfig) {
		c.loginOpts = append(c.loginOpts, login.WithHeaderDigest(prefs...))
	}
}

// WithDataDigest sets data digest preferences.
//
// deadcode: retained — part of the public iSCSI initiator API for external consumers.
func WithDataDigest(prefs ...string) Option {
	return func(c *dialConfig) {
		c.loginOpts = append(c.loginOpts, login.WithDataDigest(prefs...))
	}
}

// WithLogger sets the [*slog.Logger] used for session and login diagnostics.
//
// When WithLogger is not called, the session uses [slog.Default]. To
// suppress all library output, pass a logger backed by [io.Discard]:
//
//	slog.New(slog.NewTextHandler(io.Discard, nil))
func WithLogger(l *slog.Logger) Option {
	return func(c *dialConfig) {
		c.loginOpts = append(c.loginOpts, login.WithLoginLogger(l))
		c.sessionOpts = append(c.sessionOpts, session.WithLogger(l))
	}
}

// WithKeepaliveInterval sets the NOP-Out keepalive ping interval.
// The session sends NOP-Out PDUs at this interval to detect dead
// connections. If no NOP-In reply arrives within [WithKeepaliveTimeout]
// (default 5s), the connection is considered lost and ERL 0 reconnect
// begins.
//
// Recommended values:
//   - LTO drives (fast networks): 30s (default)
//   - DDS-4 drives (slow links, ~6 MB/s): 60s (reduces NOP overhead)
//   - High-latency WAN iSCSI: 120s
//
// A zero value uses the default (30 seconds).
//
// deadcode: retained — part of the public iSCSI initiator API for external consumers.
func WithKeepaliveInterval(d time.Duration) Option {
	return func(c *dialConfig) {
		c.sessionOpts = append(c.sessionOpts, session.WithKeepaliveInterval(d))
	}
}

// WithKeepaliveTimeout sets the keepalive timeout.
//
// deadcode: retained — part of the public iSCSI initiator API for external consumers.
func WithKeepaliveTimeout(d time.Duration) Option {
	return func(c *dialConfig) {
		c.sessionOpts = append(c.sessionOpts, session.WithKeepaliveTimeout(d))
	}
}

// WithAsyncHandler registers an async event callback.
//
// deadcode: retained — part of the public iSCSI initiator API for external consumers.
func WithAsyncHandler(h func(context.Context, AsyncEvent)) Option {
	return func(c *dialConfig) {
		c.sessionOpts = append(c.sessionOpts, session.WithAsyncHandler(func(ctx context.Context, ae session.AsyncEvent) {
			h(ctx, convertAsyncEvent(ae))
		}))
	}
}

// WithPDUHook registers a PDU send/receive hook. The []byte argument is the
// concatenation of BHS (48 bytes) + DataSegment from the internal
// transport.RawPDU. This avoids exposing internal transport types.
//
// deadcode: retained — part of the public iSCSI initiator API for external consumers.
func WithPDUHook(h func(context.Context, PDUDirection, []byte)) Option {
	return func(c *dialConfig) {
		c.sessionOpts = append(c.sessionOpts, session.WithPDUHook(func(ctx context.Context, dir session.PDUDirection, raw *transport.RawPDU) {
			pubDir := PDUDirection(dir)
			data := make([]byte, len(raw.BHS)+len(raw.DataSegment))
			copy(data, raw.BHS[:])
			copy(data[len(raw.BHS):], raw.DataSegment)
			h(ctx, pubDir, data)
		}))
	}
}

// WithMetricsHook registers a callback invoked for each [MetricEvent].
// Events include PDU send/receive counts, byte counts, and per-command
// completion latency (via [MetricCommandComplete] events with the
// [MetricEvent.Latency] field). The hook is called from internal
// goroutines and MUST NOT block.
//
// deadcode: retained — part of the public iSCSI initiator API for external consumers.
func WithMetricsHook(h func(MetricEvent)) Option {
	return func(c *dialConfig) {
		c.sessionOpts = append(c.sessionOpts, session.WithMetricsHook(func(me session.MetricEvent) {
			h(convertMetricEvent(me))
		}))
	}
}

// WithStateChangeHook registers a callback invoked when the session
// transitions between lifecycle states. The hook receives the new
// [SessionState] value. State transitions are:
//
//   - [SessionFullFeature]: login complete, session ready for commands
//   - [SessionReconnecting]: connection lost, ERL 0 reconnect started
//   - [SessionFullFeature]: reconnect succeeded, commands resume
//   - [SessionClosed]: session permanently closed
//
// The hook is called from internal goroutines and MUST NOT block or
// call back into the session (Submit, Close, etc.) — doing so risks
// deadlock. The hook MAY be called concurrently.
func WithStateChangeHook(h func(SessionState)) Option {
	return func(c *dialConfig) {
		c.sessionOpts = append(c.sessionOpts, session.WithStateChangeHook(func(s session.SessionState) {
			h(SessionState(s))
		}))
	}
}

// WithMaxRecvDataSegmentLength sets the maximum data segment size (in bytes)
// that the initiator is willing to receive per PDU. The target uses this to
// limit Data-In PDU sizes. Default is 8192 (8KB).
//
// For high-throughput workloads (tape drives, large block I/O), increasing
// this to 65536 (64KB) or 262144 (256KB) significantly reduces per-PDU
// overhead and improves streaming performance. The value must be between
// 512 and 16777215 per RFC 7143.
//
// This controls the initiator's declared value. The target independently
// declares its own MaxRecvDataSegmentLength (limiting Data-Out PDU sizes
// from the initiator). Both directions are negotiated independently.
func WithMaxRecvDataSegmentLength(size uint32) Option {
	return func(c *dialConfig) {
		c.loginOpts = append(c.loginOpts, login.WithOperationalOverrides(map[string]string{
			"MaxRecvDataSegmentLength": fmt.Sprintf("%d", size),
		}))
	}
}

// WithOperationalOverrides overrides login negotiation parameters.
// Keys must match RFC 7143 Section 13 key names exactly (e.g.,
// "InitialR2T", "ImmediateData", "MaxBurstLength", "ErrorRecoveryLevel").
// Values replace the defaults proposed during login negotiation.
//
// deadcode: retained — part of the public iSCSI initiator API for external consumers.
func WithOperationalOverrides(overrides map[string]string) Option {
	return func(c *dialConfig) {
		c.loginOpts = append(c.loginOpts, login.WithOperationalOverrides(overrides))
	}
}

// WithDigestByteOrder sets the byte order for CRC32C digest values on the wire.
// Default is LittleEndian (matches Linux LIO target). Set to binary.BigEndian
// for targets that use network byte order for digests.
//
// deadcode: retained — part of the public iSCSI initiator API for external consumers.
func WithDigestByteOrder(bo binary.ByteOrder) Option {
	return func(c *dialConfig) {
		c.sessionOpts = append(c.sessionOpts, session.WithDigestByteOrder(bo))
	}
}

// WithMaxBurstLength sets the maximum data burst size for solicited Data-Out
// transfers (write operations). Default is 262144 (256KB). Increasing this
// allows larger write bursts per R2T, reducing R2T round-trips for large writes.
//
// deadcode: retained — part of the public iSCSI initiator API for external consumers.
func WithMaxBurstLength(size uint32) Option {
	return func(c *dialConfig) {
		c.loginOpts = append(c.loginOpts, login.WithOperationalOverrides(map[string]string{
			"MaxBurstLength": fmt.Sprintf("%d", size),
		}))
	}
}

// WithFirstBurstLength sets the maximum unsolicited Data-Out size for write
// operations. Default is 65536 (64KB). Data up to this size is sent
// immediately with the SCSI Command PDU, without waiting for an R2T.
//
// deadcode: retained — part of the public iSCSI initiator API for external consumers.
func WithFirstBurstLength(size uint32) Option {
	return func(c *dialConfig) {
		c.loginOpts = append(c.loginOpts, login.WithOperationalOverrides(map[string]string{
			"FirstBurstLength": fmt.Sprintf("%d", size),
		}))
	}
}

// WithStreamBufDepth sets the streaming Data-In buffer depth (number of PDU
// chunks buffered in the chanReader channel). Higher values absorb consumer
// stalls without triggering TCP backpressure — critical for tape drives that
// stop streaming on any TCP stall. Default is 128 slots.
//
// deadcode: retained — part of the public iSCSI initiator API for external consumers.
func WithStreamBufDepth(depth int) Option {
	return func(c *dialConfig) {
		c.sessionOpts = append(c.sessionOpts, session.WithStreamBufDepth(depth))
	}
}

// WithRouterBufDepth sets the PDU dispatch buffer depth per task. Higher
// values absorb read pump stalls. Default is 64 slots.
//
// deadcode: retained — part of the public iSCSI initiator API for external consumers.
func WithRouterBufDepth(depth int) Option {
	return func(c *dialConfig) {
		c.sessionOpts = append(c.sessionOpts, session.WithRouterBufDepth(depth))
	}
}

// WithMaxReconnectAttempts sets the maximum number of ERL 0 reconnect attempts.
//
// deadcode: retained — part of the public iSCSI initiator API for external consumers.
func WithMaxReconnectAttempts(n int) Option {
	return func(c *dialConfig) {
		c.sessionOpts = append(c.sessionOpts, session.WithMaxReconnectAttempts(n))
	}
}

// WithReconnectBackoff sets the reconnect backoff duration.
//
// deadcode: retained — part of the public iSCSI initiator API for external consumers.
func WithReconnectBackoff(base time.Duration) Option {
	return func(c *dialConfig) {
		c.sessionOpts = append(c.sessionOpts, session.WithReconnectBackoff(base))
	}
}

// WithSNACKTimeout sets the timeout for SNACK-based PDU retransmission
// at ERL >= 1. When no Data-In arrives within this duration, a Status
// SNACK is sent to request the missing response (tail loss recovery).
// Default is 5 seconds.
//
// deadcode: retained — part of the public iSCSI initiator API for external consumers.
func WithSNACKTimeout(d time.Duration) Option {
	return func(c *dialConfig) {
		c.sessionOpts = append(c.sessionOpts, session.WithSNACKTimeout(d))
	}
}
