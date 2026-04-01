package scsi

import (
	"encoding/binary"
	"io"

	"github.com/rkujawa/uiscsi/internal/session"
)

// Read10 returns a READ(10) command (opcode 0x28) for reading blocks at
// a 32-bit LBA. Per SBC-3 Section 5.8.
func Read10(lun uint64, lba uint32, blocks uint16, blockSize uint32, opts ...Option) session.Command {
	o := applyOptions(opts)
	var cmd session.Command
	cmd.CDB[0] = OpRead10
	if o.dpo {
		cmd.CDB[1] |= 0x10
	}
	if o.fua {
		cmd.CDB[1] |= 0x08
	}
	binary.BigEndian.PutUint32(cmd.CDB[2:6], lba)
	binary.BigEndian.PutUint16(cmd.CDB[7:9], blocks)
	cmd.Read = true
	cmd.ExpectedDataTransferLen = uint32(blocks) * blockSize
	cmd.LUN = lun
	return cmd
}

// Read16 returns a READ(16) command (opcode 0x88) for reading blocks at
// a 64-bit LBA. Per SBC-3 Section 5.10.
func Read16(lun uint64, lba uint64, blocks uint32, blockSize uint32, opts ...Option) session.Command {
	o := applyOptions(opts)
	var cmd session.Command
	cmd.CDB[0] = OpRead16
	if o.dpo {
		cmd.CDB[1] |= 0x10
	}
	if o.fua {
		cmd.CDB[1] |= 0x08
	}
	binary.BigEndian.PutUint64(cmd.CDB[2:10], lba)
	binary.BigEndian.PutUint32(cmd.CDB[10:14], blocks)
	cmd.Read = true
	cmd.ExpectedDataTransferLen = blocks * blockSize
	cmd.LUN = lun
	return cmd
}

// Write10 returns a WRITE(10) command (opcode 0x2A) for writing blocks at
// a 32-bit LBA. data provides the write payload. Per SBC-3 Section 5.27.
func Write10(lun uint64, lba uint32, blocks uint16, blockSize uint32, data io.Reader, opts ...Option) session.Command {
	o := applyOptions(opts)
	var cmd session.Command
	cmd.CDB[0] = OpWrite10
	if o.dpo {
		cmd.CDB[1] |= 0x10
	}
	if o.fua {
		cmd.CDB[1] |= 0x08
	}
	binary.BigEndian.PutUint32(cmd.CDB[2:6], lba)
	binary.BigEndian.PutUint16(cmd.CDB[7:9], blocks)
	cmd.Write = true
	cmd.Data = data
	cmd.ExpectedDataTransferLen = uint32(blocks) * blockSize
	cmd.LUN = lun
	return cmd
}

// Write16 returns a WRITE(16) command (opcode 0x8A) for writing blocks at
// a 64-bit LBA. data provides the write payload. Per SBC-3 Section 5.29.
func Write16(lun uint64, lba uint64, blocks uint32, blockSize uint32, data io.Reader, opts ...Option) session.Command {
	o := applyOptions(opts)
	var cmd session.Command
	cmd.CDB[0] = OpWrite16
	if o.dpo {
		cmd.CDB[1] |= 0x10
	}
	if o.fua {
		cmd.CDB[1] |= 0x08
	}
	binary.BigEndian.PutUint64(cmd.CDB[2:10], lba)
	binary.BigEndian.PutUint32(cmd.CDB[10:14], blocks)
	cmd.Write = true
	cmd.Data = data
	cmd.ExpectedDataTransferLen = blocks * blockSize
	cmd.LUN = lun
	return cmd
}
