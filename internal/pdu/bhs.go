package pdu

// BHSLength is the fixed size of an iSCSI Basic Header Segment in bytes.
// All iSCSI PDUs start with exactly 48 bytes of BHS (RFC 7143 Section 11.2.1).
const BHSLength = 48

// encodeDataSegmentLength writes a 24-bit DataSegmentLength value into BHS
// bytes 5-7 in big-endian order. It does NOT touch byte 4 (TotalAHSLength).
//
// CRITICAL (Pitfall 2): Do not use binary.BigEndian.PutUint32 on bytes 4-7,
// as that would overwrite the TotalAHSLength field in byte 4.
func encodeDataSegmentLength(bhs []byte, dsLen uint32) {
	bhs[5] = byte(dsLen >> 16)
	bhs[6] = byte(dsLen >> 8)
	bhs[7] = byte(dsLen)
}

// decodeDataSegmentLength reads a 24-bit DataSegmentLength from BHS bytes 5-7.
func decodeDataSegmentLength(bhs []byte) uint32 {
	return uint32(bhs[5])<<16 | uint32(bhs[6])<<8 | uint32(bhs[7])
}

// encodeOpcodeByte encodes the opcode and immediate flag into BHS byte 0.
// Bit 7 is reserved (always 0), bit 6 is the Immediate delivery marker,
// and bits 5-0 contain the opcode value.
func encodeOpcodeByte(opcode OpCode, immediate bool) byte {
	b := byte(opcode) & 0x3f
	if immediate {
		b |= 0x40
	}
	return b
}

// decodeOpcodeByte extracts the opcode and immediate flag from BHS byte 0.
func decodeOpcodeByte(b byte) (OpCode, bool) {
	return OpCode(b & 0x3f), b&0x40 != 0
}
