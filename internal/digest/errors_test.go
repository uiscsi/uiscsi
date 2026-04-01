package digest

import (
	"errors"
	"fmt"
	"testing"
)

func TestDigestTypeString(t *testing.T) {
	tests := []struct {
		dt   DigestType
		want string
	}{
		{DigestHeader, "header"},
		{DigestData, "data"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.dt.String(); got != tt.want {
				t.Errorf("DigestType(%d).String() = %q, want %q", tt.dt, got, tt.want)
			}
		})
	}
}

func TestDigestErrorFormat(t *testing.T) {
	e := &DigestError{
		Type:     DigestHeader,
		Expected: 0xAABBCCDD,
		Actual:   0x11223344,
	}
	want := "digest: header CRC32C mismatch: expected 0xAABBCCDD, got 0x11223344"
	if got := e.Error(); got != want {
		t.Errorf("DigestError.Error() = %q, want %q", got, want)
	}

	e2 := &DigestError{
		Type:     DigestData,
		Expected: 0x00000001,
		Actual:   0xFFFFFFFF,
	}
	want2 := "digest: data CRC32C mismatch: expected 0x00000001, got 0xFFFFFFFF"
	if got := e2.Error(); got != want2 {
		t.Errorf("DigestError.Error() = %q, want %q", got, want2)
	}
}

func TestDigestErrorAs(t *testing.T) {
	var orig error = &DigestError{
		Type:     DigestHeader,
		Expected: 0x12345678,
		Actual:   0x87654321,
	}
	// Wrap it
	wrapped := fmt.Errorf("read failed: %w", orig)

	var de *DigestError
	if !errors.As(wrapped, &de) {
		t.Fatal("errors.As failed to extract *DigestError from wrapped error")
	}
	if de.Type != DigestHeader {
		t.Errorf("Type = %v, want DigestHeader", de.Type)
	}
	if de.Expected != 0x12345678 {
		t.Errorf("Expected = 0x%08X, want 0x12345678", de.Expected)
	}
	if de.Actual != 0x87654321 {
		t.Errorf("Actual = 0x%08X, want 0x87654321", de.Actual)
	}
}
