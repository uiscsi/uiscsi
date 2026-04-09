package session

import (
	"context"
	"time"

	"github.com/uiscsi/uiscsi/internal/pdu"
	"github.com/uiscsi/uiscsi/internal/transport"
)

// PDUDirection indicates whether a PDU was sent or received.
type PDUDirection uint8

const (
	PDUSend    PDUDirection = iota
	PDUReceive
)

// String returns the human-readable name for the direction.
func (d PDUDirection) String() string {
	switch d {
	case PDUSend:
		return "send"
	case PDUReceive:
		return "receive"
	default:
		return "unknown"
	}
}

// MetricEventType discriminates metric event kinds.
type MetricEventType uint8

const (
	MetricPDUSent         MetricEventType = iota
	MetricPDUReceived
	MetricCommandComplete
	MetricBytesIn
	MetricBytesOut
)

// String returns the human-readable name for the metric event type.
func (t MetricEventType) String() string {
	switch t {
	case MetricPDUSent:
		return "pdu_sent"
	case MetricPDUReceived:
		return "pdu_received"
	case MetricCommandComplete:
		return "command_complete"
	case MetricBytesIn:
		return "bytes_in"
	case MetricBytesOut:
		return "bytes_out"
	default:
		return "unknown"
	}
}

// MetricEvent carries a single metric observation. Consumers aggregate
// as they see fit -- no concrete stats struct is prescribed.
type MetricEvent struct {
	Type    MetricEventType
	OpCode  pdu.OpCode    // for PDUSent/PDUReceived
	Bytes   uint64        // for BytesIn/BytesOut
	Latency time.Duration // for CommandComplete
}

// WithPDUHook registers a callback invoked for every PDU sent or received.
// The hook sees *RawPDU (raw wire bytes). It is called in ReadPump (after
// read, before dispatch) and WritePump (before write). The hook MUST NOT
// modify the RawPDU. The hook MAY be called concurrently from multiple
// goroutines.
func WithPDUHook(h func(context.Context, PDUDirection, *transport.RawPDU)) SessionOption {
	return func(c *sessionConfig) {
		c.pduHook = h
	}
}

// WithMetricsHook registers a callback invoked for metric events.
// Push-based callback: the consumer is responsible for synchronization.
func WithMetricsHook(h func(MetricEvent)) SessionOption {
	return func(c *sessionConfig) {
		c.metricsHook = h
	}
}
