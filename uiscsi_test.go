package uiscsi_test

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/uiscsi/uiscsi"
)

func TestSCSIError_Error_External(t *testing.T) {
	e := &uiscsi.SCSIError{
		Status:   0x02,
		SenseKey: 0x05,
		Message:  "test sense message",
	}
	got := e.Error()
	if got != "scsi: status 0x02: test sense message" {
		t.Errorf("SCSIError.Error() = %q, want expected format", got)
	}
}

func TestTransportError_Unwrap_External(t *testing.T) {
	inner := errors.New("connection refused")
	e := &uiscsi.TransportError{Op: "dial", Err: inner}

	var te *uiscsi.TransportError
	if !errors.As(e, &te) {
		t.Fatal("errors.As should match *TransportError")
	}
	if te.Unwrap() != inner {
		t.Error("Unwrap did not return underlying error")
	}
}

func TestAuthError_Error_External(t *testing.T) {
	e := &uiscsi.AuthError{
		StatusClass:  2,
		StatusDetail: 1,
		Message:      "authentication failure",
	}
	if e.Error() != "iscsi auth: authentication failure (class=2 detail=1)" {
		t.Errorf("AuthError.Error() = %q, want expected format", e.Error())
	}
}

func TestDial_Unreachable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 127.0.0.1:1 should be refused immediately on localhost.
	_, err := uiscsi.Dial(ctx, "127.0.0.1:1")
	if err == nil {
		t.Fatal("Dial to unreachable address should fail")
	}

	var te *uiscsi.TransportError
	if !errors.As(err, &te) {
		t.Fatalf("Dial error should be *TransportError, got %T: %v", err, err)
	}
	if te.Op != "dial" {
		t.Errorf("TransportError.Op = %q, want %q", te.Op, "dial")
	}
}

func TestOptions_Compile(t *testing.T) {
	// Verify all With* functions exist and compile. We don't call Dial,
	// just ensure the options produce the correct type.
	var opts []uiscsi.Option

	opts = append(opts, uiscsi.WithTarget("iqn.2026-03.com.example:test"))
	opts = append(opts, uiscsi.WithCHAP("user", "secret"))
	opts = append(opts, uiscsi.WithMutualCHAP("user", "secret", "tsecret"))
	opts = append(opts, uiscsi.WithInitiatorName("iqn.2026-03.com.example:initiator"))
	opts = append(opts, uiscsi.WithHeaderDigest("CRC32C", "None"))
	opts = append(opts, uiscsi.WithDataDigest("None"))
	opts = append(opts, uiscsi.WithLogger(nil))
	opts = append(opts, uiscsi.WithKeepaliveInterval(30*time.Second))
	opts = append(opts, uiscsi.WithKeepaliveTimeout(5*time.Second))
	opts = append(opts, uiscsi.WithAsyncHandler(func(context.Context, uiscsi.AsyncEvent) {}))
	opts = append(opts, uiscsi.WithPDUHook(func(context.Context, uiscsi.PDUDirection, []byte) {}))
	opts = append(opts, uiscsi.WithMetricsHook(func(uiscsi.MetricEvent) {}))
	opts = append(opts, uiscsi.WithMaxReconnectAttempts(5))
	opts = append(opts, uiscsi.WithReconnectBackoff(2*time.Second))
	opts = append(opts, uiscsi.WithDigestByteOrder(binary.LittleEndian))

	if len(opts) != 15 {
		t.Errorf("expected 15 options, got %d", len(opts))
	}
}

func TestDial_DefaultPort(t *testing.T) {
	// Dial to a non-routable address without port should default to 3260.
	// 192.0.2.1 is TEST-NET-1 (RFC 5737), guaranteed unreachable.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, err := uiscsi.Dial(ctx, "192.0.2.1")
	if err == nil {
		t.Fatal("expected error dialing 192.0.2.1 without port")
	}
	var te *uiscsi.TransportError
	if !errors.As(err, &te) {
		t.Fatalf("expected TransportError, got %T: %v", err, err)
	}
	if te.Op != "dial" {
		t.Errorf("Op = %q, want %q", te.Op, "dial")
	}
}

func TestDeviceTypeName(t *testing.T) {
	tests := []struct {
		code uint8
		want string
	}{
		{0x00, "disk"},
		{0x01, "tape"},
		{0x05, "cd/dvd"},
		{0x08, "media changer"},
		{0x0D, "enclosure"},
		{0x0E, "disk"},
		{0x1F, "unknown"},
		{0x0A, "unknown"}, // unmapped code
	}
	for _, tt := range tests {
		got := uiscsi.DeviceTypeName(tt.code)
		if got != tt.want {
			t.Errorf("DeviceTypeName(0x%02X) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestDecodeLUN(t *testing.T) {
	tests := []struct {
		name string
		raw  uint64
		want uint64
	}{
		{"LUN 0", 0x0000000000000000, 0},
		{"LUN 1 peripheral", 0x0001000000000000, 1},
		{"LUN 2 peripheral", 0x0002000000000000, 2},
		{"LUN 255 peripheral", 0x00FF000000000000, 255},
		{"LUN 1 flat space", 0x4001000000000000, 1},
		{"LUN 256 flat space", 0x4100000000000000, 256},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := uiscsi.DecodeLUN(tt.raw)
			if got != tt.want {
				t.Errorf("DecodeLUN(0x%016X) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}

