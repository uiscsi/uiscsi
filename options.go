// options.go defines functional options for Dial and Discover.
package uiscsi

import (
	"context"
	"encoding/binary"
	"log/slog"
	"time"

	"github.com/rkujawa/uiscsi/internal/login"
	"github.com/rkujawa/uiscsi/internal/session"
	"github.com/rkujawa/uiscsi/internal/transport"
)

// Option configures a Dial or Discover call via the functional options pattern.
type Option func(*dialConfig)

// dialConfig holds the accumulated options for Dial/Discover.
type dialConfig struct {
	loginOpts   []login.LoginOption
	sessionOpts []session.SessionOption
}

// WithTarget sets the target IQN for login.
func WithTarget(iqn string) Option {
	return func(c *dialConfig) {
		c.loginOpts = append(c.loginOpts, login.WithTarget(iqn))
	}
}

// WithCHAP enables CHAP authentication.
func WithCHAP(user, secret string) Option {
	return func(c *dialConfig) {
		c.loginOpts = append(c.loginOpts, login.WithCHAP(user, secret))
	}
}

// WithMutualCHAP enables mutual CHAP authentication.
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
func WithHeaderDigest(prefs ...string) Option {
	return func(c *dialConfig) {
		c.loginOpts = append(c.loginOpts, login.WithHeaderDigest(prefs...))
	}
}

// WithDataDigest sets data digest preferences.
func WithDataDigest(prefs ...string) Option {
	return func(c *dialConfig) {
		c.loginOpts = append(c.loginOpts, login.WithDataDigest(prefs...))
	}
}

// WithLogger sets the slog.Logger for both session and login diagnostics.
func WithLogger(l *slog.Logger) Option {
	return func(c *dialConfig) {
		c.loginOpts = append(c.loginOpts, login.WithLoginLogger(l))
		c.sessionOpts = append(c.sessionOpts, session.WithLogger(l))
	}
}

// WithKeepaliveInterval sets the keepalive ping interval.
func WithKeepaliveInterval(d time.Duration) Option {
	return func(c *dialConfig) {
		c.sessionOpts = append(c.sessionOpts, session.WithKeepaliveInterval(d))
	}
}

// WithKeepaliveTimeout sets the keepalive timeout.
func WithKeepaliveTimeout(d time.Duration) Option {
	return func(c *dialConfig) {
		c.sessionOpts = append(c.sessionOpts, session.WithKeepaliveTimeout(d))
	}
}

// WithAsyncHandler registers an async event callback.
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

// WithMetricsHook registers a metrics callback.
func WithMetricsHook(h func(MetricEvent)) Option {
	return func(c *dialConfig) {
		c.sessionOpts = append(c.sessionOpts, session.WithMetricsHook(func(me session.MetricEvent) {
			h(convertMetricEvent(me))
		}))
	}
}

// WithOperationalOverrides overrides login negotiation parameters.
// Keys must match RFC 7143 Section 13 key names exactly (e.g.,
// "InitialR2T", "ImmediateData", "MaxBurstLength", "ErrorRecoveryLevel").
// Values replace the defaults proposed during login negotiation.
func WithOperationalOverrides(overrides map[string]string) Option {
	return func(c *dialConfig) {
		c.loginOpts = append(c.loginOpts, login.WithOperationalOverrides(overrides))
	}
}

// WithDigestByteOrder sets the byte order for CRC32C digest values on the wire.
// Default is LittleEndian (matches Linux LIO target). Set to binary.BigEndian
// for targets that use network byte order for digests.
func WithDigestByteOrder(bo binary.ByteOrder) Option {
	return func(c *dialConfig) {
		c.sessionOpts = append(c.sessionOpts, session.WithDigestByteOrder(bo))
	}
}

// WithMaxReconnectAttempts sets the maximum number of ERL 0 reconnect attempts.
func WithMaxReconnectAttempts(n int) Option {
	return func(c *dialConfig) {
		c.sessionOpts = append(c.sessionOpts, session.WithMaxReconnectAttempts(n))
	}
}

// WithReconnectBackoff sets the reconnect backoff duration.
func WithReconnectBackoff(base time.Duration) Option {
	return func(c *dialConfig) {
		c.sessionOpts = append(c.sessionOpts, session.WithReconnectBackoff(base))
	}
}

// WithSNACKTimeout sets the timeout for SNACK-based PDU retransmission
// at ERL >= 1. When no Data-In arrives within this duration, a Status
// SNACK is sent to request the missing response (tail loss recovery).
// Default is 5 seconds.
func WithSNACKTimeout(d time.Duration) Option {
	return func(c *dialConfig) {
		c.sessionOpts = append(c.sessionOpts, session.WithSNACKTimeout(d))
	}
}
