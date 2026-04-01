package pdu

import (
	"fmt"
	"strings"
)

// truncHex formats the first maxBytes of b as colon-separated hex.
// Returns "" if b is empty.
func truncHex(b []byte, maxBytes int) string {
	if len(b) > maxBytes {
		b = b[:maxBytes]
	}
	if len(b) == 0 {
		return ""
	}
	var buf strings.Builder
	for i, v := range b {
		if i > 0 {
			buf.WriteByte(':')
		}
		fmt.Fprintf(&buf, "%02x", v)
	}
	return buf.String()
}

// --- Initiator PDU String() methods ---

func (p *NOPOut) String() string {
	return fmt.Sprintf("NOPOut{ITT:0x%08x TTT:0x%08x CmdSN:%d}",
		p.InitiatorTaskTag, p.TargetTransferTag, p.CmdSN)
}

func (p *SCSICommand) String() string {
	return fmt.Sprintf("SCSICommand{ITT:0x%08x CmdSN:%d R:%t W:%t CDB:%s}",
		p.InitiatorTaskTag, p.CmdSN, p.Read, p.Write, truncHex(p.CDB[:], 8))
}

func (p *TaskMgmtReq) String() string {
	return fmt.Sprintf("TaskMgmtReq{ITT:0x%08x Function:%d RefTaskTag:0x%08x CmdSN:%d}",
		p.InitiatorTaskTag, p.Function, p.ReferencedTaskTag, p.CmdSN)
}

func (p *LoginReq) String() string {
	return fmt.Sprintf("LoginReq{ITT:0x%08x CSG:%d NSG:%d T:%t CmdSN:%d}",
		p.InitiatorTaskTag, p.CSG, p.NSG, p.Transit, p.CmdSN)
}

func (p *TextReq) String() string {
	return fmt.Sprintf("TextReq{ITT:0x%08x C:%t CmdSN:%d}",
		p.InitiatorTaskTag, p.Continue, p.CmdSN)
}

func (p *DataOut) String() string {
	return fmt.Sprintf("DataOut{ITT:0x%08x TTT:0x%08x DataSN:%d Offset:%d Len:%d}",
		p.InitiatorTaskTag, p.TargetTransferTag, p.DataSN, p.BufferOffset, len(p.Data))
}

func (p *LogoutReq) String() string {
	return fmt.Sprintf("LogoutReq{ITT:0x%08x Reason:%d CmdSN:%d}",
		p.InitiatorTaskTag, p.ReasonCode, p.CmdSN)
}

func (p *SNACKReq) String() string {
	return fmt.Sprintf("SNACKReq{ITT:0x%08x Type:%d BegRun:%d RunLength:%d}",
		p.InitiatorTaskTag, p.Type, p.BegRun, p.RunLength)
}

// --- Target PDU String() methods ---

func (p *NOPIn) String() string {
	return fmt.Sprintf("NOPIn{ITT:0x%08x TTT:0x%08x StatSN:%d}",
		p.InitiatorTaskTag, p.TargetTransferTag, p.StatSN)
}

func (p *SCSIResponse) String() string {
	return fmt.Sprintf("SCSIResponse{ITT:0x%08x StatSN:%d Status:0x%02x Response:0x%02x}",
		p.InitiatorTaskTag, p.StatSN, p.Status, p.Response)
}

func (p *TaskMgmtResp) String() string {
	return fmt.Sprintf("TaskMgmtResp{ITT:0x%08x Response:0x%02x StatSN:%d}",
		p.InitiatorTaskTag, p.Response, p.StatSN)
}

func (p *LoginResp) String() string {
	return fmt.Sprintf("LoginResp{ITT:0x%08x CSG:%d NSG:%d T:%t StatSN:%d StatusClass:0x%02x StatusDetail:0x%02x}",
		p.InitiatorTaskTag, p.CSG, p.NSG, p.Transit, p.StatSN, p.StatusClass, p.StatusDetail)
}

func (p *TextResp) String() string {
	return fmt.Sprintf("TextResp{ITT:0x%08x C:%t StatSN:%d}",
		p.InitiatorTaskTag, p.Continue, p.StatSN)
}

func (p *DataIn) String() string {
	return fmt.Sprintf("DataIn{ITT:0x%08x StatSN:%d DataSN:%d Offset:%d Len:%d S:%t}",
		p.InitiatorTaskTag, p.StatSN, p.DataSN, p.BufferOffset, len(p.Data), p.HasStatus)
}

func (p *LogoutResp) String() string {
	return fmt.Sprintf("LogoutResp{ITT:0x%08x Response:0x%02x StatSN:%d T2W:%d T2R:%d}",
		p.InitiatorTaskTag, p.Response, p.StatSN, p.Time2Wait, p.Time2Retain)
}

func (p *R2T) String() string {
	return fmt.Sprintf("R2T{ITT:0x%08x TTT:0x%08x R2TSN:%d Offset:%d DDTL:%d}",
		p.InitiatorTaskTag, p.TargetTransferTag, p.R2TSN, p.BufferOffset, p.DesiredDataTransferLength)
}

func (p *AsyncMsg) String() string {
	return fmt.Sprintf("AsyncMsg{ITT:0x%08x Event:%d StatSN:%d}",
		p.InitiatorTaskTag, p.AsyncEvent, p.StatSN)
}

func (p *Reject) String() string {
	return fmt.Sprintf("Reject{ITT:0x%08x Reason:0x%02x StatSN:%d}",
		p.InitiatorTaskTag, p.Reason, p.StatSN)
}
