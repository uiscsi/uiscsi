package pdu

import (
	"bytes"
	"testing"
)

// TestPDURoundTrip tests MarshalBHS -> DecodeBHS round-trip for all 18 opcode types.
func TestPDURoundTrip(t *testing.T) {
	tests := []struct {
		name string
		pdu  PDU
		// check is a function that verifies the decoded PDU matches the original
		check func(t *testing.T, decoded PDU)
	}{
		{
			name: "NOPOut",
			pdu: &NOPOut{
				Header:            Header{Immediate: true, Final: true, InitiatorTaskTag: 0xDEADBEEF},
				TargetTransferTag: 0xCAFEBABE,
				CmdSN:             42,
				ExpStatSN:         43,
			},
			check: func(t *testing.T, decoded PDU) {
				p := decoded.(*NOPOut)
				if p.InitiatorTaskTag != 0xDEADBEEF {
					t.Errorf("ITT = 0x%08x, want 0xDEADBEEF", p.InitiatorTaskTag)
				}
				if p.TargetTransferTag != 0xCAFEBABE {
					t.Errorf("TTT = 0x%08x, want 0xCAFEBABE", p.TargetTransferTag)
				}
				if p.CmdSN != 42 {
					t.Errorf("CmdSN = %d, want 42", p.CmdSN)
				}
				if p.ExpStatSN != 43 {
					t.Errorf("ExpStatSN = %d, want 43", p.ExpStatSN)
				}
				if !p.Immediate {
					t.Error("Immediate should be true")
				}
				if !p.Final {
					t.Error("Final should be true")
				}
			},
		},
		{
			name: "SCSICommand",
			pdu: &SCSICommand{
				Header:                     Header{Immediate: true, Final: true, LUN: [8]byte{0, 1}, InitiatorTaskTag: 0x12345678},
				Read:                       true,
				ExpectedDataTransferLength: 65536,
				CmdSN:                      100,
				ExpStatSN:                  101,
				CDB:                        [16]byte{0x28, 0, 0, 0, 0, 0, 0, 0, 0, 8}, // READ(10)
			},
			check: func(t *testing.T, decoded PDU) {
				p := decoded.(*SCSICommand)
				if !p.Read {
					t.Error("Read bit should be set")
				}
				if p.Write {
					t.Error("Write bit should not be set")
				}
				if p.ExpectedDataTransferLength != 65536 {
					t.Errorf("ExpectedDataTransferLength = %d, want 65536", p.ExpectedDataTransferLength)
				}
				if p.CDB[0] != 0x28 {
					t.Errorf("CDB[0] = 0x%02x, want 0x28", p.CDB[0])
				}
				if p.LUN != [8]byte{0, 1} {
					t.Errorf("LUN mismatch")
				}
			},
		},
		{
			name: "TaskMgmtReq",
			pdu: &TaskMgmtReq{
				Header:            Header{Immediate: true, Final: true, LUN: [8]byte{0, 0, 0, 0, 0, 0, 0, 1}, InitiatorTaskTag: 0xABCD},
				Function:          0x01, // ABORT TASK
				ReferencedTaskTag: 0x1234,
				CmdSN:             200,
				ExpStatSN:         201,
				RefCmdSN:          199,
			},
			check: func(t *testing.T, decoded PDU) {
				p := decoded.(*TaskMgmtReq)
				if p.Function != 0x01 {
					t.Errorf("Function = 0x%02x, want 0x01", p.Function)
				}
				if p.ReferencedTaskTag != 0x1234 {
					t.Errorf("ReferencedTaskTag = 0x%x, want 0x1234", p.ReferencedTaskTag)
				}
				if p.RefCmdSN != 199 {
					t.Errorf("RefCmdSN = %d, want 199", p.RefCmdSN)
				}
			},
		},
		{
			name: "LoginReq",
			pdu: &LoginReq{
				Header:     Header{InitiatorTaskTag: 0x1},
				Transit:    true,
				Continue:   false,
				CSG:        1,
				NSG:        3,
				VersionMax: 0,
				VersionMin: 0,
				ISID:       [6]byte{0x00, 0x02, 0x3D, 0x00, 0x00, 0x01},
				TSIH:       0,
				CID:        1,
				CmdSN:      1,
				ExpStatSN:  0,
			},
			check: func(t *testing.T, decoded PDU) {
				p := decoded.(*LoginReq)
				if !p.Transit {
					t.Error("Transit should be true")
				}
				if p.Continue {
					t.Error("Continue should be false")
				}
				if p.CSG != 1 {
					t.Errorf("CSG = %d, want 1", p.CSG)
				}
				if p.NSG != 3 {
					t.Errorf("NSG = %d, want 3", p.NSG)
				}
				if p.ISID != [6]byte{0x00, 0x02, 0x3D, 0x00, 0x00, 0x01} {
					t.Errorf("ISID mismatch: %v", p.ISID)
				}
				if p.CID != 1 {
					t.Errorf("CID = %d, want 1", p.CID)
				}
			},
		},
		{
			name: "TextReq",
			pdu: &TextReq{
				Header:            Header{Final: true, InitiatorTaskTag: 0x55},
				Continue:          false,
				TargetTransferTag: 0xFFFFFFFF,
				CmdSN:             10,
				ExpStatSN:         11,
			},
			check: func(t *testing.T, decoded PDU) {
				p := decoded.(*TextReq)
				if p.TargetTransferTag != 0xFFFFFFFF {
					t.Errorf("TTT = 0x%08x, want 0xFFFFFFFF", p.TargetTransferTag)
				}
				if !p.Final {
					t.Error("Final should be true")
				}
			},
		},
		{
			name: "DataOut",
			pdu: &DataOut{
				Header:            Header{Final: true, InitiatorTaskTag: 0x99},
				TargetTransferTag: 0xBBBB,
				ExpStatSN:         50,
				DataSN:            3,
				BufferOffset:      8192,
			},
			check: func(t *testing.T, decoded PDU) {
				p := decoded.(*DataOut)
				if p.DataSN != 3 {
					t.Errorf("DataSN = %d, want 3", p.DataSN)
				}
				if p.BufferOffset != 8192 {
					t.Errorf("BufferOffset = %d, want 8192", p.BufferOffset)
				}
			},
		},
		{
			name: "LogoutReq",
			pdu: &LogoutReq{
				Header:     Header{Immediate: true, Final: true, InitiatorTaskTag: 0xAA},
				ReasonCode: 0x00, // close session
				CID:        1,
				CmdSN:      77,
				ExpStatSN:  78,
			},
			check: func(t *testing.T, decoded PDU) {
				p := decoded.(*LogoutReq)
				if p.ReasonCode != 0x00 {
					t.Errorf("ReasonCode = %d, want 0", p.ReasonCode)
				}
				if p.CID != 1 {
					t.Errorf("CID = %d, want 1", p.CID)
				}
			},
		},
		{
			name: "SNACKReq",
			pdu: &SNACKReq{
				Header:            Header{InitiatorTaskTag: 0xCC},
				Type:              0x01, // Data/R2T SNACK
				TargetTransferTag: 0xDDDD,
				ExpStatSN:         60,
				BegRun:            100,
				RunLength:         5,
			},
			check: func(t *testing.T, decoded PDU) {
				p := decoded.(*SNACKReq)
				if p.Type != 0x01 {
					t.Errorf("Type = %d, want 1", p.Type)
				}
				if p.BegRun != 100 {
					t.Errorf("BegRun = %d, want 100", p.BegRun)
				}
				if p.RunLength != 5 {
					t.Errorf("RunLength = %d, want 5", p.RunLength)
				}
			},
		},
		// Target opcodes
		{
			name: "NOPIn",
			pdu: &NOPIn{
				Header:            Header{Final: true, InitiatorTaskTag: 0xFFFFFFFF},
				TargetTransferTag: 0x11111111,
				StatSN:            500,
				ExpCmdSN:          501,
				MaxCmdSN:          510,
			},
			check: func(t *testing.T, decoded PDU) {
				p := decoded.(*NOPIn)
				if p.InitiatorTaskTag != 0xFFFFFFFF {
					t.Errorf("ITT = 0x%08x, want 0xFFFFFFFF", p.InitiatorTaskTag)
				}
				if p.TargetTransferTag != 0x11111111 {
					t.Errorf("TTT = 0x%08x, want 0x11111111", p.TargetTransferTag)
				}
				if p.StatSN != 500 {
					t.Errorf("StatSN = %d, want 500", p.StatSN)
				}
			},
		},
		{
			name: "SCSIResponse",
			pdu: &SCSIResponse{
				Header:            Header{Final: true, InitiatorTaskTag: 0x1234},
				Underflow:         true,
				Response:          0x00, // completed at target
				Status:            0x02, // CHECK CONDITION
				StatSN:            600,
				ExpCmdSN:          601,
				MaxCmdSN:          610,
				ResidualCount:     256,
			},
			check: func(t *testing.T, decoded PDU) {
				p := decoded.(*SCSIResponse)
				if !p.Underflow {
					t.Error("Underflow should be true")
				}
				if p.Overflow {
					t.Error("Overflow should be false")
				}
				if p.Status != 0x02 {
					t.Errorf("Status = 0x%02x, want 0x02", p.Status)
				}
				if p.ResidualCount != 256 {
					t.Errorf("ResidualCount = %d, want 256", p.ResidualCount)
				}
			},
		},
		{
			name: "SCSIResponse_AllResidualFlags",
			pdu: &SCSIResponse{
				Header:            Header{Final: true, InitiatorTaskTag: 0x5678},
				BidiOverflow:      true,
				BidiUnderflow:     true,
				Overflow:          true,
				Underflow:         true,
				Response:          0x01,
				Status:            0x08,
				ExpDataSN:         5,
				BidiResidualCount: 1024,
				ResidualCount:     512,
			},
			check: func(t *testing.T, decoded PDU) {
				p := decoded.(*SCSIResponse)
				if !p.BidiOverflow {
					t.Error("BidiOverflow")
				}
				if !p.BidiUnderflow {
					t.Error("BidiUnderflow")
				}
				if !p.Overflow {
					t.Error("Overflow")
				}
				if !p.Underflow {
					t.Error("Underflow")
				}
				if p.BidiResidualCount != 1024 {
					t.Errorf("BidiResidualCount = %d, want 1024", p.BidiResidualCount)
				}
			},
		},
		{
			name: "TaskMgmtResp",
			pdu: &TaskMgmtResp{
				Header:   Header{Final: true, InitiatorTaskTag: 0xABCD},
				Response: 0x00, // function complete
				StatSN:   700,
				ExpCmdSN: 701,
				MaxCmdSN: 710,
			},
			check: func(t *testing.T, decoded PDU) {
				p := decoded.(*TaskMgmtResp)
				if p.Response != 0x00 {
					t.Errorf("Response = %d, want 0", p.Response)
				}
			},
		},
		{
			name: "LoginResp",
			pdu: &LoginResp{
				Header:        Header{InitiatorTaskTag: 0x1},
				Transit:       true,
				CSG:           1,
				NSG:           3,
				VersionMax:    0,
				VersionActive: 0,
				ISID:          [6]byte{0x00, 0x02, 0x3D, 0x00, 0x00, 0x01},
				TSIH:          0x1234,
				StatSN:        1,
				ExpCmdSN:      2,
				MaxCmdSN:      10,
				StatusClass:   0,
				StatusDetail:  0,
			},
			check: func(t *testing.T, decoded PDU) {
				p := decoded.(*LoginResp)
				if !p.Transit {
					t.Error("Transit")
				}
				if p.CSG != 1 {
					t.Errorf("CSG = %d, want 1", p.CSG)
				}
				if p.NSG != 3 {
					t.Errorf("NSG = %d, want 3", p.NSG)
				}
				if p.TSIH != 0x1234 {
					t.Errorf("TSIH = 0x%04x, want 0x1234", p.TSIH)
				}
			},
		},
		{
			name: "TextResp",
			pdu: &TextResp{
				Header:            Header{Final: true, InitiatorTaskTag: 0x55},
				Continue:          true,
				TargetTransferTag: 0xEEEE,
				StatSN:            800,
				ExpCmdSN:          801,
				MaxCmdSN:          810,
			},
			check: func(t *testing.T, decoded PDU) {
				p := decoded.(*TextResp)
				if !p.Continue {
					t.Error("Continue should be true")
				}
				if p.TargetTransferTag != 0xEEEE {
					t.Errorf("TTT = 0x%x, want 0xEEEE", p.TargetTransferTag)
				}
			},
		},
		{
			name: "DataIn",
			pdu: &DataIn{
				Header:       Header{Final: true, InitiatorTaskTag: 0x99},
				HasStatus:    true,
				Status:       0x00, // GOOD
				DataSN:       7,
				BufferOffset: 16384,
				StatSN:       900,
				ExpCmdSN:     901,
				MaxCmdSN:     910,
			},
			check: func(t *testing.T, decoded PDU) {
				p := decoded.(*DataIn)
				if !p.HasStatus {
					t.Error("S-bit should be set")
				}
				if p.Status != 0x00 {
					t.Errorf("Status = 0x%02x, want 0x00", p.Status)
				}
				if p.DataSN != 7 {
					t.Errorf("DataSN = %d, want 7", p.DataSN)
				}
				if p.BufferOffset != 16384 {
					t.Errorf("BufferOffset = %d, want 16384", p.BufferOffset)
				}
				if !p.Final {
					t.Error("Final should be true")
				}
			},
		},
		{
			name: "LogoutResp",
			pdu: &LogoutResp{
				Header:      Header{Final: true, InitiatorTaskTag: 0xBB},
				Response:    0x00,
				StatSN:      1000,
				ExpCmdSN:    1001,
				MaxCmdSN:    1010,
				Time2Wait:   2,
				Time2Retain: 20,
			},
			check: func(t *testing.T, decoded PDU) {
				p := decoded.(*LogoutResp)
				if p.Time2Wait != 2 {
					t.Errorf("Time2Wait = %d, want 2", p.Time2Wait)
				}
				if p.Time2Retain != 20 {
					t.Errorf("Time2Retain = %d, want 20", p.Time2Retain)
				}
			},
		},
		{
			name: "R2T",
			pdu: &R2T{
				Header:                    Header{Final: true, InitiatorTaskTag: 0xDD},
				TargetTransferTag:         0xAAAA,
				StatSN:                    1100,
				ExpCmdSN:                  1101,
				MaxCmdSN:                  1110,
				R2TSN:                     0,
				BufferOffset:              0,
				DesiredDataTransferLength: 65536,
			},
			check: func(t *testing.T, decoded PDU) {
				p := decoded.(*R2T)
				if p.DesiredDataTransferLength != 65536 {
					t.Errorf("DesiredDataTransferLength = %d, want 65536", p.DesiredDataTransferLength)
				}
				if p.TargetTransferTag != 0xAAAA {
					t.Errorf("TTT = 0x%x, want 0xAAAA", p.TargetTransferTag)
				}
			},
		},
		{
			name: "AsyncMsg",
			pdu: &AsyncMsg{
				Header:     Header{Final: true, InitiatorTaskTag: 0xFFFFFFFF},
				StatSN:     1200,
				ExpCmdSN:   1201,
				MaxCmdSN:   1210,
				AsyncEvent: 0x01,
				AsyncVCode: 0x00,
				Parameter1: 15,
				Parameter2: 1,
				Parameter3: 0,
			},
			check: func(t *testing.T, decoded PDU) {
				p := decoded.(*AsyncMsg)
				if p.AsyncEvent != 0x01 {
					t.Errorf("AsyncEvent = %d, want 1", p.AsyncEvent)
				}
				if p.Parameter1 != 15 {
					t.Errorf("Parameter1 = %d, want 15", p.Parameter1)
				}
			},
		},
		{
			name: "Reject",
			pdu: &Reject{
				Header:   Header{Final: true, InitiatorTaskTag: 0xFFFFFFFF},
				Reason:   0x09, // Invalid PDU field
				StatSN:   1300,
				ExpCmdSN: 1301,
				MaxCmdSN: 1310,
				DataSN:   5,
			},
			check: func(t *testing.T, decoded PDU) {
				p := decoded.(*Reject)
				if p.Reason != 0x09 {
					t.Errorf("Reason = 0x%02x, want 0x09", p.Reason)
				}
				if p.DataSN != 5 {
					t.Errorf("DataSN = %d, want 5", p.DataSN)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bhs, err := tt.pdu.MarshalBHS()
			if err != nil {
				t.Fatalf("MarshalBHS error: %v", err)
			}

			decoded, err := DecodeBHS(bhs)
			if err != nil {
				t.Fatalf("DecodeBHS error: %v", err)
			}

			if decoded.Opcode() != tt.pdu.Opcode() {
				t.Errorf("opcode = 0x%02x, want 0x%02x", uint8(decoded.Opcode()), uint8(tt.pdu.Opcode()))
			}

			tt.check(t, decoded)
		})
	}
}

// TestPDURoundTripReservedITT verifies ITT=0xFFFFFFFF (reserved value) round-trips.
func TestPDURoundTripReservedITT(t *testing.T) {
	p := &NOPIn{
		Header:            Header{Final: true, InitiatorTaskTag: 0xFFFFFFFF},
		TargetTransferTag: 0xFFFFFFFF,
		StatSN:            1,
	}
	bhs, err := p.MarshalBHS()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeBHS(bhs)
	if err != nil {
		t.Fatal(err)
	}
	n := decoded.(*NOPIn)
	if n.InitiatorTaskTag != 0xFFFFFFFF {
		t.Errorf("ITT = 0x%08x, want 0xFFFFFFFF", n.InitiatorTaskTag)
	}
}

// TestDataSegmentLengthDoesNotCorruptTotalAHSLength verifies Pitfall 2
// in the context of full PDU encoding.
func TestDataSegmentLengthDoesNotCorruptTotalAHSLength(t *testing.T) {
	p := &NOPOut{
		Header: Header{
			Final:          true,
			TotalAHSLength: 5, // arbitrary non-zero value
			DataSegmentLen: 0x123456,
		},
	}
	bhs, err := p.MarshalBHS()
	if err != nil {
		t.Fatal(err)
	}
	if bhs[4] != 5 {
		t.Errorf("TotalAHSLength corrupted: got %d, want 5", bhs[4])
	}
	decoded, err := DecodeBHS(bhs)
	if err != nil {
		t.Fatal(err)
	}
	n := decoded.(*NOPOut)
	if n.TotalAHSLength != 5 {
		t.Errorf("decoded TotalAHSLength = %d, want 5", n.TotalAHSLength)
	}
	if n.DataSegmentLen != 0x123456 {
		t.Errorf("decoded DataSegmentLen = 0x%06x, want 0x123456", n.DataSegmentLen)
	}
}

// TestEncodePDUPadding verifies data segment padding.
func TestEncodePDUPadding(t *testing.T) {
	tests := []struct {
		name      string
		dataLen   int
		wantTotal int // BHS + data + padding
	}{
		{"no data", 0, BHSLength},
		{"1 byte data", 1, BHSLength + 4},     // 1 + 3 pad
		{"2 byte data", 2, BHSLength + 4},     // 2 + 2 pad
		{"3 byte data", 3, BHSLength + 4},     // 3 + 1 pad
		{"4 byte data", 4, BHSLength + 4},     // 4 + 0 pad
		{"5 byte data", 5, BHSLength + 8},     // 5 + 3 pad
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]byte, tt.dataLen)
			for i := range data {
				data[i] = 0xFF // non-zero to detect padding
			}
			p := &NOPOut{
				Header: Header{
					Final:          true,
					DataSegmentLen: uint32(tt.dataLen),
				},
				Data: data,
			}
			encoded, err := EncodePDU(p)
			if err != nil {
				t.Fatal(err)
			}
			if len(encoded) != tt.wantTotal {
				t.Errorf("encoded length = %d, want %d", len(encoded), tt.wantTotal)
			}
			// Verify padding bytes are zero
			for i := BHSLength + tt.dataLen; i < len(encoded); i++ {
				if encoded[i] != 0 {
					t.Errorf("padding byte at offset %d = 0x%02x, want 0x00", i, encoded[i])
				}
			}
		})
	}
}

// TestEncodePDUImmediateBit verifies the immediate bit in byte 0.
func TestEncodePDUImmediateBit(t *testing.T) {
	p := &SCSICommand{
		Header: Header{Immediate: true, Final: true},
		CmdSN:  1,
	}
	encoded, err := EncodePDU(p)
	if err != nil {
		t.Fatal(err)
	}
	if encoded[0]&0x40 == 0 {
		t.Error("immediate bit not set in byte 0")
	}
	if encoded[0]&0x3f != byte(OpSCSICommand) {
		t.Errorf("opcode = 0x%02x, want 0x%02x", encoded[0]&0x3f, byte(OpSCSICommand))
	}
}

// TestEncodePDUFinalBit verifies the Final bit in byte 1.
func TestEncodePDUFinalBit(t *testing.T) {
	p := &TextReq{
		Header: Header{Final: true},
	}
	encoded, err := EncodePDU(p)
	if err != nil {
		t.Fatal(err)
	}
	if encoded[1]&0x80 == 0 {
		t.Error("Final bit not set in byte 1")
	}
}

// TestDecodeBHSUnknownOpcode verifies error on unknown opcode.
func TestDecodeBHSUnknownOpcode(t *testing.T) {
	var bhs [BHSLength]byte
	bhs[0] = 0x0F // undefined opcode
	_, err := DecodeBHS(bhs)
	if err == nil {
		t.Error("expected error for unknown opcode 0x0F, got nil")
	}
}

// TestLoginReqCSGNSGBitPacking verifies CSG/NSG bit packing in Login Request byte 1.
func TestLoginReqCSGNSGBitPacking(t *testing.T) {
	tests := []struct {
		csg, nsg uint8
	}{
		{0, 0},
		{0, 1},
		{1, 0},
		{1, 3},
		{2, 3},
		{3, 3},
	}
	for _, tt := range tests {
		p := &LoginReq{
			Header:  Header{InitiatorTaskTag: 1},
			Transit: true,
			CSG:     tt.csg,
			NSG:     tt.nsg,
		}
		bhs, err := p.MarshalBHS()
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := DecodeBHS(bhs)
		if err != nil {
			t.Fatal(err)
		}
		lr := decoded.(*LoginReq)
		if lr.CSG != tt.csg {
			t.Errorf("CSG=%d,NSG=%d: decoded CSG=%d", tt.csg, tt.nsg, lr.CSG)
		}
		if lr.NSG != tt.nsg {
			t.Errorf("CSG=%d,NSG=%d: decoded NSG=%d", tt.csg, tt.nsg, lr.NSG)
		}
	}
}

// TestRejectWithDataSegment verifies Reject PDU with a 48-byte rejected BHS
// in the data segment.
func TestRejectWithDataSegment(t *testing.T) {
	rejectedBHS := make([]byte, BHSLength)
	rejectedBHS[0] = byte(OpSCSICommand) // fill with a fake rejected BHS
	rejectedBHS[1] = 0xFF

	p := &Reject{
		Header: Header{
			Final:          true,
			DataSegmentLen: uint32(len(rejectedBHS)),
		},
		Reason: 0x09,
		StatSN: 100,
		Data:   rejectedBHS,
	}
	encoded, err := EncodePDU(p)
	if err != nil {
		t.Fatal(err)
	}

	// BHS (48) + data (48) + no padding (48 is 4-byte aligned)
	if len(encoded) != BHSLength+BHSLength {
		t.Errorf("encoded length = %d, want %d", len(encoded), BHSLength+BHSLength)
	}

	// Verify data segment
	if !bytes.Equal(encoded[BHSLength:], rejectedBHS) {
		t.Error("data segment mismatch")
	}
}

// TestSCSICommandWriteFlag verifies Write flag round-trip.
func TestSCSICommandWriteFlag(t *testing.T) {
	p := &SCSICommand{
		Header: Header{Final: true, InitiatorTaskTag: 0x42},
		Write:  true,
		Attr:   0x02, // ordered
		CmdSN:  1,
	}
	bhs, err := p.MarshalBHS()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeBHS(bhs)
	if err != nil {
		t.Fatal(err)
	}
	sc := decoded.(*SCSICommand)
	if !sc.Write {
		t.Error("Write should be true")
	}
	if sc.Read {
		t.Error("Read should be false")
	}
	if sc.Attr != 0x02 {
		t.Errorf("Attr = %d, want 2", sc.Attr)
	}
}

// TestDataInSBitWithStatus verifies DataIn S-bit and Status round-trip.
func TestDataInSBitWithStatus(t *testing.T) {
	p := &DataIn{
		Header:    Header{Final: true, InitiatorTaskTag: 0x77},
		HasStatus: true,
		Status:    0x02, // CHECK CONDITION
		DataSN:    10,
		StatSN:    999,
	}
	bhs, err := p.MarshalBHS()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeBHS(bhs)
	if err != nil {
		t.Fatal(err)
	}
	di := decoded.(*DataIn)
	if !di.HasStatus {
		t.Error("S-bit should be set")
	}
	if di.Status != 0x02 {
		t.Errorf("Status = 0x%02x, want 0x02", di.Status)
	}
	if di.StatSN != 999 {
		t.Errorf("StatSN = %d, want 999", di.StatSN)
	}
}
