package scsi

import (
	"encoding/binary"
	"fmt"

	"github.com/rkujawa/uiscsi/internal/session"
)

// ReadCapacity10Response holds the parsed READ CAPACITY (10) response.
type ReadCapacity10Response struct {
	LastLBA   uint32
	BlockSize uint32
}

// ReadCapacity16Response holds the parsed READ CAPACITY (16) response.
type ReadCapacity16Response struct {
	LastLBA                      uint64
	BlockSize                    uint32
	ProtectionEnabled            bool
	ProtectionType               uint8
	LogicalBlocksPerPhysicalBlock uint8
	LowestAlignedLBA             uint16
	Raw                          []byte
}

// ReadCapacity10 returns a READ CAPACITY (10) command (opcode 0x25).
func ReadCapacity10(lun uint64) session.Command {
	var cmd session.Command
	cmd.CDB[0] = OpReadCapacity10
	cmd.Read = true
	cmd.ExpectedDataTransferLen = 8
	cmd.LUN = lun
	return cmd
}

// ParseReadCapacity10 parses a READ CAPACITY (10) response.
func ParseReadCapacity10(result session.Result) (*ReadCapacity10Response, error) {
	data, err := checkResult(result)
	if err != nil {
		return nil, err
	}
	if len(data) < 8 {
		return nil, fmt.Errorf("scsi: READ CAPACITY (10) response too short (%d bytes, need 8)", len(data))
	}
	return &ReadCapacity10Response{
		LastLBA:   binary.BigEndian.Uint32(data[0:4]),
		BlockSize: binary.BigEndian.Uint32(data[4:8]),
	}, nil
}

// ReadCapacity16 returns a READ CAPACITY (16) command using SERVICE ACTION IN
// (opcode 0x9E, service action 0x10). Per Pitfall 1: RC16 is a service action
// command, not a standalone opcode.
func ReadCapacity16(lun uint64, allocLen uint32) session.Command {
	var cmd session.Command
	cmd.CDB[0] = OpServiceActionIn16
	cmd.CDB[1] = 0x10 // READ CAPACITY (16) service action
	binary.BigEndian.PutUint32(cmd.CDB[10:14], allocLen)
	cmd.Read = true
	cmd.ExpectedDataTransferLen = allocLen
	cmd.LUN = lun
	return cmd
}

// ParseReadCapacity16 parses a READ CAPACITY (16) response.
func ParseReadCapacity16(result session.Result) (*ReadCapacity16Response, error) {
	data, err := checkResult(result)
	if err != nil {
		return nil, err
	}
	if len(data) < 32 {
		return nil, fmt.Errorf("scsi: READ CAPACITY (16) response too short (%d bytes, need 32)", len(data))
	}

	resp := &ReadCapacity16Response{
		LastLBA:                       binary.BigEndian.Uint64(data[0:8]),
		BlockSize:                     binary.BigEndian.Uint32(data[8:12]),
		ProtectionEnabled:             data[12]&0x01 != 0,
		ProtectionType:                (data[12] >> 1) & 0x07,
		LogicalBlocksPerPhysicalBlock: data[13] & 0x0F,
		LowestAlignedLBA:              binary.BigEndian.Uint16(data[14:16]) & 0x3FFF,
		Raw:                           make([]byte, len(data)),
	}
	copy(resp.Raw, data)

	return resp, nil
}
