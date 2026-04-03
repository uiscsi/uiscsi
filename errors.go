// errors.go defines the public error hierarchy for the uiscsi package.
// All errors support errors.As/errors.Is for inspection.
package uiscsi

import (
	"errors"
	"fmt"

	"github.com/rkujawa/uiscsi/internal/login"
	"github.com/rkujawa/uiscsi/internal/scsi"
)

// SCSIError represents a SCSI command failure with status and optional sense data.
type SCSIError struct {
	Status   uint8
	SenseKey uint8
	ASC      uint8
	ASCQ     uint8
	Message  string
}

// Note: SCSIError does not implement Unwrap() because it does not wrap
// an underlying error. It is a leaf error containing extracted SCSI
// status and sense data fields. Use errors.As to match it directly.

// Error returns a human-readable description of the SCSI error.
func (e *SCSIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("scsi: status 0x%02X: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("scsi: status 0x%02X", e.Status)
}

// TransportError represents an iSCSI transport/connection failure.
type TransportError struct {
	Op  string // "dial", "login", "submit", "read", etc.
	Err error  // underlying error
}

// Error returns a human-readable description of the transport error.
func (e *TransportError) Error() string {
	return fmt.Sprintf("iscsi %s: %s", e.Op, e.Err)
}

// Unwrap returns the underlying error, enabling errors.As/errors.Is chains.
func (e *TransportError) Unwrap() error {
	return e.Err
}

// AuthError represents an authentication failure during login.
type AuthError struct {
	StatusClass  uint8
	StatusDetail uint8
	Message      string
}

// Error returns a human-readable description of the auth error.
func (e *AuthError) Error() string {
	return fmt.Sprintf("iscsi auth: %s (class=%d detail=%d)", e.Message, e.StatusClass, e.StatusDetail)
}

// wrapSCSIError converts a scsi.CommandError to a public SCSIError.
// If err is not a *scsi.CommandError, it is returned unchanged.
func wrapSCSIError(err error) error {
	var ce *scsi.CommandError
	if !errors.As(err, &ce) {
		return err
	}
	se := &SCSIError{
		Status: ce.Status,
	}
	if ce.Sense != nil {
		se.SenseKey = uint8(ce.Sense.Key)
		se.ASC = ce.Sense.ASC
		se.ASCQ = ce.Sense.ASCQ
		se.Message = ce.Sense.String()
	}
	return se
}

// wrapTransportError wraps an arbitrary error into a TransportError with the given Op.
func wrapTransportError(op string, err error) error {
	return &TransportError{Op: op, Err: err}
}

// wrapAuthError converts a login.LoginError to a public AuthError.
// Non-LoginError errors are wrapped as TransportError{Op: "login"}.
func wrapAuthError(err error) error {
	var le *login.LoginError
	if errors.As(err, &le) {
		return &AuthError{
			StatusClass:  le.StatusClass,
			StatusDetail: le.StatusDetail,
			Message:      le.Message,
		}
	}
	return &TransportError{Op: "login", Err: err}
}
