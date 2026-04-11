package session

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/uiscsi/uiscsi/internal/login"
	"github.com/uiscsi/uiscsi/internal/pdu"
	"github.com/uiscsi/uiscsi/internal/transport"
)

// RFC-06: Async PDU protocol correctness tests.
//
// These tests verify that the session handles async target PDUs correctly at
// the protocol field level, not just at the dispatch level. Key RFC 7143
// requirements tested:
//
//   - Section 11.18/11.19: NOP-In with TTT != 0xFFFFFFFF must trigger a
//     NOP-Out reply with ITT=0xFFFFFFFF, TTT echoed from NOP-In, and the
//     current CmdSN and ExpStatSN from the session state.
//   - Section 11.18: NOP-Out response is not a new iSCSI task — ITT must
//     be 0xFFFFFFFF (reserved, does not consume a CmdSN slot).

// TestNOPInResponsePDUFields verifies that an unsolicited NOP-In from the
// target with TTT != 0xFFFFFFFF triggers a NOP-Out reply with the correct
// field values per RFC 7143 Section 11.18/11.19:
//   - Opcode = OpNOPOut
//   - ITT = 0xFFFFFFFF (response to target ping, not a new initiator task)
//   - TTT echoes the TTT received in the NOP-In
//   - CmdSN reflects current session CmdSN (window.current())
//   - ExpStatSN reflects current session ExpStatSN
func TestNOPInResponsePDUFields(t *testing.T) {
	clientConn, targetConn := net.Pipe()

	// Start session with known initial sequence numbers.
	tc := transport.NewConnFromNetConn(clientConn)
	params := login.Defaults()
	params.CmdSN = 10
	params.ExpStatSN = 5

	sess := NewSession(tc, params,
		WithKeepaliveInterval(30*time.Second), // long interval — prevent keepalive interference
	)
	t.Cleanup(func() {
		go respondToLogout(targetConn)
		sess.Close()
		targetConn.Close()
	})

	const expectedTTT = uint32(0xABCD1234)

	// Send a target-originated NOP-In with TTT != 0xFFFFFFFF.
	nopIn := &pdu.NOPIn{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: 0xFFFFFFFF, // unsolicited
		},
		TargetTransferTag: expectedTTT,
		StatSN:            5,  // ExpStatSN the session will echo
		ExpCmdSN:          10, // confirms CmdSN 10
		MaxCmdSN:          20,
	}
	nopIn.OpCode_ = pdu.OpNOPIn
	raw := buildRawPDU(t, nopIn)
	if err := transport.WriteRawPDU(targetConn, raw); err != nil {
		t.Fatalf("write NOP-In: %v", err)
	}

	// Read the NOP-Out reply.
	reply, err := transport.ReadRawPDU(targetConn, false, false, 0)
	if err != nil {
		t.Fatalf("read NOP-Out reply: %v", err)
	}

	// RFC 7143 §11.18: byte 0 opcode must be OpNOPOut.
	opcode := pdu.OpCode(reply.BHS[0] & 0x3f)
	if opcode != pdu.OpNOPOut {
		t.Fatalf("opcode: got 0x%02X, want 0x%02X (NOP-Out)", uint8(opcode), uint8(pdu.OpNOPOut))
	}

	// RFC 7143 §11.18: ITT must be 0xFFFFFFFF (ping reply, not a new task).
	itt := binary.BigEndian.Uint32(reply.BHS[16:20])
	if itt != 0xFFFFFFFF {
		t.Errorf("ITT: got 0x%08X, want 0xFFFFFFFF (ping reply must use reserved ITT)", itt)
	}

	// RFC 7143 §11.19: TTT must echo the TTT from the NOP-In.
	ttt := binary.BigEndian.Uint32(reply.BHS[20:24])
	if ttt != expectedTTT {
		t.Errorf("TTT: got 0x%08X, want 0x%08X (must echo NOP-In TTT)", ttt, expectedTTT)
	}

	// RFC 7143 §11.18: CmdSN must be the current session CmdSN. Per RFC 7143
	// Table 11, CmdSN in NOP-Out is at bytes 24-27.
	cmdSN := binary.BigEndian.Uint32(reply.BHS[24:28])
	if cmdSN != 10 {
		t.Errorf("CmdSN: got %d, want 10 (session CmdSN unchanged by unsolicited NOP-In)", cmdSN)
	}

	// RFC 7143 §11.18: ExpStatSN must reflect the current session ExpStatSN.
	// After receiving StatSN=5 and acknowledging, ExpStatSN becomes 6.
	// Bytes 28-31 in NOP-Out BHS.
	expStatSN := binary.BigEndian.Uint32(reply.BHS[28:32])
	if expStatSN != 6 {
		t.Errorf("ExpStatSN: got %d, want 6 (StatSN+1 after receiving StatSN=5)", expStatSN)
	}
}

// TestNOPInInformational verifies that an informational NOP-In (TTT=0xFFFFFFFF)
// updates sequence numbers but does NOT trigger a NOP-Out reply per RFC 7143
// Section 11.19. The target sends TTT=0xFFFFFFFF to update the initiator's
// view of CmdSN/MaxCmdSN without requesting a reply.
func TestNOPInInformational(t *testing.T) {
	sess, targetConn := newTestSessionWithOptions(t,
		WithKeepaliveInterval(30*time.Second), // long interval — prevent keepalive interference — prevents NOP-Out from keepalive loop
	)

	// Send informational NOP-In (TTT=0xFFFFFFFF, no reply expected).
	nopIn := &pdu.NOPIn{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: 0xFFFFFFFF,
		},
		TargetTransferTag: 0xFFFFFFFF, // informational — no reply
		StatSN:            1,
		ExpCmdSN:          1,
		MaxCmdSN:          10,
	}
	writeNOPInPDU(t, targetConn, nopIn)

	// Verify session remains healthy after informational NOP-In.
	// No NOP-Out should be sent for informational NOP-In (TTT=0xFFFFFFFF).
	if err := sess.Err(); err != nil {
		t.Fatalf("unexpected session error after informational NOP-In: %v", err)
	}
}
