package session

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/rkujawa/uiscsi/internal/login"
	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
)

// newTestSessionWithOptions creates a Session with custom options for testing.
func newTestSessionWithOptions(t *testing.T, opts ...SessionOption) (*Session, net.Conn) {
	t.Helper()
	clientConn, targetConn := net.Pipe()

	tc := transport.NewConnFromNetConn(clientConn)
	params := login.Defaults()
	params.CmdSN = 1
	params.ExpStatSN = 1

	sess := NewSession(tc, params, opts...)
	t.Cleanup(func() {
		sess.Close()
		targetConn.Close()
	})

	return sess, targetConn
}

func TestKeepalivePing(t *testing.T) {
	sess, targetConn := newTestSessionWithOptions(t,
		WithKeepaliveInterval(50*time.Millisecond),
		WithKeepaliveTimeout(500*time.Millisecond),
	)

	// Read the NOP-Out ping from the session.
	raw, err := transport.ReadRawPDU(targetConn, false, false)
	if err != nil {
		t.Fatalf("read NOP-Out: %v", err)
	}

	opcode := raw.BHS[0] & 0x3f
	if pdu.OpCode(opcode) != pdu.OpNOPOut {
		t.Fatalf("opcode: got 0x%02X, want 0x%02X (NOP-Out)", opcode, pdu.OpNOPOut)
	}

	// Verify TTT=0xFFFFFFFF (initiator-originated ping).
	ttt := binary.BigEndian.Uint32(raw.BHS[20:24])
	if ttt != 0xFFFFFFFF {
		t.Fatalf("TTT: got 0x%08X, want 0xFFFFFFFF", ttt)
	}

	// Verify ITT is not 0xFFFFFFFF (should be a real ITT).
	itt := binary.BigEndian.Uint32(raw.BHS[16:20])
	if itt == 0xFFFFFFFF {
		t.Fatal("ITT should not be 0xFFFFFFFF for initiator ping")
	}

	// Respond with NOP-In.
	nopIn := &pdu.NOPIn{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: itt,
		},
		TargetTransferTag: 0xFFFFFFFF,
		StatSN:            1,
		ExpCmdSN:          1,
		MaxCmdSN:          10,
	}
	writeNOPInPDU(t, targetConn, nopIn)

	// Wait a bit and verify session has no error.
	time.Sleep(100 * time.Millisecond)
	if err := sess.Err(); err != nil {
		t.Fatalf("session error after successful keepalive: %v", err)
	}
}

func TestKeepaliveTimeout(t *testing.T) {
	sess, _ := newTestSessionWithOptions(t,
		WithKeepaliveInterval(50*time.Millisecond),
		WithKeepaliveTimeout(100*time.Millisecond),
	)

	// Don't respond to the NOP-Out -- let it timeout.
	// Wait for the session to detect the timeout.
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			t.Fatal("session did not detect keepalive timeout within deadline")
		case <-ticker.C:
			if err := sess.Err(); err != nil {
				// Keepalive timeout detected.
				return
			}
		}
	}
}

func TestUnsolicitedNOPInResponse(t *testing.T) {
	sess, targetConn := newTestSessionWithOptions(t,
		// Use a long keepalive interval so the keepalive loop doesn't interfere.
		WithKeepaliveInterval(10*time.Second),
		WithKeepaliveTimeout(5*time.Second),
	)

	// Send an unsolicited NOP-In from the target with TTT=0x1234.
	nopIn := &pdu.NOPIn{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: 0xFFFFFFFF, // unsolicited
		},
		TargetTransferTag: 0x1234,
		StatSN:            1,
		ExpCmdSN:          1,
		MaxCmdSN:          10,
	}
	writeNOPInPDU(t, targetConn, nopIn)

	// Read the NOP-Out response.
	raw, err := transport.ReadRawPDU(targetConn, false, false)
	if err != nil {
		t.Fatalf("read NOP-Out response: %v", err)
	}

	opcode := raw.BHS[0] & 0x3f
	if pdu.OpCode(opcode) != pdu.OpNOPOut {
		t.Fatalf("opcode: got 0x%02X, want 0x%02X (NOP-Out)", opcode, pdu.OpNOPOut)
	}

	// Verify ITT=0xFFFFFFFF (response, not new task).
	itt := binary.BigEndian.Uint32(raw.BHS[16:20])
	if itt != 0xFFFFFFFF {
		t.Fatalf("ITT: got 0x%08X, want 0xFFFFFFFF", itt)
	}

	// Verify TTT echoes the target's value.
	ttt := binary.BigEndian.Uint32(raw.BHS[20:24])
	if ttt != 0x1234 {
		t.Fatalf("TTT: got 0x%08X, want 0x00001234", ttt)
	}

	// Verify no session error.
	if err := sess.Err(); err != nil {
		t.Fatalf("unexpected session error: %v", err)
	}
}

func TestAsyncEventCallback(t *testing.T) {
	evtCh := make(chan AsyncEvent, 1)

	_, targetConn := newTestSessionWithOptions(t,
		WithKeepaliveInterval(10*time.Second),
		WithAsyncHandler(func(evt AsyncEvent) {
			evtCh <- evt
		}),
	)

	// Send AsyncMsg with EventCode=0 (SCSI async event).
	asyncMsg := &pdu.AsyncMsg{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: 0xFFFFFFFF,
		},
		StatSN:     1,
		ExpCmdSN:   1,
		MaxCmdSN:   10,
		AsyncEvent: 0,
		AsyncVCode: 42,
		Parameter1: 100,
		Parameter2: 200,
		Parameter3: 300,
	}
	writeAsyncMsgPDU(t, targetConn, asyncMsg)

	select {
	case evt := <-evtCh:
		if evt.EventCode != 0 {
			t.Fatalf("EventCode: got %d, want 0", evt.EventCode)
		}
		if evt.VendorCode != 42 {
			t.Fatalf("VendorCode: got %d, want 42", evt.VendorCode)
		}
		if evt.Parameter1 != 100 {
			t.Fatalf("Parameter1: got %d, want 100", evt.Parameter1)
		}
		if evt.Parameter2 != 200 {
			t.Fatalf("Parameter2: got %d, want 200", evt.Parameter2)
		}
		if evt.Parameter3 != 300 {
			t.Fatalf("Parameter3: got %d, want 300", evt.Parameter3)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("async handler not called within deadline")
	}
}

// writeNOPInPDU encodes and writes a NOP-In PDU to the target conn.
func writeNOPInPDU(t *testing.T, conn net.Conn, nopin *pdu.NOPIn) {
	t.Helper()
	nopin.Header.OpCode_ = pdu.OpNOPIn
	raw := buildRawPDU(t, nopin)
	if err := transport.WriteRawPDU(conn, raw); err != nil {
		t.Fatalf("write NOP-In: %v", err)
	}
}

// writeAsyncMsgPDU encodes and writes an AsyncMsg PDU to the target conn.
func writeAsyncMsgPDU(t *testing.T, conn net.Conn, async *pdu.AsyncMsg) {
	t.Helper()
	async.Header.OpCode_ = pdu.OpAsyncMsg
	raw := buildRawPDU(t, async)
	if err := transport.WriteRawPDU(conn, raw); err != nil {
		t.Fatalf("write AsyncMsg: %v", err)
	}
}
