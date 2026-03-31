package pdu

import "fmt"

// OpCode represents an iSCSI opcode value as defined in RFC 7143 Section 11.
// The opcode occupies the lower 6 bits of BHS byte 0.
type OpCode uint8

const (
	// Initiator opcodes (8)
	OpNOPOut      OpCode = 0x00
	OpSCSICommand OpCode = 0x01
	OpTaskMgmtReq OpCode = 0x02
	OpLoginReq    OpCode = 0x03
	OpTextReq     OpCode = 0x04
	OpDataOut     OpCode = 0x05
	OpLogoutReq   OpCode = 0x06
	OpSNACKReq    OpCode = 0x10

	// Target opcodes (10)
	OpNOPIn        OpCode = 0x20
	OpSCSIResponse OpCode = 0x21
	OpTaskMgmtResp OpCode = 0x22
	OpLoginResp    OpCode = 0x23
	OpTextResp     OpCode = 0x24
	OpDataIn       OpCode = 0x25
	OpLogoutResp   OpCode = 0x26
	OpR2T          OpCode = 0x31
	OpAsyncMsg     OpCode = 0x32
	OpReject       OpCode = 0x3f
)

var opcodeNames = map[OpCode]string{
	OpNOPOut:       "NOP-Out",
	OpSCSICommand:  "SCSI Command",
	OpTaskMgmtReq:  "Task Management Request",
	OpLoginReq:     "Login Request",
	OpTextReq:      "Text Request",
	OpDataOut:      "Data-Out",
	OpLogoutReq:    "Logout Request",
	OpSNACKReq:     "SNACK Request",
	OpNOPIn:        "NOP-In",
	OpSCSIResponse: "SCSI Response",
	OpTaskMgmtResp: "Task Management Response",
	OpLoginResp:    "Login Response",
	OpTextResp:     "Text Response",
	OpDataIn:       "Data-In",
	OpLogoutResp:   "Logout Response",
	OpR2T:          "Ready To Transfer",
	OpAsyncMsg:     "Async Message",
	OpReject:       "Reject",
}

// String returns the human-readable name for the opcode.
func (o OpCode) String() string {
	if name, ok := opcodeNames[o]; ok {
		return name
	}
	return fmt.Sprintf("Unknown(0x%02x)", uint8(o))
}

// IsInitiator returns true if this opcode is sent by the initiator.
// Initiator opcodes have bit 5 clear (value < 0x20).
func (o OpCode) IsInitiator() bool {
	return o&0x20 == 0
}

// IsTarget returns true if this opcode is sent by the target.
// Target opcodes have bit 5 set (value >= 0x20).
func (o OpCode) IsTarget() bool {
	return o&0x20 != 0
}
