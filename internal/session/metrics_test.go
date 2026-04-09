package session

import (
	"context"
	"testing"

	"github.com/uiscsi/uiscsi/internal/transport"
)

func TestPDUDirection_String(t *testing.T) {
	tests := []struct {
		dir  PDUDirection
		want string
	}{
		{PDUSend, "send"},
		{PDUReceive, "receive"},
		{PDUDirection(99), "unknown"},
	}
	for _, tt := range tests {
		got := tt.dir.String()
		if got != tt.want {
			t.Errorf("PDUDirection(%d).String() = %q, want %q", tt.dir, got, tt.want)
		}
	}
}

func TestMetricEventType_String(t *testing.T) {
	tests := []struct {
		typ  MetricEventType
		want string
	}{
		{MetricPDUSent, "pdu_sent"},
		{MetricPDUReceived, "pdu_received"},
		{MetricCommandComplete, "command_complete"},
		{MetricBytesIn, "bytes_in"},
		{MetricBytesOut, "bytes_out"},
		{MetricEventType(99), "unknown"},
	}
	for _, tt := range tests {
		got := tt.typ.String()
		if got != tt.want {
			t.Errorf("MetricEventType(%d).String() = %q, want %q", tt.typ, got, tt.want)
		}
	}
}

func TestWithPDUHook(t *testing.T) {
	called := false
	hook := func(context.Context, PDUDirection, *transport.RawPDU) { called = true }

	cfg := defaultConfig()
	WithPDUHook(hook)(&cfg)

	if cfg.pduHook == nil {
		t.Fatal("WithPDUHook: pduHook field is nil after applying option")
	}

	// Verify the hook is the one we set by calling it.
	cfg.pduHook(context.Background(), PDUSend, &transport.RawPDU{})
	if !called {
		t.Error("WithPDUHook: hook was not invoked")
	}
}

func TestWithMetricsHook(t *testing.T) {
	var received MetricEvent
	hook := func(e MetricEvent) { received = e }

	cfg := defaultConfig()
	WithMetricsHook(hook)(&cfg)

	if cfg.metricsHook == nil {
		t.Fatal("WithMetricsHook: metricsHook field is nil after applying option")
	}

	cfg.metricsHook(MetricEvent{Type: MetricPDUSent})
	if received.Type != MetricPDUSent {
		t.Errorf("WithMetricsHook: received type = %v, want %v", received.Type, MetricPDUSent)
	}
}
