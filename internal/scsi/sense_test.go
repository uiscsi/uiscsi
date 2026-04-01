package scsi

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestParseSense(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantKey SenseKey
		wantASC uint8
		wantASCQ uint8
		wantValid bool
		wantInfo  uint32
		wantErr bool
	}{
		{
			name: "fixed format 0x70 with valid info",
			data: []byte{
				0xF0,       // byte 0: valid=1, response code=0x70
				0x00,       // byte 1: segment number (obsolete)
				0x03,       // byte 2: sense key = MEDIUM ERROR
				0x00, 0x00, 0x01, 0x00, // bytes 3-6: information = 256
				0x0A,       // byte 7: additional sense length
				0x00, 0x00, 0x00, 0x00, // bytes 8-11: command-specific info
				0x11,       // byte 12: ASC = 0x11 (unrecovered read error)
				0x00,       // byte 13: ASCQ = 0x00
				0x00,       // byte 14: FRU code
				0x00, 0x00, 0x00, // bytes 15-17: sense key specific
			},
			wantKey:   SenseMediumError,
			wantASC:   0x11,
			wantASCQ:  0x00,
			wantValid: true,
			wantInfo:  256,
		},
		{
			name: "fixed format 0x70 invalid bit clear",
			data: []byte{
				0x70,       // byte 0: valid=0, response code=0x70
				0x00,
				0x05,       // sense key = ILLEGAL REQUEST
				0x00, 0x00, 0x00, 0x00,
				0x0A,
				0x00, 0x00, 0x00, 0x00,
				0x24,       // ASC = invalid field in CDB
				0x00,       // ASCQ
				0x00,
				0x00, 0x00, 0x00,
			},
			wantKey:   SenseIllegalRequest,
			wantASC:   0x24,
			wantASCQ:  0x00,
			wantValid: false,
			wantInfo:  0,
		},
		{
			name: "deferred fixed format 0x71",
			data: []byte{
				0xF1,       // valid=1, response code=0x71
				0x00,
				0x06,       // sense key = UNIT ATTENTION
				0x00, 0x00, 0x00, 0x00,
				0x0A,
				0x00, 0x00, 0x00, 0x00,
				0x29,       // ASC = power on reset
				0x00,
				0x00,
				0x00, 0x00, 0x00,
			},
			wantKey:  SenseUnitAttention,
			wantASC:  0x29,
			wantASCQ: 0x00,
			wantValid: true,
		},
		{
			name: "descriptor format 0x72",
			data: []byte{
				0x72,       // response code=0x72
				0x05,       // sense key = ILLEGAL REQUEST
				0x20,       // ASC = invalid command operation code
				0x00,       // ASCQ
				0x00, 0x00, 0x00, // reserved
				0x00,       // additional sense length
			},
			wantKey:  SenseIllegalRequest,
			wantASC:  0x20,
			wantASCQ: 0x00,
		},
		{
			name: "deferred descriptor format 0x73",
			data: []byte{
				0x73,       // response code=0x73
				0x02,       // sense key = NOT READY
				0x04,       // ASC = not ready
				0x01,       // ASCQ = becoming ready
				0x00, 0x00, 0x00,
				0x00,
			},
			wantKey:  SenseNotReady,
			wantASC:  0x04,
			wantASCQ: 0x01,
		},
		{
			name:    "too short data",
			data:    []byte{0x70},
			wantErr: true,
		},
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: true,
		},
		{
			name:    "unknown response code",
			data:    []byte{0x7E, 0x00, 0x00},
			wantErr: true,
		},
		{
			name: "fixed format too short for full parse",
			data: []byte{
				0x70, 0x00, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x11, 0x00, 0x00, 0x00,
				0x00, // only 17 bytes, need 18
			},
			wantErr: true,
		},
		{
			name: "descriptor format too short",
			data: []byte{
				0x72, 0x05, 0x20, 0x00, 0x00, 0x00, 0x00, // only 7 bytes, need 8
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sd, err := ParseSense(tt.data)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseSense() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSense() unexpected error: %v", err)
			}
			if sd.Key != tt.wantKey {
				t.Errorf("Key = %v, want %v", sd.Key, tt.wantKey)
			}
			if sd.ASC != tt.wantASC {
				t.Errorf("ASC = 0x%02X, want 0x%02X", sd.ASC, tt.wantASC)
			}
			if sd.ASCQ != tt.wantASCQ {
				t.Errorf("ASCQ = 0x%02X, want 0x%02X", sd.ASCQ, tt.wantASCQ)
			}
			if sd.Valid != tt.wantValid {
				t.Errorf("Valid = %v, want %v", sd.Valid, tt.wantValid)
			}
			if sd.Information != tt.wantInfo {
				t.Errorf("Information = %d, want %d", sd.Information, tt.wantInfo)
			}
			// Raw should be a defensive copy
			if len(sd.Raw) != len(tt.data) {
				t.Errorf("Raw length = %d, want %d", len(sd.Raw), len(tt.data))
			}
		})
	}
}

func TestSenseDataString(t *testing.T) {
	tests := []struct {
		name    string
		sd      SenseData
		wantContains string
	}{
		{
			name: "known ASC/ASCQ",
			sd: SenseData{
				Key:  SenseMediumError,
				ASC:  0x11,
				ASCQ: 0x00,
			},
			wantContains: "MEDIUM ERROR",
		},
		{
			name: "unknown ASC/ASCQ",
			sd: SenseData{
				Key:  SenseIllegalRequest,
				ASC:  0xFE,
				ASCQ: 0xFE,
			},
			wantContains: "UNKNOWN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.sd.String()
			if !strings.Contains(s, tt.wantContains) {
				t.Errorf("String() = %q, want to contain %q", s, tt.wantContains)
			}
		})
	}
}

func TestIsSenseKey(t *testing.T) {
	tests := []struct {
		name string
		err  error
		key  SenseKey
		want bool
	}{
		{
			name: "matching CommandError",
			err: &CommandError{
				Status: StatusCheckCondition,
				Sense:  &SenseData{Key: SenseMediumError},
			},
			key:  SenseMediumError,
			want: true,
		},
		{
			name: "non-matching CommandError",
			err: &CommandError{
				Status: StatusCheckCondition,
				Sense:  &SenseData{Key: SenseIllegalRequest},
			},
			key:  SenseMediumError,
			want: false,
		},
		{
			name: "non-CommandError",
			err:  errors.New("some error"),
			key:  SenseMediumError,
			want: false,
		},
		{
			name: "wrapped CommandError",
			err: fmt.Errorf("wrapped: %w", &CommandError{
				Status: StatusCheckCondition,
				Sense:  &SenseData{Key: SenseNotReady},
			}),
			key:  SenseNotReady,
			want: true,
		},
		{
			name: "nil sense in CommandError",
			err: &CommandError{
				Status: StatusBusy,
				Sense:  nil,
			},
			key:  SenseNoSense,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSenseKey(tt.err, tt.key); got != tt.want {
				t.Errorf("IsSenseKey() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSenseKeyString(t *testing.T) {
	keys := []struct {
		key  SenseKey
		want string
	}{
		{SenseNoSense, "NO SENSE"},
		{SenseRecoveredError, "RECOVERED ERROR"},
		{SenseNotReady, "NOT READY"},
		{SenseMediumError, "MEDIUM ERROR"},
		{SenseHardwareError, "HARDWARE ERROR"},
		{SenseIllegalRequest, "ILLEGAL REQUEST"},
		{SenseUnitAttention, "UNIT ATTENTION"},
		{SenseDataProtect, "DATA PROTECT"},
		{SenseBlankCheck, "BLANK CHECK"},
		{SenseVendorSpecific, "VENDOR SPECIFIC"},
		{SenseCopyAborted, "COPY ABORTED"},
		{SenseAbortedCommand, "ABORTED COMMAND"},
		{SenseVolumeOverflow, "VOLUME OVERFLOW"},
		{SenseMiscompare, "MISCOMPARE"},
	}

	for _, tt := range keys {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.key.String(); got != tt.want {
				t.Errorf("SenseKey(%d).String() = %q, want %q", tt.key, got, tt.want)
			}
		})
	}

	// Unknown key
	unknown := SenseKey(0x0F)
	s := unknown.String()
	if !strings.Contains(s, "0x0F") {
		t.Errorf("unknown SenseKey.String() = %q, want to contain hex value", s)
	}
}

func TestCommandErrorError(t *testing.T) {
	ce := &CommandError{
		Status: StatusCheckCondition,
		Sense: &SenseData{
			Key:  SenseMediumError,
			ASC:  0x11,
			ASCQ: 0x00,
		},
	}
	s := ce.Error()
	if !strings.Contains(s, "0x02") {
		t.Errorf("Error() = %q, want to contain status hex 0x02", s)
	}
	if !strings.Contains(s, "MEDIUM ERROR") {
		t.Errorf("Error() = %q, want to contain sense key name", s)
	}

	// CommandError with nil sense
	ce2 := &CommandError{Status: StatusBusy}
	s2 := ce2.Error()
	if !strings.Contains(s2, "0x08") {
		t.Errorf("Error() = %q, want to contain status hex 0x08", s2)
	}
}
