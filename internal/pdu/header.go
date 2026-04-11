package pdu

import (
	"encoding/binary"
	"fmt"
)

// EncodeSAMLUN encodes a simple LUN number into the 8-byte SAM-5 LUN format
// used in iSCSI BHS LUN fields and REPORT LUNS responses. Uses single-level
// addressing (address mode 00b) where the LUN number goes into bytes 0-1.
// Panics if lun > 0xFFFF because SAM-5 single-level addressing supports at
// most 16-bit LUN values.
func EncodeSAMLUN(lun uint64) [8]byte {
	if lun > 0xFFFF {
		panic(fmt.Sprintf("pdu: EncodeSAMLUN: lun %d exceeds single-level addressing maximum (0xFFFF)", lun))
	}
	var encoded [8]byte
	binary.BigEndian.PutUint16(encoded[0:2], uint16(lun))
	return encoded
}

// DecodeSAMLUN decodes an 8-byte SAM-5 LUN encoding into a simple LUN number.
// Assumes single-level addressing (address mode 00b).
func DecodeSAMLUN(b []byte) uint64 {
	if len(b) < 2 {
		return 0
	}
	return uint64(binary.BigEndian.Uint16(b[0:2]))
}

// PDU is the interface implemented by all iSCSI PDU types.
// Each concrete type represents a specific opcode and provides BHS
// marshaling/unmarshaling plus access to the data segment.
type PDU interface {
	// Opcode returns the iSCSI opcode for this PDU type.
	Opcode() OpCode
	// MarshalBHS encodes the PDU fields into a 48-byte Basic Header Segment.
	MarshalBHS() ([BHSLength]byte, error)
	// DataSegment returns the PDU's data segment payload, or nil if empty.
	DataSegment() []byte
}

// Header contains BHS fields common to all iSCSI PDU types.
// Concrete PDU types embed Header and add opcode-specific fields.
type Header struct {
	Immediate        bool
	OpCode_          OpCode // underscore to avoid collision with Opcode() method
	Final            bool
	TotalAHSLength   uint8  // in 4-byte words
	DataSegmentLen   uint32 // 24-bit max (0x00FFFFFF)
	LUN              [8]byte
	InitiatorTaskTag uint32
}

// marshalHeader writes the common Header fields into the first 20 bytes of a BHS.
func (h *Header) marshalHeader(bhs []byte) error {
	bhs[0] = encodeOpcodeByte(h.OpCode_, h.Immediate)
	if h.Final {
		bhs[1] |= 0x80
	}
	bhs[4] = h.TotalAHSLength
	if err := encodeDataSegmentLength(bhs, h.DataSegmentLen); err != nil {
		return err
	}
	copy(bhs[8:16], h.LUN[:])
	binary.BigEndian.PutUint32(bhs[16:20], h.InitiatorTaskTag)
	return nil
}

// unmarshalHeader reads the common Header fields from a 48-byte BHS.
func (h *Header) unmarshalHeader(bhs [BHSLength]byte) {
	h.OpCode_, h.Immediate = decodeOpcodeByte(bhs[0])
	h.Final = bhs[1]&0x80 != 0
	h.TotalAHSLength = bhs[4]
	h.DataSegmentLen = decodeDataSegmentLength(bhs[:])
	copy(h.LUN[:], bhs[8:16])
	h.InitiatorTaskTag = binary.BigEndian.Uint32(bhs[16:20])
}

// DecodeBHS decodes a 48-byte Basic Header Segment into the appropriate
// concrete PDU type based on the opcode in byte 0. It returns the PDU
// interface or an error for unknown opcodes.
func DecodeBHS(bhs [BHSLength]byte) (PDU, error) {
	opcode, _ := decodeOpcodeByte(bhs[0])
	switch opcode {
	case OpNOPOut:
		p := &NOPOut{}
		p.UnmarshalBHS(bhs)
		return p, nil
	case OpSCSICommand:
		p := &SCSICommand{}
		p.UnmarshalBHS(bhs)
		return p, nil
	case OpTaskMgmtReq:
		p := &TaskMgmtReq{}
		p.UnmarshalBHS(bhs)
		return p, nil
	case OpLoginReq:
		p := &LoginReq{}
		p.UnmarshalBHS(bhs)
		return p, nil
	case OpTextReq:
		p := &TextReq{}
		p.UnmarshalBHS(bhs)
		return p, nil
	case OpDataOut:
		p := &DataOut{}
		p.UnmarshalBHS(bhs)
		return p, nil
	case OpLogoutReq:
		p := &LogoutReq{}
		p.UnmarshalBHS(bhs)
		return p, nil
	case OpSNACKReq:
		p := &SNACKReq{}
		p.UnmarshalBHS(bhs)
		return p, nil
	case OpNOPIn:
		p := &NOPIn{}
		p.UnmarshalBHS(bhs)
		return p, nil
	case OpSCSIResponse:
		p := &SCSIResponse{}
		p.UnmarshalBHS(bhs)
		return p, nil
	case OpTaskMgmtResp:
		p := &TaskMgmtResp{}
		p.UnmarshalBHS(bhs)
		return p, nil
	case OpLoginResp:
		p := &LoginResp{}
		p.UnmarshalBHS(bhs)
		return p, nil
	case OpTextResp:
		p := &TextResp{}
		p.UnmarshalBHS(bhs)
		return p, nil
	case OpDataIn:
		p := &DataIn{}
		p.UnmarshalBHS(bhs)
		return p, nil
	case OpLogoutResp:
		p := &LogoutResp{}
		p.UnmarshalBHS(bhs)
		return p, nil
	case OpR2T:
		p := &R2T{}
		p.UnmarshalBHS(bhs)
		return p, nil
	case OpAsyncMsg:
		p := &AsyncMsg{}
		p.UnmarshalBHS(bhs)
		return p, nil
	case OpReject:
		p := &Reject{}
		p.UnmarshalBHS(bhs)
		return p, nil
	default:
		return nil, &ProtocolError{
			Kind:   BadOpcode,
			Op:     "decode",
			Detail: fmt.Sprintf("unknown opcode 0x%02x", uint8(opcode)),
			Opcode: opcode,
			Got:    uint32(opcode),
		}
	}
}
