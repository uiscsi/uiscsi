package scsi

import (
	"encoding/binary"
	"testing"
)

func TestInquiryVPD(t *testing.T) {
	tests := []struct {
		name     string
		lun      uint64
		pageCode uint8
		allocLen uint16
		wantCDB  [16]byte
	}{
		{
			name:     "supported pages",
			lun:      0,
			pageCode: 0x00,
			allocLen: 252,
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = 0x12
				cdb[1] = 0x01 // EVPD
				cdb[2] = 0x00
				binary.BigEndian.PutUint16(cdb[3:5], 252)
				return cdb
			}(),
		},
		{
			name:     "device identification",
			lun:      1,
			pageCode: 0x83,
			allocLen: 252,
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = 0x12
				cdb[1] = 0x01
				cdb[2] = 0x83
				binary.BigEndian.PutUint16(cdb[3:5], 252)
				return cdb
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := InquiryVPD(tt.lun, tt.pageCode, tt.allocLen)
			if cmd.CDB != tt.wantCDB {
				t.Errorf("CDB = %X, want %X", cmd.CDB, tt.wantCDB)
			}
			if !cmd.Read {
				t.Error("Read should be true")
			}
			if cmd.Write {
				t.Error("Write should be false")
			}
			if cmd.ExpectedDataTransferLen != uint32(tt.allocLen) {
				t.Errorf("ExpectedDataTransferLen = %d, want %d", cmd.ExpectedDataTransferLen, tt.allocLen)
			}
			if cmd.LUN != tt.lun {
				t.Errorf("LUN = %d, want %d", cmd.LUN, tt.lun)
			}
		})
	}
}

func TestParseVPDSupportedPages(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    []uint8
		wantErr bool
	}{
		{
			name: "three pages",
			data: []byte{0x00, 0x00, 0x00, 0x03, 0x00, 0x80, 0x83},
			want: []uint8{0x00, 0x80, 0x83},
		},
		{
			name: "empty list",
			data: []byte{0x00, 0x00, 0x00, 0x00},
			want: []uint8{},
		},
		{
			name:    "short data",
			data:    []byte{0x00, 0x00},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pages, err := ParseVPDSupportedPages(goodResult(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(pages) != len(tt.want) {
				t.Fatalf("got %d pages, want %d", len(pages), len(tt.want))
			}
			for i, p := range pages {
				if p != tt.want[i] {
					t.Errorf("page[%d] = 0x%02X, want 0x%02X", i, p, tt.want[i])
				}
			}
		})
	}
}

func TestParseVPDSerialNumber(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    string
		wantErr bool
	}{
		{
			name: "ASCII serial",
			data: []byte{0x00, 0x80, 0x00, 0x08, 'A', 'B', 'C', '1', '2', '3', '4', '5'},
			want: "ABC12345",
		},
		{
			name: "trailing spaces trimmed",
			data: []byte{0x00, 0x80, 0x00, 0x0A, 'S', 'E', 'R', 'I', 'A', 'L', ' ', ' ', ' ', ' '},
			want: "SERIAL",
		},
		{
			name:    "short data",
			data:    []byte{0x00, 0x80, 0x00},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serial, err := ParseVPDSerialNumber(goodResult(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if serial != tt.want {
				t.Errorf("serial = %q, want %q", serial, tt.want)
			}
		})
	}
}

func TestParseVPDDeviceIdentification(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    []Designator
		wantErr bool
	}{
		{
			name: "single descriptor",
			data: func() []byte {
				// VPD header: peripheral, page code, page length (2 bytes)
				d := make([]byte, 4+4+8) // header + descriptor header + 8-byte identifier
				d[0] = 0x00              // peripheral
				d[1] = 0x83              // page code
				binary.BigEndian.PutUint16(d[2:4], 12) // page length = descriptor header(4) + identifier(8)
				// Descriptor at offset 4
				d[4] = 0x61 // ProtocolID=6 (bits 7-4), CodeSet=1 (bits 3-0)
				d[5] = 0x03 // Association=0 (bits 5-4=00), Type=3 (bits 3-0)
				d[6] = 0x00 // reserved
				d[7] = 0x08 // identifier length
				copy(d[8:16], []byte("IDENT123"))
				return d
			}(),
			want: []Designator{
				{
					CodeSet:    1,
					ProtocolID: 6,
					Type:       3,
					Association: 0,
					Identifier: []byte("IDENT123"),
				},
			},
		},
		{
			name: "multiple descriptors variable length (Pitfall 5)",
			data: func() []byte {
				// Two descriptors: first 4-byte ID, second 8-byte ID
				d := make([]byte, 4+4+4+4+8) // header + desc1(4+4) + desc2(4+8)
				d[0] = 0x00
				d[1] = 0x83
				binary.BigEndian.PutUint16(d[2:4], 20) // page length
				// Descriptor 1 at offset 4
				d[4] = 0x01  // ProtocolID=0, CodeSet=1
				d[5] = 0x01  // Association=0, Type=1
				d[7] = 0x04  // length 4
				copy(d[8:12], []byte("ABCD"))
				// Descriptor 2 at offset 12
				d[12] = 0x22 // ProtocolID=2, CodeSet=2
				d[13] = 0x13 // Association=1 (bits 5-4=01), Type=3
				d[15] = 0x08 // length 8
				copy(d[16:24], []byte("LONGIDEN"))
				return d
			}(),
			want: []Designator{
				{CodeSet: 1, ProtocolID: 0, Type: 1, Association: 0, Identifier: []byte("ABCD")},
				{CodeSet: 2, ProtocolID: 2, Type: 3, Association: 1, Identifier: []byte("LONGIDEN")},
			},
		},
		{
			name: "malformed descriptor length exceeds data",
			data: func() []byte {
				d := make([]byte, 4+4+2) // header + descriptor header + only 2 bytes
				d[1] = 0x83
				binary.BigEndian.PutUint16(d[2:4], 6)
				d[7] = 0x08 // claims 8 bytes but only 2 available
				return d
			}(),
			wantErr: true,
		},
		{
			name:    "short data",
			data:    []byte{0x00, 0x83},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			desigs, err := ParseVPDDeviceIdentification(goodResult(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(desigs) != len(tt.want) {
				t.Fatalf("got %d designators, want %d", len(desigs), len(tt.want))
			}
			for i, d := range desigs {
				w := tt.want[i]
				if d.CodeSet != w.CodeSet {
					t.Errorf("[%d] CodeSet = %d, want %d", i, d.CodeSet, w.CodeSet)
				}
				if d.ProtocolID != w.ProtocolID {
					t.Errorf("[%d] ProtocolID = %d, want %d", i, d.ProtocolID, w.ProtocolID)
				}
				if d.Type != w.Type {
					t.Errorf("[%d] Type = %d, want %d", i, d.Type, w.Type)
				}
				if d.Association != w.Association {
					t.Errorf("[%d] Association = %d, want %d", i, d.Association, w.Association)
				}
				if string(d.Identifier) != string(w.Identifier) {
					t.Errorf("[%d] Identifier = %q, want %q", i, d.Identifier, w.Identifier)
				}
			}
		})
	}
}

func TestParseVPDBlockLimits(t *testing.T) {
	data := make([]byte, 64)
	data[0] = 0x00 // peripheral
	data[1] = 0xB0 // page code
	binary.BigEndian.PutUint16(data[2:4], 60) // page length
	data[5] = 0x20                             // MaxCompareAndWriteLength
	binary.BigEndian.PutUint16(data[6:8], 128) // OptimalTransferLengthGranularity
	binary.BigEndian.PutUint32(data[8:12], 65536)  // MaxTransferLength
	binary.BigEndian.PutUint32(data[12:16], 8192)  // OptimalTransferLength
	binary.BigEndian.PutUint32(data[20:24], 4096)  // MaxUnmapLBACount
	binary.BigEndian.PutUint32(data[24:28], 256)   // MaxUnmapBlockDescCount
	binary.BigEndian.PutUint32(data[28:32], 512)   // OptimalUnmapGranularity

	bl, err := ParseVPDBlockLimits(goodResult(data))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bl.MaxCompareAndWriteLength != 0x20 {
		t.Errorf("MaxCompareAndWriteLength = %d, want 32", bl.MaxCompareAndWriteLength)
	}
	if bl.OptimalTransferLengthGranularity != 128 {
		t.Errorf("OptimalTransferLengthGranularity = %d, want 128", bl.OptimalTransferLengthGranularity)
	}
	if bl.MaxTransferLength != 65536 {
		t.Errorf("MaxTransferLength = %d, want 65536", bl.MaxTransferLength)
	}
	if bl.OptimalTransferLength != 8192 {
		t.Errorf("OptimalTransferLength = %d, want 8192", bl.OptimalTransferLength)
	}
	if bl.MaxUnmapLBACount != 4096 {
		t.Errorf("MaxUnmapLBACount = %d, want 4096", bl.MaxUnmapLBACount)
	}
	if bl.MaxUnmapBlockDescCount != 256 {
		t.Errorf("MaxUnmapBlockDescCount = %d, want 256", bl.MaxUnmapBlockDescCount)
	}
	if bl.OptimalUnmapGranularity != 512 {
		t.Errorf("OptimalUnmapGranularity = %d, want 512", bl.OptimalUnmapGranularity)
	}

	// Short data
	_, err = ParseVPDBlockLimits(goodResult([]byte{0x00, 0xB0, 0x00, 0x04}))
	if err == nil {
		t.Fatal("expected error for short block limits data")
	}
}

func TestParseVPDBlockCharacteristics(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		wantRotation uint16
		wantForm     uint8
	}{
		{
			name: "SSD (non-rotating)",
			data: func() []byte {
				d := make([]byte, 64)
				d[1] = 0xB1
				binary.BigEndian.PutUint16(d[2:4], 60)
				binary.BigEndian.PutUint16(d[4:6], 1) // non-rotating = SSD
				d[7] = 0x02                            // 2.5 inch
				return d
			}(),
			wantRotation: 1,
			wantForm:     0x02,
		},
		{
			name: "HDD 7200 RPM",
			data: func() []byte {
				d := make([]byte, 64)
				d[1] = 0xB1
				binary.BigEndian.PutUint16(d[2:4], 60)
				binary.BigEndian.PutUint16(d[4:6], 7200)
				d[7] = 0x01 // 3.5 inch
				return d
			}(),
			wantRotation: 7200,
			wantForm:     0x01,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bc, err := ParseVPDBlockCharacteristics(goodResult(tt.data))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if bc.MediumRotationRate != tt.wantRotation {
				t.Errorf("MediumRotationRate = %d, want %d", bc.MediumRotationRate, tt.wantRotation)
			}
			if bc.NominalFormFactor != tt.wantForm {
				t.Errorf("NominalFormFactor = %d, want %d", bc.NominalFormFactor, tt.wantForm)
			}
		})
	}

	// Short data
	_, err := ParseVPDBlockCharacteristics(goodResult([]byte{0x00, 0xB1, 0x00, 0x01, 0x00}))
	if err == nil {
		t.Fatal("expected error for short block characteristics data")
	}
}

func TestParseVPDLogicalBlockProvisioning(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		wantLBPU  bool
		wantLBPWS bool
		wantLBPWS10 bool
		wantProvType uint8
		wantThresh   uint8
	}{
		{
			name: "thin provisioned with UNMAP",
			data: func() []byte {
				d := make([]byte, 8)
				d[1] = 0xB2
				binary.BigEndian.PutUint16(d[2:4], 4)
				d[4] = 0x05 // threshold exponent
				d[5] = 0xE0 // LBPU=1 (bit7), LBPWS=1 (bit6), LBPWS10=1 (bit5)
				d[6] = 0x02 // provisioning type = 2 (thin)
				return d
			}(),
			wantLBPU:     true,
			wantLBPWS:    true,
			wantLBPWS10:  true,
			wantProvType: 0x02,
			wantThresh:   0x05,
		},
		{
			name: "fully provisioned",
			data: func() []byte {
				d := make([]byte, 8)
				d[1] = 0xB2
				binary.BigEndian.PutUint16(d[2:4], 4)
				d[4] = 0x00
				d[5] = 0x00
				d[6] = 0x00
				return d
			}(),
			wantLBPU:     false,
			wantLBPWS:    false,
			wantLBPWS10:  false,
			wantProvType: 0x00,
			wantThresh:   0x00,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lbp, err := ParseVPDLogicalBlockProvisioning(goodResult(tt.data))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if lbp.LBPU != tt.wantLBPU {
				t.Errorf("LBPU = %v, want %v", lbp.LBPU, tt.wantLBPU)
			}
			if lbp.LBPWS != tt.wantLBPWS {
				t.Errorf("LBPWS = %v, want %v", lbp.LBPWS, tt.wantLBPWS)
			}
			if lbp.LBPWS10 != tt.wantLBPWS10 {
				t.Errorf("LBPWS10 = %v, want %v", lbp.LBPWS10, tt.wantLBPWS10)
			}
			if lbp.ProvisioningType != tt.wantProvType {
				t.Errorf("ProvisioningType = %d, want %d", lbp.ProvisioningType, tt.wantProvType)
			}
			if lbp.ThresholdExponent != tt.wantThresh {
				t.Errorf("ThresholdExponent = %d, want %d", lbp.ThresholdExponent, tt.wantThresh)
			}
		})
	}
}
