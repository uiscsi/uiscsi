package pdu

import "testing"

func TestOpcodeString(t *testing.T) {
	tests := []struct {
		op   OpCode
		want string
	}{
		{OpNOPOut, "NOP-Out"},
		{OpSCSICommand, "SCSI Command"},
		{OpTaskMgmtReq, "Task Management Request"},
		{OpLoginReq, "Login Request"},
		{OpTextReq, "Text Request"},
		{OpDataOut, "Data-Out"},
		{OpLogoutReq, "Logout Request"},
		{OpSNACKReq, "SNACK Request"},
		{OpNOPIn, "NOP-In"},
		{OpSCSIResponse, "SCSI Response"},
		{OpTaskMgmtResp, "Task Management Response"},
		{OpLoginResp, "Login Response"},
		{OpTextResp, "Text Response"},
		{OpDataIn, "Data-In"},
		{OpLogoutResp, "Logout Response"},
		{OpR2T, "Ready To Transfer"},
		{OpAsyncMsg, "Async Message"},
		{OpReject, "Reject"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.op.String(); got != tt.want {
				t.Errorf("OpCode(0x%02x).String() = %q, want %q", uint8(tt.op), got, tt.want)
			}
		})
	}
}

func TestOpcodeStringUnknown(t *testing.T) {
	unknown := OpCode(0xFF)
	if got := unknown.String(); got == "" {
		t.Error("String() for unknown opcode should not be empty")
	}
}

func TestOpcodeIsInitiator(t *testing.T) {
	initiatorOps := []OpCode{
		OpNOPOut, OpSCSICommand, OpTaskMgmtReq, OpLoginReq,
		OpTextReq, OpDataOut, OpLogoutReq, OpSNACKReq,
	}
	for _, op := range initiatorOps {
		if !op.IsInitiator() {
			t.Errorf("OpCode(0x%02x).IsInitiator() = false, want true", uint8(op))
		}
		if op.IsTarget() {
			t.Errorf("OpCode(0x%02x).IsTarget() = true, want false", uint8(op))
		}
	}
}

func TestOpcodeIsTarget(t *testing.T) {
	targetOps := []OpCode{
		OpNOPIn, OpSCSIResponse, OpTaskMgmtResp, OpLoginResp,
		OpTextResp, OpDataIn, OpLogoutResp, OpR2T, OpAsyncMsg, OpReject,
	}
	for _, op := range targetOps {
		if !op.IsTarget() {
			t.Errorf("OpCode(0x%02x).IsTarget() = false, want true", uint8(op))
		}
		if op.IsInitiator() {
			t.Errorf("OpCode(0x%02x).IsInitiator() = true, want false", uint8(op))
		}
	}
}

func TestOpcodeDistinct(t *testing.T) {
	allOps := []OpCode{
		OpNOPOut, OpSCSICommand, OpTaskMgmtReq, OpLoginReq,
		OpTextReq, OpDataOut, OpLogoutReq, OpSNACKReq,
		OpNOPIn, OpSCSIResponse, OpTaskMgmtResp, OpLoginResp,
		OpTextResp, OpDataIn, OpLogoutResp, OpR2T, OpAsyncMsg, OpReject,
	}
	seen := make(map[OpCode]bool)
	for _, op := range allOps {
		if seen[op] {
			t.Errorf("duplicate opcode value: 0x%02x", uint8(op))
		}
		seen[op] = true
	}
	if len(seen) != 18 {
		t.Errorf("expected 18 distinct opcodes, got %d", len(seen))
	}
}
