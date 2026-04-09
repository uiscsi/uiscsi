package uiscsi

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/uiscsi/uiscsi/internal/login"
	"github.com/uiscsi/uiscsi/internal/scsi"
)

func TestSCSIError_Error(t *testing.T) {
	e := &SCSIError{
		Status:   0x02,
		SenseKey: 0x05,
		ASC:      0x24,
		ASCQ:     0x00,
		Message:  "ILLEGAL REQUEST: Invalid field in CDB",
	}
	got := e.Error()
	want := "scsi: status 0x02: ILLEGAL REQUEST: Invalid field in CDB"
	if got != want {
		t.Errorf("SCSIError.Error() = %q, want %q", got, want)
	}
}

func TestSCSIError_ErrorNoMessage(t *testing.T) {
	e := &SCSIError{
		Status: 0x02,
	}
	got := e.Error()
	want := "scsi: status 0x02"
	if got != want {
		t.Errorf("SCSIError.Error() = %q, want %q", got, want)
	}
}

func TestTransportError_Error(t *testing.T) {
	inner := fmt.Errorf("connection refused")
	e := &TransportError{Op: "dial", Err: inner}
	got := e.Error()
	want := "iscsi dial: connection refused"
	if got != want {
		t.Errorf("TransportError.Error() = %q, want %q", got, want)
	}
}

func TestTransportError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("connection refused")
	e := &TransportError{Op: "dial", Err: inner}

	if e.Unwrap() != inner {
		t.Errorf("TransportError.Unwrap() did not return underlying error")
	}
}

func TestAuthError_Error(t *testing.T) {
	e := &AuthError{
		StatusClass:  2,
		StatusDetail: 1,
		Message:      "authentication failure",
	}
	got := e.Error()
	want := "iscsi auth: authentication failure (class=2 detail=1)"
	if got != want {
		t.Errorf("AuthError.Error() = %q, want %q", got, want)
	}
}

func TestErrorsAs_SCSIError(t *testing.T) {
	inner := &SCSIError{Status: 0x02, SenseKey: 0x05, Message: "test"}
	wrapped := fmt.Errorf("command failed: %w", inner)

	var target *SCSIError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As failed to find *SCSIError in wrapped error")
	}
	if target.Status != 0x02 {
		t.Errorf("SCSIError.Status = 0x%02X, want 0x02", target.Status)
	}
}

func TestErrorsAs_TransportError(t *testing.T) {
	inner := &TransportError{Op: "submit", Err: fmt.Errorf("broken pipe")}
	wrapped := fmt.Errorf("session error: %w", inner)

	var target *TransportError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As failed to find *TransportError in wrapped error")
	}
	if target.Op != "submit" {
		t.Errorf("TransportError.Op = %q, want %q", target.Op, "submit")
	}
}

func TestErrorsAs_AuthError(t *testing.T) {
	inner := &AuthError{StatusClass: 2, StatusDetail: 1, Message: "auth fail"}
	wrapped := fmt.Errorf("login failed: %w", inner)

	var target *AuthError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As failed to find *AuthError in wrapped error")
	}
	if target.StatusClass != 2 {
		t.Errorf("AuthError.StatusClass = %d, want 2", target.StatusClass)
	}
}

func TestWrapSCSIError(t *testing.T) {
	sense := &scsi.SenseData{
		Key:  scsi.SenseIllegalRequest,
		ASC:  0x24,
		ASCQ: 0x00,
	}
	cmdErr := &scsi.CommandError{
		Status: 0x02,
		Sense:  sense,
	}

	wrapped := wrapSCSIError(cmdErr)
	var se *SCSIError
	if !errors.As(wrapped, &se) {
		t.Fatal("wrapSCSIError did not produce *SCSIError")
	}
	if se.Status != 0x02 {
		t.Errorf("SCSIError.Status = 0x%02X, want 0x02", se.Status)
	}
	if se.SenseKey != uint8(scsi.SenseIllegalRequest) {
		t.Errorf("SCSIError.SenseKey = 0x%02X, want 0x05", se.SenseKey)
	}
	if se.ASC != 0x24 {
		t.Errorf("SCSIError.ASC = 0x%02X, want 0x24", se.ASC)
	}
}

func TestWrapSCSIError_NonCommandError(t *testing.T) {
	plain := fmt.Errorf("some other error")
	wrapped := wrapSCSIError(plain)

	// Should return the original error when not a CommandError.
	if wrapped != plain {
		t.Errorf("wrapSCSIError should return original error for non-CommandError")
	}
}

func TestWrapTransportError(t *testing.T) {
	inner := fmt.Errorf("connection reset")
	wrapped := wrapTransportError("submit", inner)

	var te *TransportError
	if !errors.As(wrapped, &te) {
		t.Fatal("wrapTransportError did not produce *TransportError")
	}
	if te.Op != "submit" {
		t.Errorf("TransportError.Op = %q, want %q", te.Op, "submit")
	}
	if te.Err != inner {
		t.Error("TransportError.Err does not match original")
	}
}

func TestWrapAuthError(t *testing.T) {
	loginErr := &login.LoginError{
		StatusClass:  2,
		StatusDetail: 1,
		Message:      "authentication failure",
	}

	wrapped := wrapAuthError(loginErr)
	var ae *AuthError
	if !errors.As(wrapped, &ae) {
		t.Fatal("wrapAuthError did not produce *AuthError")
	}
	if ae.StatusClass != 2 {
		t.Errorf("AuthError.StatusClass = %d, want 2", ae.StatusClass)
	}
	if ae.StatusDetail != 1 {
		t.Errorf("AuthError.StatusDetail = %d, want 1", ae.StatusDetail)
	}
}

func TestWrapAuthError_NonLoginError(t *testing.T) {
	plain := fmt.Errorf("some other error")
	wrapped := wrapAuthError(plain)

	// Non-LoginError should be wrapped as TransportError with op="login".
	var te *TransportError
	if !errors.As(wrapped, &te) {
		t.Fatal("wrapAuthError for non-LoginError should produce *TransportError")
	}
	if te.Op != "login" {
		t.Errorf("TransportError.Op = %q, want %q", te.Op, "login")
	}
}

func TestAuthError_ErrorIncludesStatusCodes(t *testing.T) {
	ae := &AuthError{
		Message:      "authentication failed",
		StatusClass:  2,
		StatusDetail: 1,
	}
	got := ae.Error()
	if !strings.Contains(got, "class=2") {
		t.Fatalf("AuthError.Error() missing StatusClass: %s", got)
	}
	if !strings.Contains(got, "detail=1") {
		t.Fatalf("AuthError.Error() missing StatusDetail: %s", got)
	}
}
