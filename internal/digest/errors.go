// Package digest provides CRC32C digest computation and error types for
// iSCSI header and data digests as specified in RFC 7143 Section 12.1.
package digest

import "fmt"

// DigestType indicates whether a digest error relates to a header or data digest.
type DigestType uint8

const (
	// DigestHeader indicates a header digest (CRC32C over BHS+AHS).
	DigestHeader DigestType = iota
	// DigestData indicates a data digest (CRC32C over data segment+padding).
	DigestData
)

// String returns "header" or "data" for the digest type.
func (d DigestType) String() string {
	switch d {
	case DigestHeader:
		return "header"
	case DigestData:
		return "data"
	default:
		return fmt.Sprintf("unknown(%d)", d)
	}
}

// DigestError is returned when a CRC32C digest mismatch is detected during
// PDU reading. It carries the digest type and both expected and actual values
// so callers can log detailed diagnostics.
type DigestError struct {
	Type     DigestType
	Expected uint32
	Actual   uint32
}

// Error returns a human-readable description of the digest mismatch.
func (e *DigestError) Error() string {
	return fmt.Sprintf("digest: %s CRC32C mismatch: expected 0x%08X, got 0x%08X",
		e.Type, e.Expected, e.Actual)
}
