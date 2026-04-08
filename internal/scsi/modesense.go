package scsi

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/rkujawa/uiscsi/internal/session"
)

// ModeSense6Response holds the parsed MODE SENSE (6) response.
type ModeSense6Response struct {
	ModeDataLength        uint8
	MediumType            uint8
	DeviceSpecific        uint8
	BlockDescriptorLength uint8
	BlockDescriptors      []byte
	Pages                 []byte
	Raw                   []byte
}

// ModeSense10Response holds the parsed MODE SENSE (10) response.
type ModeSense10Response struct {
	ModeDataLength        uint16
	MediumType            uint8
	DeviceSpecific        uint8
	LongLBA               bool
	BlockDescriptorLength uint16
	BlockDescriptors      []byte
	Pages                 []byte
	Raw                   []byte
}

// ModeSense6 returns a MODE SENSE (6) command (opcode 0x1A).
func ModeSense6(lun uint64, pageCode, subpageCode, allocLen uint8, opts ...Option) session.Command {
	o := applyOptions(opts)

	var cmd session.Command
	cmd.CDB[0] = OpModeSense6
	if o.dbd {
		cmd.CDB[1] = 0x08 // DBD bit (bit 3)
	}
	cmd.CDB[2] = (o.pageControl << 6) | (pageCode & 0x3F)
	cmd.CDB[3] = subpageCode
	cmd.CDB[4] = allocLen
	cmd.Read = true
	cmd.ExpectedDataTransferLen = uint32(allocLen)
	cmd.LUN = lun
	return cmd
}

// ModeSense10 returns a MODE SENSE (10) command (opcode 0x5A).
func ModeSense10(lun uint64, pageCode, subpageCode uint8, allocLen uint16, opts ...Option) session.Command {
	o := applyOptions(opts)

	var cmd session.Command
	cmd.CDB[0] = OpModeSense10
	if o.dbd {
		cmd.CDB[1] = 0x08 // DBD bit (bit 3)
	}
	cmd.CDB[2] = (o.pageControl << 6) | (pageCode & 0x3F)
	cmd.CDB[3] = subpageCode
	binary.BigEndian.PutUint16(cmd.CDB[7:9], allocLen)
	cmd.Read = true
	cmd.ExpectedDataTransferLen = uint32(allocLen)
	cmd.LUN = lun
	return cmd
}

// ModeSelect6 returns a MODE SELECT (6) command (opcode 0x15).
// PF (Page Format) is set. data contains the mode parameter header +
// block descriptor + mode pages to send.
func ModeSelect6(lun uint64, data []byte) session.Command {
	var cmd session.Command
	cmd.CDB[0] = OpModeSelect6
	cmd.CDB[1] = 0x10 // PF bit (bit 4)
	cmd.CDB[4] = uint8(len(data))
	cmd.Write = true
	cmd.Data = bytes.NewReader(data)
	cmd.ExpectedDataTransferLen = uint32(len(data))
	cmd.LUN = lun
	return cmd
}

// ModeSelect10 returns a MODE SELECT (10) command (opcode 0x55).
// PF (Page Format) is set. data contains the mode parameter header +
// block descriptor + mode pages to send.
func ModeSelect10(lun uint64, data []byte) session.Command {
	var cmd session.Command
	cmd.CDB[0] = OpModeSelect10
	cmd.CDB[1] = 0x10 // PF bit (bit 4)
	binary.BigEndian.PutUint16(cmd.CDB[7:9], uint16(len(data)))
	cmd.Write = true
	cmd.Data = bytes.NewReader(data)
	cmd.ExpectedDataTransferLen = uint32(len(data))
	cmd.LUN = lun
	return cmd
}

// ParseModeSense6 parses a MODE SENSE (6) response.
func ParseModeSense6(result session.Result) (*ModeSense6Response, error) {
	data, err := checkResult(result)
	if err != nil {
		return nil, err
	}
	if len(data) < 4 {
		return nil, fmt.Errorf("scsi: MODE SENSE (6) response too short (%d bytes, need 4)", len(data))
	}

	bdl := data[3]
	resp := &ModeSense6Response{
		ModeDataLength:        data[0],
		MediumType:            data[1],
		DeviceSpecific:        data[2],
		BlockDescriptorLength: bdl,
		Raw:                   make([]byte, len(data)),
	}
	copy(resp.Raw, data)

	bdStart := uint16(4)
	bdEnd := bdStart + uint16(bdl)
	if bdEnd > uint16(len(data)) {
		bdEnd = uint16(len(data))
	}
	resp.BlockDescriptors = make([]byte, bdEnd-bdStart)
	copy(resp.BlockDescriptors, data[bdStart:bdEnd])

	if int(bdEnd) < len(data) {
		resp.Pages = make([]byte, len(data)-int(bdEnd))
		copy(resp.Pages, data[bdEnd:])
	}

	return resp, nil
}

// ParseModeSense10 parses a MODE SENSE (10) response.
func ParseModeSense10(result session.Result) (*ModeSense10Response, error) {
	data, err := checkResult(result)
	if err != nil {
		return nil, err
	}
	if len(data) < 8 {
		return nil, fmt.Errorf("scsi: MODE SENSE (10) response too short (%d bytes, need 8)", len(data))
	}

	bdl := binary.BigEndian.Uint16(data[6:8])
	resp := &ModeSense10Response{
		ModeDataLength:        binary.BigEndian.Uint16(data[0:2]),
		MediumType:            data[2],
		DeviceSpecific:        data[3],
		LongLBA:               data[4]&0x01 != 0,
		BlockDescriptorLength: bdl,
		Raw:                   make([]byte, len(data)),
	}
	copy(resp.Raw, data)

	bdStart := uint32(8)
	bdEnd := bdStart + uint32(bdl)
	if bdEnd > uint32(len(data)) {
		bdEnd = uint32(len(data))
	}
	resp.BlockDescriptors = make([]byte, bdEnd-bdStart)
	copy(resp.BlockDescriptors, data[bdStart:bdEnd])

	if bdEnd < uint32(len(data)) {
		resp.Pages = make([]byte, uint32(len(data))-bdEnd)
		copy(resp.Pages, data[bdEnd:])
	}

	return resp, nil
}
