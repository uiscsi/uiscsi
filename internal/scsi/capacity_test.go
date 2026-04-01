package scsi

import (
	"encoding/binary"
	"testing"
)

func TestReadCapacity10(t *testing.T) {
	cmd := ReadCapacity10(5)

	if cmd.CDB[0] != OpReadCapacity10 {
		t.Errorf("CDB[0] = 0x%02X, want 0x%02X", cmd.CDB[0], OpReadCapacity10)
	}
	// Bytes 1-9 should be zero
	for i := 1; i <= 9; i++ {
		if cmd.CDB[i] != 0 {
			t.Errorf("CDB[%d] = 0x%02X, want 0x00", i, cmd.CDB[i])
		}
	}
	if !cmd.Read {
		t.Error("Read should be true")
	}
	if cmd.Write {
		t.Error("Write should be false")
	}
	if cmd.ExpectedDataTransferLen != 8 {
		t.Errorf("ExpectedDataTransferLen = %d, want 8", cmd.ExpectedDataTransferLen)
	}
	if cmd.LUN != 5 {
		t.Errorf("LUN = %d, want 5", cmd.LUN)
	}
}

func TestParseReadCapacity10(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		wantLBA   uint32
		wantBS    uint32
		wantErr   bool
	}{
		{
			name: "normal response",
			data: func() []byte {
				b := make([]byte, 8)
				binary.BigEndian.PutUint32(b[0:4], 0x001FFFFF) // last LBA
				binary.BigEndian.PutUint32(b[4:8], 512)        // block size
				return b
			}(),
			wantLBA: 0x001FFFFF,
			wantBS:  512,
		},
		{
			name: "max values",
			data: func() []byte {
				b := make([]byte, 8)
				binary.BigEndian.PutUint32(b[0:4], 0xFFFFFFFF) // max LBA
				binary.BigEndian.PutUint32(b[4:8], 4096)       // 4K blocks
				return b
			}(),
			wantLBA: 0xFFFFFFFF,
			wantBS:  4096,
		},
		{
			name:    "short data",
			data:    make([]byte, 7),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ParseReadCapacity10(goodResult(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.LastLBA != tt.wantLBA {
				t.Errorf("LastLBA = 0x%08X, want 0x%08X", resp.LastLBA, tt.wantLBA)
			}
			if resp.BlockSize != tt.wantBS {
				t.Errorf("BlockSize = %d, want %d", resp.BlockSize, tt.wantBS)
			}
		})
	}
}

func TestReadCapacity16(t *testing.T) {
	cmd := ReadCapacity16(7, 32)

	// Pitfall 1: RC16 uses SERVICE ACTION IN
	if cmd.CDB[0] != OpServiceActionIn16 {
		t.Errorf("CDB[0] = 0x%02X, want 0x%02X (SERVICE ACTION IN)", cmd.CDB[0], OpServiceActionIn16)
	}
	// Service action = 0x10
	if cmd.CDB[1] != 0x10 {
		t.Errorf("CDB[1] = 0x%02X, want 0x10 (READ CAPACITY 16 service action)", cmd.CDB[1])
	}
	// Allocation length in bytes 10-13
	allocLen := binary.BigEndian.Uint32(cmd.CDB[10:14])
	if allocLen != 32 {
		t.Errorf("allocation length = %d, want 32", allocLen)
	}
	if !cmd.Read {
		t.Error("Read should be true")
	}
	if cmd.LUN != 7 {
		t.Errorf("LUN = %d, want 7", cmd.LUN)
	}
	if cmd.ExpectedDataTransferLen != 32 {
		t.Errorf("ExpectedDataTransferLen = %d, want 32", cmd.ExpectedDataTransferLen)
	}
}

func TestParseReadCapacity16(t *testing.T) {
	tests := []struct {
		name          string
		data          []byte
		wantLBA       uint64
		wantBS        uint32
		wantProtEn    bool
		wantProtType  uint8
		wantLBPerPhys uint8
		wantLowestLBA uint16
		wantErr       bool
	}{
		{
			name: "normal 32-byte response",
			data: func() []byte {
				b := make([]byte, 32)
				binary.BigEndian.PutUint64(b[0:8], 0x00000001FFFFFFFF) // last LBA
				binary.BigEndian.PutUint32(b[8:12], 512)               // block size
				b[12] = 0x00                                            // no protection
				b[13] = 0x03                                            // 2^3 = 8 logical blocks per physical
				binary.BigEndian.PutUint16(b[14:16], 0x0000)           // lowest aligned LBA
				return b
			}(),
			wantLBA:       0x00000001FFFFFFFF,
			wantBS:        512,
			wantProtEn:    false,
			wantProtType:  0,
			wantLBPerPhys: 3,
			wantLowestLBA: 0,
		},
		{
			name: "with protection enabled",
			data: func() []byte {
				b := make([]byte, 32)
				binary.BigEndian.PutUint64(b[0:8], 1000000)
				binary.BigEndian.PutUint32(b[8:12], 4096)
				b[12] = 0x07 // protection enabled, type 3
				b[13] = 0x02
				binary.BigEndian.PutUint16(b[14:16], 0x1234)
				return b
			}(),
			wantLBA:       1000000,
			wantBS:        4096,
			wantProtEn:    true,
			wantProtType:  3,
			wantLBPerPhys: 2,
			wantLowestLBA: 0x1234,
		},
		{
			name:    "short data",
			data:    make([]byte, 31),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ParseReadCapacity16(goodResult(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.LastLBA != tt.wantLBA {
				t.Errorf("LastLBA = 0x%016X, want 0x%016X", resp.LastLBA, tt.wantLBA)
			}
			if resp.BlockSize != tt.wantBS {
				t.Errorf("BlockSize = %d, want %d", resp.BlockSize, tt.wantBS)
			}
			if resp.ProtectionEnabled != tt.wantProtEn {
				t.Errorf("ProtectionEnabled = %v, want %v", resp.ProtectionEnabled, tt.wantProtEn)
			}
			if resp.ProtectionType != tt.wantProtType {
				t.Errorf("ProtectionType = %d, want %d", resp.ProtectionType, tt.wantProtType)
			}
			if resp.LogicalBlocksPerPhysicalBlock != tt.wantLBPerPhys {
				t.Errorf("LBPerPhys = %d, want %d", resp.LogicalBlocksPerPhysicalBlock, tt.wantLBPerPhys)
			}
			if resp.LowestAlignedLBA != tt.wantLowestLBA {
				t.Errorf("LowestAlignedLBA = 0x%04X, want 0x%04X", resp.LowestAlignedLBA, tt.wantLowestLBA)
			}
		})
	}
}
