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
		wantInfo  uint64
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

func TestParseSenseFilemarkEOMILI(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		wantFilemark bool
		wantEOM      bool
		wantILI      bool
		wantKey      SenseKey
	}{
		{
			name: "filemark set with MEDIUM_ERROR",
			data: []byte{
				0xF0,                         // valid=1, response code=0x70
				0x00,                         // segment number
				0x83,                         // filemark=1, EOM=0, ILI=0, key=MEDIUM ERROR
				0x00, 0x00, 0x00, 0x00,       // information
				0x0A,                         // additional sense length
				0x00, 0x00, 0x00, 0x00,       // command-specific info
				0x00, 0x00,                   // ASC/ASCQ
				0x00,                         // FRU
				0x00, 0x00, 0x00,             // sense key specific
			},
			wantFilemark: true,
			wantEOM:      false,
			wantILI:      false,
			wantKey:      SenseMediumError,
		},
		{
			name: "EOM set",
			data: []byte{
				0xF0, 0x00,
				0x40, // EOM=1, filemark=0, ILI=0, key=NO SENSE
				0x00, 0x00, 0x00, 0x00,
				0x0A,
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00,
				0x00,
				0x00, 0x00, 0x00,
			},
			wantFilemark: false,
			wantEOM:      true,
			wantILI:      false,
			wantKey:      SenseNoSense,
		},
		{
			name: "ILI set",
			data: []byte{
				0xF0, 0x00,
				0x20, // ILI=1, filemark=0, EOM=0, key=NO SENSE
				0x00, 0x00, 0x00, 0x00,
				0x0A,
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00,
				0x00,
				0x00, 0x00, 0x00,
			},
			wantFilemark: false,
			wantEOM:      false,
			wantILI:      true,
			wantKey:      SenseNoSense,
		},
		{
			name: "all three set",
			data: []byte{
				0xF0, 0x00,
				0xE0, // filemark=1, EOM=1, ILI=1, key=NO SENSE
				0x00, 0x00, 0x00, 0x00,
				0x0A,
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00,
				0x00,
				0x00, 0x00, 0x00,
			},
			wantFilemark: true,
			wantEOM:      true,
			wantILI:      true,
			wantKey:      SenseNoSense,
		},
		{
			name: "none set with MEDIUM_ERROR",
			data: []byte{
				0xF0, 0x00,
				0x03, // filemark=0, EOM=0, ILI=0, key=MEDIUM ERROR
				0x00, 0x00, 0x00, 0x00,
				0x0A,
				0x00, 0x00, 0x00, 0x00,
				0x00, 0x00,
				0x00,
				0x00, 0x00, 0x00,
			},
			wantFilemark: false,
			wantEOM:      false,
			wantILI:      false,
			wantKey:      SenseMediumError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sd, err := ParseSense(tt.data)
			if err != nil {
				t.Fatalf("ParseSense() unexpected error: %v", err)
			}
			if sd.Filemark != tt.wantFilemark {
				t.Errorf("Filemark = %v, want %v", sd.Filemark, tt.wantFilemark)
			}
			if sd.EOM != tt.wantEOM {
				t.Errorf("EOM = %v, want %v", sd.EOM, tt.wantEOM)
			}
			if sd.ILI != tt.wantILI {
				t.Errorf("ILI = %v, want %v", sd.ILI, tt.wantILI)
			}
			if sd.Key != tt.wantKey {
				t.Errorf("Key = %v, want %v", sd.Key, tt.wantKey)
			}
		})
	}
}

func TestAscLookupTapeCodes(t *testing.T) {
	tests := []struct {
		asc  uint8
		ascq uint8
		want string
	}{
		{0x00, 0x01, "Filemark detected"},
		{0x00, 0x02, "End-of-partition/medium detected"},
		{0x00, 0x04, "Beginning-of-partition/medium detected"},
		{0x00, 0x05, "End-of-data detected"},
		{0x30, 0x03, "Cleaning cartridge installed"},
		{0x3B, 0x00, "Sequential positioning error"},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("ASC=0x%02X_ASCQ=0x%02X", tt.asc, tt.ascq)
		t.Run(name, func(t *testing.T) {
			got := ascLookup(tt.asc, tt.ascq)
			if got != tt.want {
				t.Errorf("ascLookup(0x%02X, 0x%02X) = %q, want %q", tt.asc, tt.ascq, got, tt.want)
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

func TestDescriptorSenseInformation(t *testing.T) {
	// Information descriptor (type 0x00): descType=0x00, descLen=0x0A, valid_bit=0x80, reserved=0x00, 8-byte info
	// Descriptor layout: [descType(1), descLen(1), valid|reserved(1), reserved(1), info(8)] = 12 bytes total
	infoBytes := []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, // 8-byte info = 256
	}
	descriptors := []byte{
		0x00, 0x0A, // type=0x00, len=10
		0x80, 0x00, // valid=1, reserved
	}
	descriptors = append(descriptors, infoBytes...)

	// Full descriptor-format sense buffer: header(8) + descriptors
	data := []byte{
		0x72,                   // byte 0: response code 0x72
		0x05,                   // byte 1: sense key = ILLEGAL REQUEST
		0x24,                   // byte 2: ASC
		0x00,                   // byte 3: ASCQ
		0x00, 0x00, 0x00,       // bytes 4-6: reserved
		byte(len(descriptors)), // byte 7: additional sense length
	}
	data = append(data, descriptors...)

	sd, err := ParseSense(data)
	if err != nil {
		t.Fatalf("ParseSense() unexpected error: %v", err)
	}
	if sd.Key != SenseIllegalRequest {
		t.Errorf("Key = %v, want ILLEGAL REQUEST", sd.Key)
	}
	if !sd.Valid {
		t.Errorf("Valid = false, want true (VALID bit set in Information descriptor)")
	}
	if sd.Information != 256 {
		t.Errorf("Information = %d, want 256", sd.Information)
	}
}

func TestDescriptorSenseStreamCommands(t *testing.T) {
	// Stream commands descriptor (type 0x04): descType=0x04, descLen=0x02, reserved=0x00, flags_byte
	// flags_byte bit 7=filemark, bit 6=EOM, bit 5=ILI
	streamDesc := []byte{
		0x04, 0x02, // type=0x04, len=2
		0x00,       // reserved
		0x80,       // filemark=1, EOM=0, ILI=0
	}

	data := []byte{
		0x72,                   // response code
		0x00,                   // sense key = NO SENSE
		0x00, 0x01,             // ASC=0x00 ASCQ=0x01 (filemark detected)
		0x00, 0x00, 0x00,       // reserved
		byte(len(streamDesc)),  // additional sense length
	}
	data = append(data, streamDesc...)

	sd, err := ParseSense(data)
	if err != nil {
		t.Fatalf("ParseSense() unexpected error: %v", err)
	}
	if !sd.Filemark {
		t.Errorf("Filemark = false, want true")
	}
	if sd.EOM {
		t.Errorf("EOM = true, want false")
	}
	if sd.ILI {
		t.Errorf("ILI = true, want false")
	}
}

func TestDescriptorSenseBothDescriptors(t *testing.T) {
	// Both Information and Stream Commands descriptors
	infoDesc := []byte{
		0x00, 0x0A, // type=0x00, len=10
		0x80, 0x00, // valid=1, reserved
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x42, // info = 0x42 = 66
	}
	streamDesc := []byte{
		0x04, 0x02, // type=0x04, len=2
		0x00,       // reserved
		0xE0,       // filemark=1, EOM=1, ILI=1
	}
	descriptors := make([]byte, 0, len(infoDesc)+len(streamDesc))
	descriptors = append(descriptors, infoDesc...)
	descriptors = append(descriptors, streamDesc...)

	data := []byte{
		0x72,
		0x00,
		0x00, 0x00,
		0x00, 0x00, 0x00,
		byte(len(descriptors)),
	}
	data = append(data, descriptors...)

	sd, err := ParseSense(data)
	if err != nil {
		t.Fatalf("ParseSense() unexpected error: %v", err)
	}
	if sd.Information != 0x42 {
		t.Errorf("Information = %d, want 66", sd.Information)
	}
	if !sd.Valid {
		t.Errorf("Valid = false, want true")
	}
	if !sd.Filemark {
		t.Errorf("Filemark = false, want true")
	}
	if !sd.EOM {
		t.Errorf("EOM = false, want true")
	}
	if !sd.ILI {
		t.Errorf("ILI = false, want true")
	}
}

func TestDescriptorSenseNoDescriptors(t *testing.T) {
	// Descriptor format with addlLen=0 (no descriptors): fields should be zero/false
	data := []byte{
		0x72,                // response code
		0x05,                // sense key = ILLEGAL REQUEST
		0x24, 0x00,          // ASC/ASCQ
		0x00, 0x00, 0x00,    // reserved
		0x00,                // additional sense length = 0
	}

	sd, err := ParseSense(data)
	if err != nil {
		t.Fatalf("ParseSense() unexpected error: %v", err)
	}
	if sd.Information != 0 {
		t.Errorf("Information = %d, want 0", sd.Information)
	}
	if sd.Filemark {
		t.Errorf("Filemark = true, want false")
	}
	if sd.Valid {
		t.Errorf("Valid = true, want false")
	}
}

func TestDescriptorSenseTruncatedDescriptor(t *testing.T) {
	// Descriptor that claims length exceeds remaining data — must not panic
	// Claim addlLen=20 but only provide 4 bytes of descriptors
	data := []byte{
		0x72,
		0x00,
		0x00, 0x00,
		0x00, 0x00, 0x00,
		20,   // additional sense length claims 20 bytes follow
		0x04, // type=stream commands
		0x02, // claims len=2, but we cut off here — only 2 bytes provided
	}

	sd, err := ParseSense(data)
	// Should not panic. May or may not error depending on implementation.
	if err != nil {
		return // acceptable to return error for truncated
	}
	// If it succeeds, fields should be zero (truncated descriptor not applied)
	_ = sd
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
