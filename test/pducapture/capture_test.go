package pducapture_test

import (
	"context"
	"testing"

	"github.com/uiscsi/uiscsi"
	"github.com/uiscsi/uiscsi/internal/pdu"
	"github.com/uiscsi/uiscsi/test/pducapture"
)

// marshalPDU is a test helper that marshals a PDU into a byte slice.
func marshalPDU(t *testing.T, p pdu.PDU) []byte {
	t.Helper()
	bhs, err := p.MarshalBHS()
	if err != nil {
		t.Fatalf("MarshalBHS: %v", err)
	}
	return bhs[:]
}

func TestRecorder_Hook_DecodesAndStores(t *testing.T) {
	var rec pducapture.Recorder
	hook := rec.Hook()

	// First PDU: SCSICommand sent.
	cmd := &pdu.SCSICommand{
		Header: pdu.Header{OpCode_: pdu.OpSCSICommand, Final: true},
		CmdSN:  1,
	}
	hook(context.Background(), uiscsi.PDUSend, marshalPDU(t, cmd))

	all := rec.All()
	if len(all) != 1 {
		t.Fatalf("All: got %d PDUs, want 1", len(all))
	}
	if all[0].Direction != uiscsi.PDUSend {
		t.Errorf("Direction: got %d, want PDUSend", all[0].Direction)
	}
	if all[0].Decoded.Opcode() != pdu.OpSCSICommand {
		t.Errorf("Opcode: got 0x%02X, want 0x%02X", all[0].Decoded.Opcode(), pdu.OpSCSICommand)
	}
	if all[0].Seq != 0 {
		t.Errorf("Seq: got %d, want 0", all[0].Seq)
	}

	// Second PDU: LoginReq sent.
	loginReq := &pdu.LoginReq{
		Header:     pdu.Header{OpCode_: pdu.OpLoginReq, Final: true},
		VersionMax: 0x00,
		VersionMin: 0x00,
		ISID:       [6]byte{0x40, 0x01, 0x02, 0x03, 0x04, 0x05},
		CmdSN:      1,
	}
	hook(context.Background(), uiscsi.PDUSend, marshalPDU(t, loginReq))

	all = rec.All()
	if len(all) != 2 {
		t.Fatalf("All: got %d PDUs, want 2", len(all))
	}
	if all[1].Decoded.Opcode() != pdu.OpLoginReq {
		t.Errorf("Opcode[1]: got 0x%02X, want 0x%02X", all[1].Decoded.Opcode(), pdu.OpLoginReq)
	}
	if all[1].Seq != 1 {
		t.Errorf("Seq[1]: got %d, want 1", all[1].Seq)
	}
}

func TestRecorder_Hook_ShortData(t *testing.T) {
	var rec pducapture.Recorder
	hook := rec.Hook()

	// Less than 48 bytes should be silently skipped.
	hook(context.Background(), uiscsi.PDUSend, make([]byte, 10))

	if got := rec.All(); len(got) != 0 {
		t.Fatalf("All: got %d PDUs, want 0 (short data should be skipped)", len(got))
	}
}

func TestRecorder_Hook_InvalidOpcode(t *testing.T) {
	var rec pducapture.Recorder
	hook := rec.Hook()

	// 48 bytes with invalid opcode (0x3E is not a valid opcode).
	data := make([]byte, 48)
	data[0] = 0x3E
	hook(context.Background(), uiscsi.PDUSend, data)

	if got := rec.All(); len(got) != 0 {
		t.Fatalf("All: got %d PDUs, want 0 (invalid opcode should be skipped)", len(got))
	}
}

func TestRecorder_Filter(t *testing.T) {
	var rec pducapture.Recorder
	hook := rec.Hook()

	// PDU 1: SCSICommand sent.
	cmd := &pdu.SCSICommand{
		Header: pdu.Header{OpCode_: pdu.OpSCSICommand, Final: true},
		CmdSN:  1,
	}
	hook(context.Background(), uiscsi.PDUSend, marshalPDU(t, cmd))

	// PDU 2: SCSIResponse received (simulating target response).
	resp := &pdu.SCSIResponse{
		Header: pdu.Header{OpCode_: pdu.OpSCSIResponse, Final: true},
	}
	hook(context.Background(), uiscsi.PDUReceive, marshalPDU(t, resp))

	// PDU 3: LoginReq sent.
	loginReq := &pdu.LoginReq{
		Header:     pdu.Header{OpCode_: pdu.OpLoginReq, Final: true},
		VersionMax: 0x00,
		ISID:       [6]byte{0x40, 0x01, 0x02, 0x03, 0x04, 0x05},
		CmdSN:      1,
	}
	hook(context.Background(), uiscsi.PDUSend, marshalPDU(t, loginReq))

	// Filter: SCSICommand sent.
	got := rec.Filter(pdu.OpSCSICommand, uiscsi.PDUSend)
	if len(got) != 1 {
		t.Errorf("Filter(SCSICommand, Send): got %d, want 1", len(got))
	}

	// Sent shorthand.
	got = rec.Sent(pdu.OpSCSICommand)
	if len(got) != 1 {
		t.Errorf("Sent(SCSICommand): got %d, want 1", len(got))
	}

	// Received shorthand.
	got = rec.Received(pdu.OpSCSIResponse)
	if len(got) != 1 {
		t.Errorf("Received(SCSIResponse): got %d, want 1", len(got))
	}

	// No SCSICommand received.
	got = rec.Received(pdu.OpSCSICommand)
	if len(got) != 0 {
		t.Errorf("Received(SCSICommand): got %d, want 0", len(got))
	}
}

func TestRecorder_RawDefensiveCopy(t *testing.T) {
	var rec pducapture.Recorder
	hook := rec.Hook()

	cmd := &pdu.SCSICommand{
		Header: pdu.Header{OpCode_: pdu.OpSCSICommand, Final: true},
		CmdSN:  42,
	}
	data := marshalPDU(t, cmd)

	// Save original for comparison.
	original := make([]byte, len(data))
	copy(original, data)

	hook(context.Background(), uiscsi.PDUSend, data)

	// Mutate the original slice after capture.
	for i := range data {
		data[i] = 0xFF
	}

	// Captured raw should still match original.
	all := rec.All()
	if len(all) != 1 {
		t.Fatalf("All: got %d PDUs, want 1", len(all))
	}
	for i, b := range all[0].Raw {
		if b != original[i] {
			t.Fatalf("Raw[%d]: got 0x%02X, want 0x%02X (defensive copy failed)", i, b, original[i])
		}
	}
}
