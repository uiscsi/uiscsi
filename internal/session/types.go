// Package session implements the iSCSI session layer: command dispatch,
// CmdSN flow control, Data-In reassembly, and session lifecycle management
// per RFC 7143.
package session

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/uiscsi/uiscsi/internal/login"
	"github.com/uiscsi/uiscsi/internal/transport"
)

// Command represents a SCSI command to be submitted via the session.
// The caller fills in CDB, data direction flags, and transfer length;
// the session assigns CmdSN, ITT, and ExpStatSN.
type Command struct {
	CDB                    [16]byte
	Read                   bool
	Write                  bool
	ExpectedDataTransferLen uint32
	LUN                    uint64
	TaskAttributes         uint8

	// Data provides write payload. Non-nil means write command. Callers
	// with []byte use bytes.NewReader(). The io.Reader must remain
	// readable until the Result is received.
	Data io.Reader
}

// Result carries the outcome of a submitted SCSI command.
// For read commands, Data is an io.Reader that streams the response data
// assembled from one or more Data-In PDUs. For non-read commands Data is nil.
type Result struct {
	Status        uint8
	SenseData     []byte
	Data          io.Reader // nil for non-read commands
	Overflow      bool
	Underflow     bool
	ResidualCount uint32
	Err           error // non-nil if the command failed at the transport level
}

// AsyncEvent carries an asynchronous event message from the target.
// RFC 7143 Section 11.9.
type AsyncEvent struct {
	EventCode  uint8
	VendorCode uint8
	Parameter1 uint16
	Parameter2 uint16
	Parameter3 uint16
	Data       []byte
}

// DiscoveryTarget represents a target discovered via SendTargets.
type DiscoveryTarget struct {
	Name    string
	Portals []Portal
}

// Portal represents a target portal (address + port + portal group tag).
type Portal struct {
	Address  string
	Port     int
	GroupTag int
}

// TMF function codes per RFC 7143 Section 11.5.1.
const (
	TMFAbortTask        uint8 = 1
	TMFAbortTaskSet     uint8 = 2
	TMFClearTaskSet     uint8 = 3
	TMFLogicalUnitReset uint8 = 5
	TMFTargetWarmReset  uint8 = 6
	TMFTargetColdReset  uint8 = 7
	TMFTaskReassign     uint8 = 14
)

// TMF response codes per RFC 7143 Section 11.6.1.
const (
	TMFRespComplete           uint8 = 0
	TMFRespTaskNotExist       uint8 = 1
	TMFRespLUNNotExist        uint8 = 2
	TMFRespTaskAllegiant      uint8 = 3
	TMFRespReassignNotSupport uint8 = 4
	TMFRespNotSupported       uint8 = 5
	TMFRespAuthFailed         uint8 = 6
	TMFRespRejected           uint8 = 255
)

// SNACK type values per RFC 7143 Section 11.16.1.
const (
	SNACKTypeDataR2T    uint8 = 0
	SNACKTypeStatus     uint8 = 1
	SNACKTypeDataACK    uint8 = 2
	SNACKTypeRDataSNACK uint8 = 3
)

// TMFResult carries the outcome of a task management function request.
type TMFResult struct {
	Response uint8
	Err      error
}

// ErrTaskAborted is the sentinel error delivered to a task's resultCh when
// the task is aborted via a TMF request.
var ErrTaskAborted = errors.New("session: task aborted")

// SessionOption configures a Session via the functional options pattern.
type SessionOption func(*sessionConfig)

// sessionConfig holds tunable session parameters. Unexported to enforce
// construction via SessionOption functions.
type sessionConfig struct {
	keepaliveInterval    time.Duration
	keepaliveTimeout     time.Duration
	asyncHandler         func(context.Context, AsyncEvent)
	pduHook              func(context.Context, PDUDirection, *transport.RawPDU)
	metricsHook          func(MetricEvent)
	logger               *slog.Logger
	maxReconnectAttempts int
	reconnectBackoff     time.Duration
	snackTimeout         time.Duration
	targetAddr           string
	loginOpts            []login.LoginOption
	digestByteOrder      binary.ByteOrder
	streamBufDepth       int // chanReader buffer depth (0 = default 128)
	routerBufDepth       int // persistent router channel depth (0 = default 64)
}

// defaultConfig returns a sessionConfig with sensible defaults.
func defaultConfig() sessionConfig {
	return sessionConfig{
		keepaliveInterval:    30 * time.Second,
		keepaliveTimeout:     5 * time.Second,
		logger:               slog.Default(),
		maxReconnectAttempts: 3,
		reconnectBackoff:     1 * time.Second,
		snackTimeout:         5 * time.Second,
	}
}

// WithKeepaliveInterval sets the interval between NOP-Out keepalive pings.
func WithKeepaliveInterval(d time.Duration) SessionOption {
	return func(c *sessionConfig) {
		c.keepaliveInterval = d
	}
}

// WithKeepaliveTimeout sets the deadline for a NOP-In reply to a keepalive ping.
func WithKeepaliveTimeout(d time.Duration) SessionOption {
	return func(c *sessionConfig) {
		c.keepaliveTimeout = d
	}
}

// WithAsyncHandler registers a callback invoked for each AsyncMsg received
// from the target. If nil, async events are logged and discarded.
func WithAsyncHandler(h func(context.Context, AsyncEvent)) SessionOption {
	return func(c *sessionConfig) {
		c.asyncHandler = h
	}
}

// WithLogger overrides the default slog.Logger for session diagnostics.
func WithLogger(l *slog.Logger) SessionOption {
	return func(c *sessionConfig) {
		c.logger = l
	}
}

// WithMaxReconnectAttempts sets the maximum number of reconnection attempts
// for error recovery level 0 (session-level reconnection).
func WithMaxReconnectAttempts(n int) SessionOption {
	return func(c *sessionConfig) {
		c.maxReconnectAttempts = n
	}
}

// WithReconnectBackoff sets the base backoff duration between reconnection
// attempts. Actual backoff may include jitter or exponential growth.
func WithReconnectBackoff(base time.Duration) SessionOption {
	return func(c *sessionConfig) {
		c.reconnectBackoff = base
	}
}

// WithSNACKTimeout sets the timeout for SNACK-based PDU retransmission
// requests used in error recovery level 1.
func WithSNACKTimeout(d time.Duration) SessionOption {
	return func(c *sessionConfig) {
		c.snackTimeout = d
	}
}

// WithStreamBufDepth sets the chanReader buffer depth for streaming reads
// (StreamExecute / SubmitStreaming). Higher values absorb more consumer
// stalls before triggering TCP backpressure. Default is 128 slots.
func WithStreamBufDepth(depth int) SessionOption {
	return func(c *sessionConfig) {
		c.streamBufDepth = depth
	}
}

// WithRouterBufDepth sets the PDU dispatch channel depth for persistent
// task routing. Higher values absorb more processing stalls before blocking
// the read pump. Default is 64 slots.
func WithRouterBufDepth(depth int) SessionOption {
	return func(c *sessionConfig) {
		c.routerBufDepth = depth
	}
}

// WithDigestByteOrder sets the byte order for CRC32C digest values.
func WithDigestByteOrder(bo binary.ByteOrder) SessionOption {
	return func(c *sessionConfig) {
		c.digestByteOrder = bo
	}
}

// WithReconnectInfo configures the session for ERL 0 automatic reconnection
// and ERL 2 connection replacement. The target address and login options are
// stored so the session can re-dial and re-login after a connection drop.
func WithReconnectInfo(addr string, loginOpts ...login.LoginOption) SessionOption {
	return func(c *sessionConfig) {
		c.targetAddr = addr
		c.loginOpts = loginOpts
	}
}
