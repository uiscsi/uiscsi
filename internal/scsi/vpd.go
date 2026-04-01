package scsi

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/rkujawa/uiscsi/internal/session"
)

// Designator represents a single identification descriptor from VPD page
// 0x83 (Device Identification) per SPC-4 Section 7.8.4.
type Designator struct {
	CodeSet    uint8  // bits 3-0 of byte 0
	ProtocolID uint8  // bits 7-4 of byte 0
	Type       uint8  // bits 3-0 of byte 1
	Association uint8 // bits 5-4 of byte 1
	Identifier []byte
}

// BlockLimits holds parsed VPD page 0xB0 (Block Limits) data per SBC-3.
type BlockLimits struct {
	MaxCompareAndWriteLength         uint8  // byte 5
	OptimalTransferLengthGranularity uint16 // bytes 6-7
	MaxTransferLength                uint32 // bytes 8-11
	OptimalTransferLength            uint32 // bytes 12-15
	MaxUnmapLBACount                 uint32 // bytes 20-23
	MaxUnmapBlockDescCount           uint32 // bytes 24-27
	OptimalUnmapGranularity          uint32 // bytes 28-31
	Raw                              []byte
}

// BlockCharacteristics holds parsed VPD page 0xB1 data per SBC-3.
type BlockCharacteristics struct {
	MediumRotationRate uint16 // bytes 4-5 (0x0000=not reported, 0x0001=non-rotating/SSD)
	NominalFormFactor  uint8  // byte 7 bits 3-0
	Raw                []byte
}

// LogicalBlockProvisioning holds parsed VPD page 0xB2 data per SBC-3.
type LogicalBlockProvisioning struct {
	ThresholdExponent uint8 // byte 4
	LBPU              bool  // byte 5 bit 7 (UNMAP command supported)
	LBPWS             bool  // byte 5 bit 6 (WRITE SAME with UNMAP supported)
	LBPWS10           bool  // byte 5 bit 5 (WRITE SAME(10) with UNMAP)
	ProvisioningType  uint8 // byte 6 bits 2-0
	Raw               []byte
}

// ParseVPDSupportedPages parses VPD page 0x00 (Supported VPD Pages) from
// a session.Result. Returns the list of supported page codes.
func ParseVPDSupportedPages(result session.Result) ([]uint8, error) {
	data, err := checkResult(result)
	if err != nil {
		return nil, err
	}
	if len(data) < 4 {
		return nil, fmt.Errorf("scsi: VPD supported pages response too short (%d bytes, need 4)", len(data))
	}
	pageLen := int(data[3])
	end := 4 + pageLen
	if end > len(data) {
		end = len(data)
	}
	pages := make([]uint8, end-4)
	copy(pages, data[4:end])
	return pages, nil
}

// ParseVPDSerialNumber parses VPD page 0x80 (Unit Serial Number) from a
// session.Result. Returns the serial number string with trailing spaces trimmed.
func ParseVPDSerialNumber(result session.Result) (string, error) {
	data, err := checkResult(result)
	if err != nil {
		return "", err
	}
	if len(data) < 4 {
		return "", fmt.Errorf("scsi: VPD serial number response too short (%d bytes, need 4)", len(data))
	}
	pageLen := int(data[3])
	end := 4 + pageLen
	if end > len(data) {
		end = len(data)
	}
	return strings.TrimRight(string(data[4:end]), " "), nil
}

// ParseVPDDeviceIdentification parses VPD page 0x83 (Device Identification)
// from a session.Result. Walks variable-length descriptors per SPC-4 Section
// 7.8.4 (Pitfall 5: each descriptor has a 4-byte header with identifier
// length in byte 3).
func ParseVPDDeviceIdentification(result session.Result) ([]Designator, error) {
	data, err := checkResult(result)
	if err != nil {
		return nil, err
	}
	if len(data) < 4 {
		return nil, fmt.Errorf("scsi: VPD device identification response too short (%d bytes, need 4)", len(data))
	}
	pageLen := int(binary.BigEndian.Uint16(data[2:4]))
	end := 4 + pageLen
	if end > len(data) {
		end = len(data)
	}

	var desigs []Designator
	offset := 4
	for offset+4 <= end {
		idLen := int(data[offset+3])
		if offset+4+idLen > end {
			return nil, fmt.Errorf("scsi: VPD 0x83 descriptor at offset %d: identifier length %d exceeds remaining data", offset, idLen)
		}
		d := Designator{
			CodeSet:    data[offset] & 0x0F,
			ProtocolID: (data[offset] >> 4) & 0x0F,
			Type:       data[offset+1] & 0x0F,
			Association: (data[offset+1] >> 4) & 0x03,
			Identifier: make([]byte, idLen),
		}
		copy(d.Identifier, data[offset+4:offset+4+idLen])
		desigs = append(desigs, d)
		offset += 4 + idLen
	}

	return desigs, nil
}

// ParseVPDBlockLimits parses VPD page 0xB0 (Block Limits) from a
// session.Result. Requires at least 32 bytes of response data.
func ParseVPDBlockLimits(result session.Result) (*BlockLimits, error) {
	data, err := checkResult(result)
	if err != nil {
		return nil, err
	}
	if len(data) < 32 {
		return nil, fmt.Errorf("scsi: VPD block limits response too short (%d bytes, need 32)", len(data))
	}
	bl := &BlockLimits{
		MaxCompareAndWriteLength:         data[5],
		OptimalTransferLengthGranularity: binary.BigEndian.Uint16(data[6:8]),
		MaxTransferLength:                binary.BigEndian.Uint32(data[8:12]),
		OptimalTransferLength:            binary.BigEndian.Uint32(data[12:16]),
		MaxUnmapLBACount:                 binary.BigEndian.Uint32(data[20:24]),
		MaxUnmapBlockDescCount:           binary.BigEndian.Uint32(data[24:28]),
		OptimalUnmapGranularity:          binary.BigEndian.Uint32(data[28:32]),
		Raw:                              make([]byte, len(data)),
	}
	copy(bl.Raw, data)
	return bl, nil
}

// ParseVPDBlockCharacteristics parses VPD page 0xB1 (Block Device
// Characteristics) from a session.Result. Requires at least 8 bytes.
func ParseVPDBlockCharacteristics(result session.Result) (*BlockCharacteristics, error) {
	data, err := checkResult(result)
	if err != nil {
		return nil, err
	}
	if len(data) < 8 {
		return nil, fmt.Errorf("scsi: VPD block characteristics response too short (%d bytes, need 8)", len(data))
	}
	bc := &BlockCharacteristics{
		MediumRotationRate: binary.BigEndian.Uint16(data[4:6]),
		NominalFormFactor:  data[7] & 0x0F,
		Raw:                make([]byte, len(data)),
	}
	copy(bc.Raw, data)
	return bc, nil
}

// ParseVPDLogicalBlockProvisioning parses VPD page 0xB2 (Logical Block
// Provisioning) from a session.Result. Requires at least 8 bytes.
func ParseVPDLogicalBlockProvisioning(result session.Result) (*LogicalBlockProvisioning, error) {
	data, err := checkResult(result)
	if err != nil {
		return nil, err
	}
	if len(data) < 8 {
		return nil, fmt.Errorf("scsi: VPD logical block provisioning response too short (%d bytes, need 8)", len(data))
	}
	lbp := &LogicalBlockProvisioning{
		ThresholdExponent: data[4],
		LBPU:              data[5]&0x80 != 0,
		LBPWS:             data[5]&0x40 != 0,
		LBPWS10:           data[5]&0x20 != 0,
		ProvisioningType:  data[6] & 0x07,
		Raw:               make([]byte, len(data)),
	}
	copy(lbp.Raw, data)
	return lbp, nil
}
