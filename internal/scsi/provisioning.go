package scsi

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/rkujawa/uiscsi/internal/session"
)

// SynchronizeCache10 returns a SYNCHRONIZE CACHE(10) command (opcode 0x35).
// It requests the target to flush its volatile cache for the specified LBA
// range to non-volatile storage. WithImmed() makes the command return
// immediately before the flush completes.
func SynchronizeCache10(lun uint64, lba uint32, blocks uint16, opts ...Option) session.Command {
	cfg := applyOptions(opts)
	var cmd session.Command
	cmd.CDB[0] = OpSynchronizeCache10
	if cfg.immed {
		cmd.CDB[1] |= 0x02
	}
	binary.BigEndian.PutUint32(cmd.CDB[2:6], lba)
	binary.BigEndian.PutUint16(cmd.CDB[7:9], blocks)
	cmd.LUN = lun
	return cmd
}

// SynchronizeCache16 returns a SYNCHRONIZE CACHE(16) command (opcode 0x91).
// It supports 64-bit LBA and 32-bit block count for large devices.
func SynchronizeCache16(lun uint64, lba uint64, blocks uint32, opts ...Option) session.Command {
	cfg := applyOptions(opts)
	var cmd session.Command
	cmd.CDB[0] = OpSynchronizeCache16
	if cfg.immed {
		cmd.CDB[1] |= 0x02
	}
	binary.BigEndian.PutUint64(cmd.CDB[2:10], lba)
	binary.BigEndian.PutUint32(cmd.CDB[10:14], blocks)
	cmd.LUN = lun
	return cmd
}

// WriteSame10 returns a WRITE SAME(10) command (opcode 0x41).
// It writes the same single block of data to the specified LBA range.
// WithUnmap() deallocates the blocks (thin provisioning).
// WithAnchor() anchors the blocks.
// WithNDOB() uses no data-out buffer (the target zeros or deallocates).
func WriteSame10(lun uint64, lba uint32, blocks uint16, blockSize uint32, data io.Reader, opts ...Option) session.Command {
	cfg := applyOptions(opts)
	var cmd session.Command
	cmd.CDB[0] = OpWriteSame10
	if cfg.unmap {
		cmd.CDB[1] |= 0x08
	}
	if cfg.anchor {
		cmd.CDB[1] |= 0x04
	}
	if cfg.ndob {
		cmd.CDB[1] |= 0x01
	}
	binary.BigEndian.PutUint32(cmd.CDB[2:6], lba)
	binary.BigEndian.PutUint16(cmd.CDB[7:9], blocks)
	cmd.LUN = lun

	if cfg.ndob {
		// No Data-Out Buffer: no data transfer
		return cmd
	}
	cmd.Write = true
	cmd.Data = data
	cmd.ExpectedDataTransferLen = blockSize
	return cmd
}

// WriteSame16 returns a WRITE SAME(16) command (opcode 0x93).
// It supports 64-bit LBA and 32-bit block count.
func WriteSame16(lun uint64, lba uint64, blocks uint32, blockSize uint32, data io.Reader, opts ...Option) session.Command {
	cfg := applyOptions(opts)
	var cmd session.Command
	cmd.CDB[0] = OpWriteSame16
	if cfg.unmap {
		cmd.CDB[1] |= 0x08
	}
	if cfg.anchor {
		cmd.CDB[1] |= 0x04
	}
	if cfg.ndob {
		cmd.CDB[1] |= 0x01
	}
	binary.BigEndian.PutUint64(cmd.CDB[2:10], lba)
	binary.BigEndian.PutUint32(cmd.CDB[10:14], blocks)
	cmd.LUN = lun

	if cfg.ndob {
		return cmd
	}
	cmd.Write = true
	cmd.Data = data
	cmd.ExpectedDataTransferLen = blockSize
	return cmd
}

// UnmapBlockDescriptor describes a single LBA range to deallocate.
type UnmapBlockDescriptor struct {
	LBA        uint64
	BlockCount uint32
}

// Unmap returns an UNMAP command (opcode 0x42).
// It deallocates the specified LBA ranges (thin provisioning).
// The parameter data is serialized as an 8-byte header followed by
// 16-byte descriptors per SBC-3.
func Unmap(lun uint64, descriptors []UnmapBlockDescriptor, opts ...Option) session.Command {
	cfg := applyOptions(opts)

	bdDataLen := len(descriptors) * 16
	paramListLen := 8 + bdDataLen

	// Build parameter data
	paramData := make([]byte, paramListLen)
	// Header: bytes 0-1 = total data length - 2
	binary.BigEndian.PutUint16(paramData[0:2], uint16(paramListLen-2))
	// Header: bytes 2-3 = block descriptor data length
	binary.BigEndian.PutUint16(paramData[2:4], uint16(bdDataLen))
	// Header: bytes 4-7 = reserved (zero)

	// Descriptors: 16 bytes each (8-byte LBA + 4-byte count + 4 reserved)
	for i, d := range descriptors {
		off := 8 + i*16
		binary.BigEndian.PutUint64(paramData[off:off+8], d.LBA)
		binary.BigEndian.PutUint32(paramData[off+8:off+12], d.BlockCount)
		// off+12 to off+16 = reserved (zero)
	}

	var cmd session.Command
	cmd.CDB[0] = OpUnmap
	if cfg.anchor {
		cmd.CDB[1] |= 0x01
	}
	binary.BigEndian.PutUint16(cmd.CDB[7:9], uint16(paramListLen))
	cmd.Write = true
	cmd.Data = bytes.NewReader(paramData)
	cmd.ExpectedDataTransferLen = uint32(paramListLen)
	cmd.LUN = lun
	return cmd
}
