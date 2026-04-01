package scsi

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func TestSynchronizeCache10(t *testing.T) {
	tests := []struct {
		name       string
		lun        uint64
		lba        uint32
		blocks     uint16
		opts       []Option
		wantCDB    [16]byte
	}{
		{
			name:   "zero LBA and blocks",
			lun:    0,
			lba:    0,
			blocks: 0,
			wantCDB: [16]byte{OpSynchronizeCache10},
		},
		{
			name:   "with IMMED",
			lun:    0,
			lba:    0,
			blocks: 0,
			opts:   []Option{WithImmed()},
			wantCDB: [16]byte{OpSynchronizeCache10, 0x02},
		},
		{
			name:   "specific LBA and blocks",
			lun:    2,
			lba:    1000,
			blocks: 64,
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = OpSynchronizeCache10
				binary.BigEndian.PutUint32(cdb[2:6], 1000)
				binary.BigEndian.PutUint16(cdb[7:9], 64)
				return cdb
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := SynchronizeCache10(tt.lun, tt.lba, tt.blocks, tt.opts...)
			if cmd.CDB != tt.wantCDB {
				t.Errorf("CDB = %X, want %X", cmd.CDB, tt.wantCDB)
			}
			if cmd.Read {
				t.Error("Read should be false")
			}
			if cmd.Write {
				t.Error("Write should be false")
			}
			if cmd.ExpectedDataTransferLen != 0 {
				t.Errorf("ExpectedDataTransferLen = %d, want 0", cmd.ExpectedDataTransferLen)
			}
			if cmd.LUN != tt.lun {
				t.Errorf("LUN = %d, want %d", cmd.LUN, tt.lun)
			}
		})
	}
}

func TestSynchronizeCache16(t *testing.T) {
	tests := []struct {
		name    string
		lun     uint64
		lba     uint64
		blocks  uint32
		opts    []Option
		wantCDB [16]byte
	}{
		{
			name: "64-bit LBA",
			lun:  1,
			lba:  0x100000000,
			blocks: 1024,
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = OpSynchronizeCache16
				binary.BigEndian.PutUint64(cdb[2:10], 0x100000000)
				binary.BigEndian.PutUint32(cdb[10:14], 1024)
				return cdb
			}(),
		},
		{
			name: "with IMMED",
			lun:  0,
			lba:  0,
			blocks: 0,
			opts: []Option{WithImmed()},
			wantCDB: [16]byte{OpSynchronizeCache16, 0x02},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := SynchronizeCache16(tt.lun, tt.lba, tt.blocks, tt.opts...)
			if cmd.CDB != tt.wantCDB {
				t.Errorf("CDB = %X, want %X", cmd.CDB, tt.wantCDB)
			}
			if cmd.Read {
				t.Error("Read should be false")
			}
			if cmd.Write {
				t.Error("Write should be false")
			}
			if cmd.ExpectedDataTransferLen != 0 {
				t.Errorf("ExpectedDataTransferLen = %d, want 0", cmd.ExpectedDataTransferLen)
			}
			if cmd.LUN != tt.lun {
				t.Errorf("LUN = %d, want %d", cmd.LUN, tt.lun)
			}
		})
	}
}

func TestWriteSame10(t *testing.T) {
	dummyData := bytes.NewReader([]byte{0xAA})

	tests := []struct {
		name      string
		lun       uint64
		lba       uint32
		blocks    uint16
		blockSize uint32
		data      io.Reader
		opts      []Option
		wantCDB   [16]byte
		wantWrite bool
		wantXfer  uint32
	}{
		{
			name:      "with UNMAP flag",
			lun:       0,
			lba:       100,
			blocks:    50,
			blockSize: 512,
			data:      dummyData,
			opts:      []Option{WithUnmap()},
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = OpWriteSame10
				cdb[1] = 0x08 // UNMAP
				binary.BigEndian.PutUint32(cdb[2:6], 100)
				binary.BigEndian.PutUint16(cdb[7:9], 50)
				return cdb
			}(),
			wantWrite: true,
			wantXfer:  512,
		},
		{
			name:      "with ANCHOR flag",
			lun:       0,
			lba:       0,
			blocks:    10,
			blockSize: 512,
			data:      dummyData,
			opts:      []Option{WithAnchor()},
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = OpWriteSame10
				cdb[1] = 0x04 // ANCHOR
				binary.BigEndian.PutUint16(cdb[7:9], 10)
				return cdb
			}(),
			wantWrite: true,
			wantXfer:  512,
		},
		{
			name:      "with NDOB (no data)",
			lun:       1,
			lba:       0,
			blocks:    100,
			blockSize: 512,
			data:      dummyData,
			opts:      []Option{WithNDOB()},
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = OpWriteSame10
				cdb[1] = 0x01 // NDOB
				binary.BigEndian.PutUint16(cdb[7:9], 100)
				return cdb
			}(),
			wantWrite: false,
			wantXfer:  0,
		},
		{
			name:      "normal with data",
			lun:       0,
			lba:       200,
			blocks:    25,
			blockSize: 4096,
			data:      dummyData,
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = OpWriteSame10
				binary.BigEndian.PutUint32(cdb[2:6], 200)
				binary.BigEndian.PutUint16(cdb[7:9], 25)
				return cdb
			}(),
			wantWrite: true,
			wantXfer:  4096,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := WriteSame10(tt.lun, tt.lba, tt.blocks, tt.blockSize, tt.data, tt.opts...)
			if cmd.CDB != tt.wantCDB {
				t.Errorf("CDB = %X, want %X", cmd.CDB, tt.wantCDB)
			}
			if cmd.Write != tt.wantWrite {
				t.Errorf("Write = %v, want %v", cmd.Write, tt.wantWrite)
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

func TestWriteSame16(t *testing.T) {
	dummyData := bytes.NewReader([]byte{0xBB})

	tests := []struct {
		name      string
		lba       uint64
		blocks    uint32
		blockSize uint32
		opts      []Option
		wantCDB   [16]byte
		wantWrite bool
		wantXfer  uint32
	}{
		{
			name:      "64-bit LBA with all flags",
			lba:       0x200000000,
			blocks:    0xFFFFFFFF,
			blockSize: 512,
			opts:      []Option{WithUnmap(), WithAnchor(), WithNDOB()},
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = OpWriteSame16
				cdb[1] = 0x08 | 0x04 | 0x01 // UNMAP + ANCHOR + NDOB
				binary.BigEndian.PutUint64(cdb[2:10], 0x200000000)
				binary.BigEndian.PutUint32(cdb[10:14], 0xFFFFFFFF)
				return cdb
			}(),
			wantWrite: false, // NDOB means no data
			wantXfer:  0,
		},
		{
			name:      "normal write same 16",
			lba:       1024,
			blocks:    256,
			blockSize: 4096,
			wantCDB: func() [16]byte {
				var cdb [16]byte
				cdb[0] = OpWriteSame16
				binary.BigEndian.PutUint64(cdb[2:10], 1024)
				binary.BigEndian.PutUint32(cdb[10:14], 256)
				return cdb
			}(),
			wantWrite: true,
			wantXfer:  4096,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := WriteSame16(0, tt.lba, tt.blocks, tt.blockSize, dummyData, tt.opts...)
			if cmd.CDB != tt.wantCDB {
				t.Errorf("CDB = %X, want %X", cmd.CDB, tt.wantCDB)
			}
			if cmd.Write != tt.wantWrite {
				t.Errorf("Write = %v, want %v", cmd.Write, tt.wantWrite)
			}
			if cmd.ExpectedDataTransferLen != tt.wantXfer {
				t.Errorf("ExpectedDataTransferLen = %d, want %d", cmd.ExpectedDataTransferLen, tt.wantXfer)
			}
		})
	}
}

func TestUnmap(t *testing.T) {
	t.Run("single descriptor", func(t *testing.T) {
		descs := []UnmapBlockDescriptor{
			{LBA: 1000, BlockCount: 64},
		}
		cmd := Unmap(0, descs)

		if cmd.CDB[0] != OpUnmap {
			t.Errorf("CDB[0] = 0x%02X, want 0x%02X", cmd.CDB[0], OpUnmap)
		}

		// Parameter list length in bytes 7-8 = 8 (header) + 16 (1 desc) = 24
		paramLen := binary.BigEndian.Uint16(cmd.CDB[7:9])
		if paramLen != 24 {
			t.Errorf("parameter list length = %d, want 24", paramLen)
		}

		if !cmd.Write {
			t.Error("Write should be true")
		}
		if cmd.ExpectedDataTransferLen != 24 {
			t.Errorf("ExpectedDataTransferLen = %d, want 24", cmd.ExpectedDataTransferLen)
		}

		// Read the parameter data
		data, err := io.ReadAll(cmd.Data)
		if err != nil {
			t.Fatalf("reading data: %v", err)
		}
		if len(data) != 24 {
			t.Fatalf("data length = %d, want 24", len(data))
		}

		// Header: data length (bytes 0-1) = total - 2 = 22
		dataLen := binary.BigEndian.Uint16(data[0:2])
		if dataLen != 22 {
			t.Errorf("data length field = %d, want 22", dataLen)
		}

		// Block descriptor data length (bytes 2-3) = 16
		bdLen := binary.BigEndian.Uint16(data[2:4])
		if bdLen != 16 {
			t.Errorf("BD data length = %d, want 16", bdLen)
		}

		// Reserved bytes 4-7 should be zero
		for i := 4; i < 8; i++ {
			if data[i] != 0 {
				t.Errorf("header byte %d = 0x%02X, want 0x00", i, data[i])
			}
		}

		// Descriptor: LBA at offset 8, block count at offset 16
		descLBA := binary.BigEndian.Uint64(data[8:16])
		if descLBA != 1000 {
			t.Errorf("descriptor LBA = %d, want 1000", descLBA)
		}
		descCount := binary.BigEndian.Uint32(data[16:20])
		if descCount != 64 {
			t.Errorf("descriptor block count = %d, want 64", descCount)
		}

		// Reserved bytes 20-23
		for i := 20; i < 24; i++ {
			if data[i] != 0 {
				t.Errorf("descriptor reserved byte %d = 0x%02X, want 0x00", i, data[i])
			}
		}
	})

	t.Run("multiple descriptors", func(t *testing.T) {
		descs := []UnmapBlockDescriptor{
			{LBA: 0, BlockCount: 100},
			{LBA: 500, BlockCount: 200},
			{LBA: 1000, BlockCount: 300},
		}
		cmd := Unmap(1, descs)

		// 8 header + 3*16 = 56 bytes
		paramLen := binary.BigEndian.Uint16(cmd.CDB[7:9])
		if paramLen != 56 {
			t.Errorf("parameter list length = %d, want 56", paramLen)
		}

		data, err := io.ReadAll(cmd.Data)
		if err != nil {
			t.Fatalf("reading data: %v", err)
		}
		if len(data) != 56 {
			t.Fatalf("data length = %d, want 56", len(data))
		}

		// Header data length = 56 - 2 = 54
		dataLen := binary.BigEndian.Uint16(data[0:2])
		if dataLen != 54 {
			t.Errorf("data length field = %d, want 54", dataLen)
		}

		// BD data length = 3*16 = 48
		bdLen := binary.BigEndian.Uint16(data[2:4])
		if bdLen != 48 {
			t.Errorf("BD data length = %d, want 48", bdLen)
		}

		// Verify third descriptor at offset 8 + 2*16 = 40
		descLBA := binary.BigEndian.Uint64(data[40:48])
		if descLBA != 1000 {
			t.Errorf("descriptor[2] LBA = %d, want 1000", descLBA)
		}
		descCount := binary.BigEndian.Uint32(data[48:52])
		if descCount != 300 {
			t.Errorf("descriptor[2] block count = %d, want 300", descCount)
		}

		if cmd.LUN != 1 {
			t.Errorf("LUN = %d, want 1", cmd.LUN)
		}
	})

	t.Run("empty descriptors", func(t *testing.T) {
		cmd := Unmap(0, nil)

		// 8 header + 0 descriptors = 8 bytes
		paramLen := binary.BigEndian.Uint16(cmd.CDB[7:9])
		if paramLen != 8 {
			t.Errorf("parameter list length = %d, want 8", paramLen)
		}

		data, err := io.ReadAll(cmd.Data)
		if err != nil {
			t.Fatalf("reading data: %v", err)
		}
		if len(data) != 8 {
			t.Fatalf("data length = %d, want 8", len(data))
		}

		// data length = 8 - 2 = 6
		dataLen := binary.BigEndian.Uint16(data[0:2])
		if dataLen != 6 {
			t.Errorf("data length field = %d, want 6", dataLen)
		}

		// BD data length = 0
		bdLen := binary.BigEndian.Uint16(data[2:4])
		if bdLen != 0 {
			t.Errorf("BD data length = %d, want 0", bdLen)
		}
	})

	t.Run("with ANCHOR flag", func(t *testing.T) {
		cmd := Unmap(0, nil, WithAnchor())
		if cmd.CDB[1] != 0x01 {
			t.Errorf("CDB[1] = 0x%02X, want 0x01 (ANCHOR)", cmd.CDB[1])
		}
	})
}
