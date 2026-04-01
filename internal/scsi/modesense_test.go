package scsi

import (
	"encoding/binary"
	"testing"
)

func TestModeSense6(t *testing.T) {
	tests := []struct {
		name        string
		pageCode    uint8
		subpageCode uint8
		allocLen    uint8
		opts        []Option
		wantCDB     [16]byte
	}{
		{
			name:        "basic without DBD",
			pageCode:    0x3F, // all pages
			subpageCode: 0x00,
			allocLen:    255,
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = OpModeSense6
				cdb[1] = 0x00          // DBD=0
				cdb[2] = 0x3F          // page code, PC=00
				cdb[3] = 0x00          // subpage
				cdb[4] = 255           // allocation length
				return cdb
			}(),
		},
		{
			name:        "with DBD set",
			pageCode:    0x08, // caching page
			subpageCode: 0x00,
			allocLen:    64,
			opts:        []Option{WithDBD()},
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = OpModeSense6
				cdb[1] = 0x08          // DBD=1 (bit 3)
				cdb[2] = 0x08          // page code
				cdb[3] = 0x00
				cdb[4] = 64
				return cdb
			}(),
		},
		{
			name:        "with page control",
			pageCode:    0x1C,
			subpageCode: 0x01,
			allocLen:    128,
			opts:        []Option{WithPageControl(1)}, // changeable values
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = OpModeSense6
				cdb[1] = 0x00
				cdb[2] = 0x5C          // PC=01 (bits 7-6) | page=0x1C
				cdb[3] = 0x01
				cdb[4] = 128
				return cdb
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := ModeSense6(0, tt.pageCode, tt.subpageCode, tt.allocLen, tt.opts...)
			if cmd.CDB != tt.wantCDB {
				t.Errorf("CDB = %X\nwant %X", cmd.CDB, tt.wantCDB)
			}
			if !cmd.Read {
				t.Error("Read should be true")
			}
			if cmd.ExpectedDataTransferLen != uint32(tt.allocLen) {
				t.Errorf("ExpectedDataTransferLen = %d, want %d", cmd.ExpectedDataTransferLen, tt.allocLen)
			}
		})
	}
}

func TestModeSense10(t *testing.T) {
	tests := []struct {
		name        string
		pageCode    uint8
		subpageCode uint8
		allocLen    uint16
		opts        []Option
		wantByte0   uint8
		wantByte1   uint8
		wantByte2   uint8
		wantByte3   uint8
		wantAllocHi uint8
		wantAllocLo uint8
	}{
		{
			name:        "basic",
			pageCode:    0x3F,
			subpageCode: 0x00,
			allocLen:    1024,
			wantByte0:   OpModeSense10,
			wantByte1:   0x00,
			wantByte2:   0x3F,
			wantByte3:   0x00,
			wantAllocHi: 0x04, // 1024 >> 8
			wantAllocLo: 0x00, // 1024 & 0xFF
		},
		{
			name:        "with DBD",
			pageCode:    0x08,
			subpageCode: 0x00,
			allocLen:    256,
			opts:        []Option{WithDBD()},
			wantByte0:   OpModeSense10,
			wantByte1:   0x08, // DBD
			wantByte2:   0x08,
			wantByte3:   0x00,
			wantAllocHi: 0x01,
			wantAllocLo: 0x00,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := ModeSense10(0, tt.pageCode, tt.subpageCode, tt.allocLen, tt.opts...)
			if cmd.CDB[0] != tt.wantByte0 {
				t.Errorf("CDB[0] = 0x%02X, want 0x%02X", cmd.CDB[0], tt.wantByte0)
			}
			if cmd.CDB[1] != tt.wantByte1 {
				t.Errorf("CDB[1] = 0x%02X, want 0x%02X", cmd.CDB[1], tt.wantByte1)
			}
			if cmd.CDB[2] != tt.wantByte2 {
				t.Errorf("CDB[2] = 0x%02X, want 0x%02X", cmd.CDB[2], tt.wantByte2)
			}
			if cmd.CDB[3] != tt.wantByte3 {
				t.Errorf("CDB[3] = 0x%02X, want 0x%02X", cmd.CDB[3], tt.wantByte3)
			}
			// Allocation length in bytes 7-8
			if cmd.CDB[7] != tt.wantAllocHi || cmd.CDB[8] != tt.wantAllocLo {
				t.Errorf("alloc bytes = [0x%02X, 0x%02X], want [0x%02X, 0x%02X]",
					cmd.CDB[7], cmd.CDB[8], tt.wantAllocHi, tt.wantAllocLo)
			}
			if !cmd.Read {
				t.Error("Read should be true")
			}
			if cmd.ExpectedDataTransferLen != uint32(tt.allocLen) {
				t.Errorf("ExpectedDataTransferLen = %d, want %d", cmd.ExpectedDataTransferLen, tt.allocLen)
			}
		})
	}
}

func TestParseModeSense6(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantMDL uint8
		wantMT  uint8
		wantBDL uint8
		wantBD  int
		wantPg  int
		wantErr bool
	}{
		{
			name: "header only, no block descriptors",
			data: []byte{
				0x03, // mode data length
				0x00, // medium type
				0x00, // device specific
				0x00, // block descriptor length
			},
			wantMDL: 3,
			wantMT:  0,
			wantBDL: 0,
			wantBD:  0,
			wantPg:  0,
		},
		{
			name: "with block descriptor and page",
			data: func() []byte {
				d := make([]byte, 4+8+4) // header + 8-byte BD + 4-byte page
				d[0] = byte(len(d) - 1)  // mode data length
				d[3] = 8                  // block descriptor length
				// block descriptor (8 bytes)
				d[4] = 0x00 // density
				binary.BigEndian.PutUint32(d[4:8], 0x00FFFFFF) // num blocks (3 bytes, but in 4-byte field)
				// mode page at offset 12
				d[12] = 0x08 // page code
				d[13] = 0x02 // page length
				d[14] = 0x01 // page data
				d[15] = 0x02
				return d
			}(),
			wantMDL: 15,
			wantMT:  0,
			wantBDL: 8,
			wantBD:  8,
			wantPg:  4,
		},
		{
			name:    "too short",
			data:    []byte{0x03, 0x00, 0x00},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ParseModeSense6(goodResult(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.ModeDataLength != tt.wantMDL {
				t.Errorf("ModeDataLength = %d, want %d", resp.ModeDataLength, tt.wantMDL)
			}
			if resp.MediumType != tt.wantMT {
				t.Errorf("MediumType = %d, want %d", resp.MediumType, tt.wantMT)
			}
			if resp.BlockDescriptorLength != tt.wantBDL {
				t.Errorf("BlockDescriptorLength = %d, want %d", resp.BlockDescriptorLength, tt.wantBDL)
			}
			if len(resp.BlockDescriptors) != tt.wantBD {
				t.Errorf("BlockDescriptors len = %d, want %d", len(resp.BlockDescriptors), tt.wantBD)
			}
			if len(resp.Pages) != tt.wantPg {
				t.Errorf("Pages len = %d, want %d", len(resp.Pages), tt.wantPg)
			}
		})
	}
}

func TestParseModeSense10(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantMDL uint16
		wantBDL uint16
		wantLBA bool
		wantBD  int
		wantPg  int
		wantErr bool
	}{
		{
			name: "header only",
			data: func() []byte {
				d := make([]byte, 8)
				binary.BigEndian.PutUint16(d[0:2], 6) // mode data length
				d[2] = 0x00 // medium type
				d[3] = 0x00 // device specific
				d[4] = 0x00 // long LBA = 0
				d[5] = 0x00 // reserved
				binary.BigEndian.PutUint16(d[6:8], 0) // block descriptor length
				return d
			}(),
			wantMDL: 6,
			wantBDL: 0,
			wantBD:  0,
			wantPg:  0,
		},
		{
			name: "with long LBA flag",
			data: func() []byte {
				d := make([]byte, 8+16) // header + 16-byte long block descriptor
				binary.BigEndian.PutUint16(d[0:2], uint16(len(d)-2))
				d[4] = 0x01 // long LBA = 1
				binary.BigEndian.PutUint16(d[6:8], 16) // BD length
				return d
			}(),
			wantMDL: 22,
			wantBDL: 16,
			wantLBA: true,
			wantBD:  16,
			wantPg:  0,
		},
		{
			name:    "too short",
			data:    make([]byte, 7),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := ParseModeSense10(goodResult(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.ModeDataLength != tt.wantMDL {
				t.Errorf("ModeDataLength = %d, want %d", resp.ModeDataLength, tt.wantMDL)
			}
			if resp.BlockDescriptorLength != tt.wantBDL {
				t.Errorf("BlockDescriptorLength = %d, want %d", resp.BlockDescriptorLength, tt.wantBDL)
			}
			if resp.LongLBA != tt.wantLBA {
				t.Errorf("LongLBA = %v, want %v", resp.LongLBA, tt.wantLBA)
			}
			if len(resp.BlockDescriptors) != tt.wantBD {
				t.Errorf("BlockDescriptors len = %d, want %d", len(resp.BlockDescriptors), tt.wantBD)
			}
			if len(resp.Pages) != tt.wantPg {
				t.Errorf("Pages len = %d, want %d", len(resp.Pages), tt.wantPg)
			}
		})
	}
}
