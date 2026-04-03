package pdu

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// AHSType identifies the type of an Additional Header Segment.
// Defined in RFC 7143 Section 11.2.1.2.
type AHSType uint8

const (
	// AHSExtendedCDB carries extended CDB bytes beyond the 16-byte CDB in the BHS.
	AHSExtendedCDB AHSType = 1
	// AHSBidiReadDataLen carries the expected bidirectional read data length.
	AHSBidiReadDataLen AHSType = 2
)

// AHS represents a single Additional Header Segment.
// Each AHS contains a type identifier and variable-length data.
type AHS struct {
	Type AHSType
	Data []byte
}

// ahsHeaderLen is the fixed overhead per AHS: 2 bytes AHSLength + 1 byte AHSType + 1 byte type-specific.
const ahsHeaderLen = 4

// MarshalAHS encodes AHS segments into wire format. Each segment is padded
// to a 4-byte boundary. Returns nil if segments is nil or empty.
func MarshalAHS(segments []AHS) []byte {
	if len(segments) == 0 {
		return nil
	}
	var out []byte
	for _, seg := range segments {
		// AHSLength: total length of this AHS in bytes, not including padding,
		// measured from after the AHSLength field itself.
		// The wire layout is: [AHSLength(2)] [AHSType(1)] [TypeSpecific(1)] [Data...]
		// AHSLength = 1 (type) + 1 (type-specific) + len(data)
		ahsLen := uint16(2 + len(seg.Data))

		// Build segment header
		var hdr [ahsHeaderLen]byte
		binary.BigEndian.PutUint16(hdr[0:2], ahsLen)
		hdr[2] = byte(seg.Type)
		hdr[3] = 0 // type-specific byte, reserved for most types

		out = append(out, hdr[:]...)
		out = append(out, seg.Data...)

		// Pad to 4-byte boundary
		totalLen := uint32(ahsHeaderLen + len(seg.Data))
		pad := PadLen(totalLen)
		for range pad {
			out = append(out, 0)
		}
	}
	return out
}

// UnmarshalAHS decodes AHS segments from wire format data. Returns nil and
// no error for nil or empty input. Returns an error if the data is truncated
// or otherwise malformed.
func UnmarshalAHS(data []byte) ([]AHS, error) {
	if len(data) == 0 {
		return nil, nil
	}

	var segments []AHS
	offset := 0
	for offset < len(data) {
		if offset+ahsHeaderLen > len(data) {
			return nil, errors.New("pdu: truncated AHS header")
		}

		ahsLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		ahsType := AHSType(data[offset+2])
		// ahsLen includes the type byte and type-specific byte, so data length
		// is ahsLen - 2
		if ahsLen < 2 {
			return nil, errors.New("pdu: invalid AHS length")
		}
		dataLen := ahsLen - 2
		if dataLen > 16384 {
			return nil, fmt.Errorf("pdu: AHS data length %d exceeds reasonable maximum", dataLen)
		}

		// Validate AHS type. Per RFC 7143 Section 11.2.1.2, only types 1
		// (Extended CDB) and 2 (Bidirectional Read Data Length) are defined.
		// Accept unknown types for forward compatibility but note them.
		switch ahsType {
		case AHSExtendedCDB, AHSBidiReadDataLen:
			// Known types — proceed normally.
		default:
			// Unknown AHS type. Future types may be added by the spec.
		}

		dataStart := offset + ahsHeaderLen
		dataEnd := dataStart + dataLen

		if dataEnd > len(data) {
			return nil, errors.New("pdu: AHS data exceeds available bytes")
		}

		segData := make([]byte, dataLen)
		copy(segData, data[dataStart:dataEnd])

		segments = append(segments, AHS{
			Type: ahsType,
			Data: segData,
		})

		// Advance past this segment including padding to 4-byte boundary
		totalLen := uint32(ahsHeaderLen + dataLen)
		offset += int(totalLen + PadLen(totalLen))
	}

	return segments, nil
}
