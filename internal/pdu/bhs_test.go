package pdu

import "testing"

func TestEncodeDecodeDataSegmentLength(t *testing.T) {
	tests := []struct {
		name string
		val  uint32
	}{
		{"zero", 0},
		{"one", 1},
		{"255", 255},
		{"256", 256},
		{"0x000100", 0x000100},
		{"0x123456", 0x123456},
		{"max 24-bit", 0xFFFFFF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bhs [BHSLength]byte
			encodeDataSegmentLength(bhs[:], tt.val)
			got := decodeDataSegmentLength(bhs[:])
			if got != tt.val {
				t.Errorf("round-trip failed: encoded %d, decoded %d", tt.val, got)
			}
		})
	}
}

func TestEncodeDataSegmentLengthPreservesTotalAHSLength(t *testing.T) {
	// Pitfall 2 regression: encoding DataSegmentLength (bytes 5-7) must NOT
	// corrupt TotalAHSLength (byte 4).
	var bhs [BHSLength]byte
	bhs[4] = 0xAA // TotalAHSLength
	encodeDataSegmentLength(bhs[:], 0x123456)
	if bhs[4] != 0xAA {
		t.Errorf("TotalAHSLength corrupted: got 0x%02x, want 0xAA", bhs[4])
	}
	if bhs[5] != 0x12 || bhs[6] != 0x34 || bhs[7] != 0x56 {
		t.Errorf("DataSegmentLength bytes wrong: got [0x%02x, 0x%02x, 0x%02x], want [0x12, 0x34, 0x56]",
			bhs[5], bhs[6], bhs[7])
	}
}

func TestEncodeDecodeOpcodeByte(t *testing.T) {
	allOps := []OpCode{
		OpNOPOut, OpSCSICommand, OpTaskMgmtReq, OpLoginReq,
		OpTextReq, OpDataOut, OpLogoutReq, OpSNACKReq,
		OpNOPIn, OpSCSIResponse, OpTaskMgmtResp, OpLoginResp,
		OpTextResp, OpDataIn, OpLogoutResp, OpR2T, OpAsyncMsg, OpReject,
	}
	for _, op := range allOps {
		for _, imm := range []bool{false, true} {
			b := encodeOpcodeByte(op, imm)
			gotOp, gotImm := decodeOpcodeByte(b)
			if gotOp != op {
				t.Errorf("opcode round-trip failed for 0x%02x imm=%v: got 0x%02x", uint8(op), imm, uint8(gotOp))
			}
			if gotImm != imm {
				t.Errorf("immediate round-trip failed for 0x%02x imm=%v: got %v", uint8(op), imm, gotImm)
			}
		}
	}
}

func TestEncodeOpcodeByteValues(t *testing.T) {
	// Specific value checks from the plan
	if got := encodeOpcodeByte(OpSCSICommand, false); got != 0x01 {
		t.Errorf("encodeOpcodeByte(OpSCSICommand, false) = 0x%02x, want 0x01", got)
	}
	if got := encodeOpcodeByte(OpSCSICommand, true); got != 0x41 {
		t.Errorf("encodeOpcodeByte(OpSCSICommand, true) = 0x%02x, want 0x41", got)
	}
}

func TestEncodeOpcodeByteMasks(t *testing.T) {
	// Verify upper 2 bits are used correctly: bit 7 reserved (0), bit 6 immediate
	b := encodeOpcodeByte(OpReject, false) // OpReject = 0x3f
	if b&0x80 != 0 {
		t.Error("bit 7 (reserved) should be 0")
	}
	if b&0x40 != 0 {
		t.Error("bit 6 (immediate) should be 0 when immediate=false")
	}
	if b&0x3f != byte(OpReject) {
		t.Errorf("opcode bits wrong: got 0x%02x, want 0x%02x", b&0x3f, byte(OpReject))
	}
}
