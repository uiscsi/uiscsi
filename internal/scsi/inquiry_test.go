package scsi

import (
	"encoding/binary"
	"testing"
)

func TestInquiry(t *testing.T) {
	cmd := Inquiry(2, 96)

	if cmd.CDB[0] != OpInquiry {
		t.Errorf("CDB[0] = 0x%02X, want 0x%02X", cmd.CDB[0], OpInquiry)
	}
	// EVPD = 0
	if cmd.CDB[1] != 0 {
		t.Errorf("CDB[1] = 0x%02X, want 0x00 (EVPD=0)", cmd.CDB[1])
	}
	// Page code = 0
	if cmd.CDB[2] != 0 {
		t.Errorf("CDB[2] = 0x%02X, want 0x00 (page code)", cmd.CDB[2])
	}
	// Allocation length in bytes 3-4
	allocLen := binary.BigEndian.Uint16(cmd.CDB[3:5])
	if allocLen != 96 {
		t.Errorf("allocation length = %d, want 96", allocLen)
	}
	if !cmd.Read {
		t.Error("Read should be true")
	}
	if cmd.Write {
		t.Error("Write should be false")
	}
	if cmd.ExpectedDataTransferLen != 96 {
		t.Errorf("ExpectedDataTransferLen = %d, want 96", cmd.ExpectedDataTransferLen)
	}
	if cmd.LUN != 2 {
		t.Errorf("LUN = %d, want 2", cmd.LUN)
	}
}

func TestParseInquiry(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		wantType  uint8
		wantQual  uint8
		wantVendor string
		wantProduct string
		wantRev   string
		wantErr   bool
	}{
		{
			name: "standard 36-byte response",
			data: func() []byte {
				d := make([]byte, 36)
				d[0] = 0x00       // peripheral device type = 0 (disk), qualifier = 0
				d[4] = 31         // additional length
				copy(d[8:16], []byte("VENDOR  "))  // 8 bytes, space padded
				copy(d[16:32], []byte("PRODUCT MODEL   ")) // 16 bytes
				copy(d[32:36], []byte("1.0 "))     // 4 bytes
				return d
			}(),
			wantType:    0x00,
			wantQual:    0x00,
			wantVendor:  "VENDOR",
			wantProduct: "PRODUCT MODEL",
			wantRev:     "1.0",
		},
		{
			name: "device type and qualifier",
			data: func() []byte {
				d := make([]byte, 36)
				d[0] = 0x65 // qualifier=3, device type=5 (CD-ROM)
				d[4] = 31
				copy(d[8:16], []byte("CDMAKER "))
				copy(d[16:32], []byte("BURNER 3000     "))
				copy(d[32:36], []byte("2.01"))
				return d
			}(),
			wantType:    0x05,
			wantQual:    0x03,
			wantVendor:  "CDMAKER",
			wantProduct: "BURNER 3000",
			wantRev:     "2.01",
		},
		{
			name:    "short response",
			data:    make([]byte, 35),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ParseInquiry(goodResult(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.PeripheralDeviceType != tt.wantType {
				t.Errorf("DeviceType = %d, want %d", resp.PeripheralDeviceType, tt.wantType)
			}
			if resp.PeripheralQualifier != tt.wantQual {
				t.Errorf("Qualifier = %d, want %d", resp.PeripheralQualifier, tt.wantQual)
			}
			if resp.Vendor != tt.wantVendor {
				t.Errorf("Vendor = %q, want %q", resp.Vendor, tt.wantVendor)
			}
			if resp.Product != tt.wantProduct {
				t.Errorf("Product = %q, want %q", resp.Product, tt.wantProduct)
			}
			if resp.Revision != tt.wantRev {
				t.Errorf("Revision = %q, want %q", resp.Revision, tt.wantRev)
			}
		})
	}

	// CHECK CONDITION test
	t.Run("check condition", func(t *testing.T) {
		result := checkConditionResult(SenseIllegalRequest, 0x24, 0x00)
		_, err := ParseInquiry(result)
		if err == nil {
			t.Fatal("expected error for CHECK CONDITION")
		}
		if !IsSenseKey(err, SenseIllegalRequest) {
			t.Errorf("expected ILLEGAL REQUEST sense key, got %v", err)
		}
	})
}
