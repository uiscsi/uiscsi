package pdu

import (
	"strings"
	"testing"
)

func TestStringNOPOut(t *testing.T) {
	p := &NOPOut{}
	p.InitiatorTaskTag = 0x00000001
	p.TargetTransferTag = 0xFFFFFFFF
	p.CmdSN = 42
	s := p.String()
	assertContains(t, s, "NOPOut{")
	assertContains(t, s, "ITT:0x00000001")
	assertContains(t, s, "TTT:0xffffffff")
	assertContains(t, s, "CmdSN:42")
}

func TestStringSCSICommand(t *testing.T) {
	p := &SCSICommand{}
	p.InitiatorTaskTag = 0x0000000A
	p.CmdSN = 7
	p.Read = true
	p.Write = false
	p.CDB = [16]byte{0x28, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	s := p.String()
	assertContains(t, s, "SCSICommand{")
	assertContains(t, s, "ITT:0x0000000a")
	assertContains(t, s, "CmdSN:7")
	assertContains(t, s, "R:true")
	assertContains(t, s, "W:false")
	assertContains(t, s, "CDB:28:00:00:00:00:01:00:00")
	// CDB should be truncated to 8 bytes
	if strings.Count(s, ":") > 10 { // Allow some colons from other fields
		// Verify CDB doesn't show all 16 bytes
	}
}

func TestStringTaskMgmtReq(t *testing.T) {
	p := &TaskMgmtReq{}
	p.InitiatorTaskTag = 0x00000005
	p.Function = 1
	p.ReferencedTaskTag = 0x00000003
	p.CmdSN = 10
	s := p.String()
	assertContains(t, s, "TaskMgmtReq{")
	assertContains(t, s, "Function:1")
	assertContains(t, s, "RefTaskTag:0x00000003")
}

func TestStringLoginReq(t *testing.T) {
	p := &LoginReq{}
	p.InitiatorTaskTag = 0x00000001
	p.CSG = 1
	p.NSG = 3
	p.Transit = true
	p.CmdSN = 0
	s := p.String()
	assertContains(t, s, "LoginReq{")
	assertContains(t, s, "CSG:1")
	assertContains(t, s, "NSG:3")
	assertContains(t, s, "T:true")
}

func TestStringTextReq(t *testing.T) {
	p := &TextReq{}
	p.InitiatorTaskTag = 0x00000002
	p.Continue = true
	p.CmdSN = 5
	s := p.String()
	assertContains(t, s, "TextReq{")
	assertContains(t, s, "C:true")
	assertContains(t, s, "CmdSN:5")
}

func TestStringDataOut(t *testing.T) {
	p := &DataOut{}
	p.InitiatorTaskTag = 0x00000001
	p.TargetTransferTag = 0x00000002
	p.DataSN = 3
	p.BufferOffset = 512
	p.Data = make([]byte, 1024)
	s := p.String()
	assertContains(t, s, "DataOut{")
	assertContains(t, s, "DataSN:3")
	assertContains(t, s, "Offset:512")
	assertContains(t, s, "Len:1024")
}

func TestStringLogoutReq(t *testing.T) {
	p := &LogoutReq{}
	p.InitiatorTaskTag = 0x00000001
	p.ReasonCode = 0
	p.CmdSN = 15
	s := p.String()
	assertContains(t, s, "LogoutReq{")
	assertContains(t, s, "Reason:0")
	assertContains(t, s, "CmdSN:15")
}

func TestStringSNACKReq(t *testing.T) {
	p := &SNACKReq{}
	p.InitiatorTaskTag = 0x00000001
	p.Type = 2
	p.BegRun = 100
	p.RunLength = 50
	s := p.String()
	assertContains(t, s, "SNACKReq{")
	assertContains(t, s, "Type:2")
	assertContains(t, s, "BegRun:100")
	assertContains(t, s, "RunLength:50")
}

func TestStringNOPIn(t *testing.T) {
	p := &NOPIn{}
	p.InitiatorTaskTag = 0xFFFFFFFF
	p.TargetTransferTag = 0x00000001
	p.StatSN = 99
	s := p.String()
	assertContains(t, s, "NOPIn{")
	assertContains(t, s, "TTT:0x00000001")
	assertContains(t, s, "StatSN:99")
}

func TestStringSCSIResponse(t *testing.T) {
	p := &SCSIResponse{}
	p.InitiatorTaskTag = 0x0000000A
	p.StatSN = 5
	p.Status = 0x02 // CHECK CONDITION
	p.Response = 0x00
	s := p.String()
	assertContains(t, s, "SCSIResponse{")
	assertContains(t, s, "Status:0x02")
	assertContains(t, s, "Response:0x00")
	assertContains(t, s, "StatSN:5")
}

func TestStringTaskMgmtResp(t *testing.T) {
	p := &TaskMgmtResp{}
	p.InitiatorTaskTag = 0x00000005
	p.Response = 0
	p.StatSN = 20
	s := p.String()
	assertContains(t, s, "TaskMgmtResp{")
	assertContains(t, s, "Response:0x00")
	assertContains(t, s, "StatSN:20")
}

func TestStringLoginResp(t *testing.T) {
	p := &LoginResp{}
	p.InitiatorTaskTag = 0x00000001
	p.CSG = 1
	p.NSG = 3
	p.Transit = true
	p.StatSN = 0
	p.StatusClass = 0
	p.StatusDetail = 0
	s := p.String()
	assertContains(t, s, "LoginResp{")
	assertContains(t, s, "StatusClass:0x00")
	assertContains(t, s, "StatusDetail:0x00")
}

func TestStringTextResp(t *testing.T) {
	p := &TextResp{}
	p.InitiatorTaskTag = 0x00000001
	p.Continue = false
	p.StatSN = 10
	s := p.String()
	assertContains(t, s, "TextResp{")
	assertContains(t, s, "C:false")
	assertContains(t, s, "StatSN:10")
}

func TestStringDataIn(t *testing.T) {
	p := &DataIn{}
	p.InitiatorTaskTag = 0x0000000A
	p.StatSN = 1
	p.DataSN = 0
	p.BufferOffset = 0
	p.HasStatus = true
	p.Data = make([]byte, 512)
	s := p.String()
	assertContains(t, s, "DataIn{")
	assertContains(t, s, "DataSN:0")
	assertContains(t, s, "Offset:0")
	assertContains(t, s, "Len:512")
	assertContains(t, s, "S:true")
}

func TestStringLogoutResp(t *testing.T) {
	p := &LogoutResp{}
	p.InitiatorTaskTag = 0x00000001
	p.Response = 0
	p.StatSN = 5
	p.Time2Wait = 2
	p.Time2Retain = 20
	s := p.String()
	assertContains(t, s, "LogoutResp{")
	assertContains(t, s, "Response:0x00")
	assertContains(t, s, "T2W:2")
	assertContains(t, s, "T2R:20")
}

func TestStringR2T(t *testing.T) {
	p := &R2T{}
	p.InitiatorTaskTag = 0x0000000A
	p.TargetTransferTag = 0x00000001
	p.R2TSN = 0
	p.BufferOffset = 1024
	p.DesiredDataTransferLength = 4096
	s := p.String()
	assertContains(t, s, "R2T{")
	assertContains(t, s, "R2TSN:0")
	assertContains(t, s, "Offset:1024")
	assertContains(t, s, "DDTL:4096")
}

func TestStringAsyncMsg(t *testing.T) {
	p := &AsyncMsg{}
	p.InitiatorTaskTag = 0xFFFFFFFF
	p.AsyncEvent = 1
	p.StatSN = 50
	s := p.String()
	assertContains(t, s, "AsyncMsg{")
	assertContains(t, s, "Event:1")
	assertContains(t, s, "StatSN:50")
}

func TestStringReject(t *testing.T) {
	p := &Reject{}
	p.InitiatorTaskTag = 0xFFFFFFFF
	p.Reason = 0x09
	p.StatSN = 30
	s := p.String()
	assertContains(t, s, "Reject{")
	assertContains(t, s, "Reason:0x09")
	assertContains(t, s, "StatSN:30")
}

func TestStringCDBTruncation(t *testing.T) {
	p := &SCSICommand{}
	p.InitiatorTaskTag = 0x00000001
	p.CmdSN = 1
	p.CDB = [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10}
	s := p.String()
	// Only first 8 bytes should appear
	assertContains(t, s, "CDB:01:02:03:04:05:06:07:08")
	// Byte 9 (0x09) should NOT appear in CDB field
	if strings.Contains(s, "CDB:01:02:03:04:05:06:07:08:09") {
		t.Errorf("CDB should be truncated to 8 bytes, got: %s", s)
	}
}

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("String() = %q, want substring %q", s, substr)
	}
}
