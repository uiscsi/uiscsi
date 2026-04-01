package scsi

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestRead10(t *testing.T) {
	tests := []struct {
		name      string
		lun       uint64
		lba       uint32
		blocks    uint16
		blockSize uint32
		opts      []Option
		wantCDB   [16]byte
		wantXfer  uint32
	}{
		{
			name:      "simple read lba=0 blocks=1 bs=512",
			lun:       0,
			lba:       0,
			blocks:    1,
			blockSize: 512,
			wantCDB:   [16]byte{0x28, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00},
			wantXfer:  512,
		},
		{
			name:      "high LBA with FUA",
			lun:       1,
			lba:       0x01000000,
			blocks:    256,
			blockSize: 4096,
			opts:      []Option{WithFUA()},
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = 0x28
				cdb[1] = 0x08 // FUA bit 3
				binary.BigEndian.PutUint32(cdb[2:6], 0x01000000)
				binary.BigEndian.PutUint16(cdb[7:9], 256)
				return cdb
			}(),
			wantXfer: 1048576,
		},
		{
			name:      "DPO only",
			lun:       0,
			lba:       100,
			blocks:    10,
			blockSize: 512,
			opts:      []Option{WithDPO()},
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = 0x28
				cdb[1] = 0x10 // DPO bit 4
				binary.BigEndian.PutUint32(cdb[2:6], 100)
				binary.BigEndian.PutUint16(cdb[7:9], 10)
				return cdb
			}(),
			wantXfer: 5120,
		},
		{
			name:      "FUA and DPO combined",
			lun:       0,
			lba:       500,
			blocks:    32,
			blockSize: 512,
			opts:      []Option{WithFUA(), WithDPO()},
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = 0x28
				cdb[1] = 0x18 // FUA + DPO
				binary.BigEndian.PutUint32(cdb[2:6], 500)
				binary.BigEndian.PutUint16(cdb[7:9], 32)
				return cdb
			}(),
			wantXfer: 16384,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := Read10(tt.lun, tt.lba, tt.blocks, tt.blockSize, tt.opts...)
			if cmd.CDB != tt.wantCDB {
				t.Errorf("CDB = %X, want %X", cmd.CDB, tt.wantCDB)
			}
			if !cmd.Read {
				t.Error("Read should be true")
			}
			if cmd.Write {
				t.Error("Write should be false")
			}
			if cmd.ExpectedDataTransferLen != tt.wantXfer {
				t.Errorf("ExpectedDataTransferLen = %d, want %d", cmd.ExpectedDataTransferLen, tt.wantXfer)
			}
			if cmd.LUN != tt.lun {
				t.Errorf("LUN = %d, want %d", cmd.LUN, tt.lun)
			}
		})
	}
}

func TestRead16(t *testing.T) {
	tests := []struct {
		name      string
		lun       uint64
		lba       uint64
		blocks    uint32
		blockSize uint32
		opts      []Option
		wantCDB   [16]byte
		wantXfer  uint32
	}{
		{
			name:      "64-bit LBA beyond 32-bit range",
			lun:       0,
			lba:       0x0000000100000000,
			blocks:    1,
			blockSize: 512,
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = 0x88
				binary.BigEndian.PutUint64(cdb[2:10], 0x0000000100000000)
				binary.BigEndian.PutUint32(cdb[10:14], 1)
				return cdb
			}(),
			wantXfer: 512,
		},
		{
			name:      "with FUA",
			lun:       2,
			lba:       1024,
			blocks:    64,
			blockSize: 4096,
			opts:      []Option{WithFUA()},
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = 0x88
				cdb[1] = 0x08 // FUA
				binary.BigEndian.PutUint64(cdb[2:10], 1024)
				binary.BigEndian.PutUint32(cdb[10:14], 64)
				return cdb
			}(),
			wantXfer: 262144,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := Read16(tt.lun, tt.lba, tt.blocks, tt.blockSize, tt.opts...)
			if cmd.CDB != tt.wantCDB {
				t.Errorf("CDB = %X, want %X", cmd.CDB, tt.wantCDB)
			}
			if !cmd.Read {
				t.Error("Read should be true")
			}
			if cmd.Write {
				t.Error("Write should be false")
			}
			if cmd.ExpectedDataTransferLen != tt.wantXfer {
				t.Errorf("ExpectedDataTransferLen = %d, want %d", cmd.ExpectedDataTransferLen, tt.wantXfer)
			}
			if cmd.LUN != tt.lun {
				t.Errorf("LUN = %d, want %d", cmd.LUN, tt.lun)
			}
		})
	}
}

func TestWrite10(t *testing.T) {
	tests := []struct {
		name      string
		lun       uint64
		lba       uint32
		blocks    uint16
		blockSize uint32
		opts      []Option
		wantCDB   [16]byte
		wantXfer  uint32
	}{
		{
			name:      "basic write",
			lun:       0,
			lba:       100,
			blocks:    8,
			blockSize: 512,
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = 0x2A
				binary.BigEndian.PutUint32(cdb[2:6], 100)
				binary.BigEndian.PutUint16(cdb[7:9], 8)
				return cdb
			}(),
			wantXfer: 4096,
		},
		{
			name:      "write with FUA",
			lun:       1,
			lba:       200,
			blocks:    16,
			blockSize: 4096,
			opts:      []Option{WithFUA()},
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = 0x2A
				cdb[1] = 0x08 // FUA
				binary.BigEndian.PutUint32(cdb[2:6], 200)
				binary.BigEndian.PutUint16(cdb[7:9], 16)
				return cdb
			}(),
			wantXfer: 65536,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := bytes.NewReader(make([]byte, tt.wantXfer))
			cmd := Write10(tt.lun, tt.lba, tt.blocks, tt.blockSize, data, tt.opts...)
			if cmd.CDB != tt.wantCDB {
				t.Errorf("CDB = %X, want %X", cmd.CDB, tt.wantCDB)
			}
			if cmd.Read {
				t.Error("Read should be false")
			}
			if !cmd.Write {
				t.Error("Write should be true")
			}
			if cmd.Data == nil {
				t.Error("Data should not be nil")
			}
			if cmd.ExpectedDataTransferLen != tt.wantXfer {
				t.Errorf("ExpectedDataTransferLen = %d, want %d", cmd.ExpectedDataTransferLen, tt.wantXfer)
			}
			if cmd.LUN != tt.lun {
				t.Errorf("LUN = %d, want %d", cmd.LUN, tt.lun)
			}
		})
	}
}

func TestWrite16(t *testing.T) {
	data := bytes.NewReader(make([]byte, 32768))
	cmd := Write16(2, 0x0000000100000000, 64, 512, data, WithFUA())

	wantCDB := func() [16]byte {
		var cdb [16]byte
		cdb[0] = 0x8A
		cdb[1] = 0x08 // FUA
		binary.BigEndian.PutUint64(cdb[2:10], 0x0000000100000000)
		binary.BigEndian.PutUint32(cdb[10:14], 64)
		return cdb
	}()

	if cmd.CDB != wantCDB {
		t.Errorf("CDB = %X, want %X", cmd.CDB, wantCDB)
	}
	if cmd.Read {
		t.Error("Read should be false")
	}
	if !cmd.Write {
		t.Error("Write should be true")
	}
	if cmd.Data == nil {
		t.Error("Data should not be nil")
	}
	if cmd.ExpectedDataTransferLen != 32768 {
		t.Errorf("ExpectedDataTransferLen = %d, want 32768", cmd.ExpectedDataTransferLen)
	}
	if cmd.LUN != 2 {
		t.Errorf("LUN = %d, want 2", cmd.LUN)
	}
}
