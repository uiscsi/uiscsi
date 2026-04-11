package session

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/uiscsi/uiscsi/internal/pdu"
	"github.com/uiscsi/uiscsi/internal/transport"
)

func TestLogoutGraceful(t *testing.T) {
	sess, targetConn := newTestSessionWithOptions(t,
		WithKeepaliveInterval(10*time.Second),
	)

	// Logout in a goroutine since it blocks waiting for response.
	errCh := make(chan error, 1)
	go func() {
		errCh <- sess.Logout(context.Background())
	}()

	// Read the LogoutReq from the target side.
	raw, err := transport.ReadRawPDU(targetConn, false, false, 0)
	if err != nil {
		t.Fatalf("read LogoutReq: %v", err)
	}

	opcode := raw.BHS[0] & 0x3f
	if pdu.OpCode(opcode) != pdu.OpLogoutReq {
		t.Fatalf("opcode: got 0x%02X, want 0x%02X (LogoutReq)", opcode, pdu.OpLogoutReq)
	}

	// Verify reason code = 0 (close session).
	reasonCode := raw.BHS[1] & 0x7f
	if reasonCode != 0 {
		t.Fatalf("reason code: got %d, want 0", reasonCode)
	}

	itt := binary.BigEndian.Uint32(raw.BHS[16:20])

	// Respond with LogoutResp (success).
	writeLogoutRespPDU(t, targetConn, &pdu.LogoutResp{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: itt,
		},
		Response: 0,
		StatSN:   1,
		ExpCmdSN: 2,
		MaxCmdSN: 10,
	})

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Logout error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Logout did not complete within deadline")
	}
}

func TestLogoutTimeout(t *testing.T) {
	sess, _ := newTestSessionWithOptions(t,
		WithKeepaliveInterval(10*time.Second),
	)

	// Set a short timeout context so logout times out quickly.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Don't respond to the LogoutReq -- let it timeout.
	err := sess.Logout(ctx)
	if err == nil {
		t.Fatal("expected Logout to return error on timeout")
	}
}

func TestLogoutDrainsInFlight(t *testing.T) {
	sess, targetConn := newTestSessionWithOptions(t,
		WithKeepaliveInterval(10*time.Second),
	)

	// Submit a read command.
	cmd := Command{
		Read:                    true,
		ExpectedDataTransferLen: 5,
	}
	cmd.CDB[0] = 0x28 // READ(10)

	resultCh, err := sess.Submit(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Read the SCSI command PDU from target side.
	scsiCmd := readSCSICommandPDU(t, targetConn)

	// Start Logout in background -- it should wait for the command to complete.
	logoutDone := make(chan error, 1)
	go func() {
		logoutDone <- sess.Logout(context.Background())
	}()

	// Give the Logout goroutine time to start draining.
	time.Sleep(50 * time.Millisecond)

	// Complete the in-flight read command.
	writeDataInPDU(t, targetConn, &pdu.DataIn{
		Header:    pdu.Header{InitiatorTaskTag: scsiCmd.InitiatorTaskTag},
		HasStatus: true,
		Status:    0x00,
		StatSN:    1,
		ExpCmdSN:  2,
		MaxCmdSN:  10,
		DataSN:    0,
		Data:      []byte("hello"),
	})

	// Verify the read result is received.
	result := <-resultCh
	if result.Err != nil {
		t.Fatalf("read result error: %v", result.Err)
	}

	// Now the Logout should proceed with the LogoutReq.
	raw, err := transport.ReadRawPDU(targetConn, false, false, 0)
	if err != nil {
		t.Fatalf("read LogoutReq: %v", err)
	}

	opcode := raw.BHS[0] & 0x3f
	if pdu.OpCode(opcode) != pdu.OpLogoutReq {
		t.Fatalf("opcode: got 0x%02X, want 0x%02X (LogoutReq)", opcode, pdu.OpLogoutReq)
	}

	itt := binary.BigEndian.Uint32(raw.BHS[16:20])

	// Respond with LogoutResp (success).
	writeLogoutRespPDU(t, targetConn, &pdu.LogoutResp{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: itt,
		},
		Response: 0,
		StatSN:   2,
		ExpCmdSN: 3,
		MaxCmdSN: 10,
	})

	select {
	case err := <-logoutDone:
		if err != nil {
			t.Fatalf("Logout error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Logout did not complete within deadline")
	}
}

func TestLogoutReasonCode2(t *testing.T) {
	sess, targetConn := newTestSessionWithOptions(t,
		WithKeepaliveInterval(10*time.Second),
	)

	// LogoutConnection in background with reason code 2.
	errCh := make(chan error, 1)
	go func() {
		errCh <- sess.LogoutConnection(context.Background(), 2)
	}()

	// Read the LogoutReq.
	raw, err := transport.ReadRawPDU(targetConn, false, false, 0)
	if err != nil {
		t.Fatalf("read LogoutReq: %v", err)
	}

	opcode := raw.BHS[0] & 0x3f
	if pdu.OpCode(opcode) != pdu.OpLogoutReq {
		t.Fatalf("opcode: got 0x%02X, want 0x%02X (LogoutReq)", opcode, pdu.OpLogoutReq)
	}

	// Verify reason code = 2 (remove connection for recovery).
	reasonCode := raw.BHS[1] & 0x7f
	if reasonCode != 2 {
		t.Fatalf("reason code: got %d, want 2", reasonCode)
	}

	itt := binary.BigEndian.Uint32(raw.BHS[16:20])

	// Respond with LogoutResp (success).
	writeLogoutRespPDU(t, targetConn, &pdu.LogoutResp{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: itt,
		},
		Response: 0,
		StatSN:   1,
		ExpCmdSN: 2,
		MaxCmdSN: 10,
	})

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("LogoutConnection error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("LogoutConnection did not complete within deadline")
	}
}

func TestCloseWithLogout(t *testing.T) {
	sess, targetConn := newTestSessionWithOptions(t,
		WithKeepaliveInterval(10*time.Second),
	)

	// Close in background -- should trigger logout.
	closeDone := make(chan error, 1)
	go func() {
		closeDone <- sess.Close()
	}()

	// Read the LogoutReq from Close.
	raw, err := transport.ReadRawPDU(targetConn, false, false, 0)
	if err != nil {
		t.Fatalf("read LogoutReq: %v", err)
	}

	opcode := raw.BHS[0] & 0x3f
	if pdu.OpCode(opcode) != pdu.OpLogoutReq {
		t.Fatalf("opcode: got 0x%02X, want 0x%02X (LogoutReq)", opcode, pdu.OpLogoutReq)
	}

	itt := binary.BigEndian.Uint32(raw.BHS[16:20])

	// Respond with LogoutResp (success).
	writeLogoutRespPDU(t, targetConn, &pdu.LogoutResp{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: itt,
		},
		Response: 0,
		StatSN:   1,
		ExpCmdSN: 1,
		MaxCmdSN: 10,
	})

	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not complete within deadline")
	}
}

// writeLogoutRespPDU encodes and writes a LogoutResp PDU to the target conn.
func writeLogoutRespPDU(t *testing.T, conn net.Conn, resp *pdu.LogoutResp) {
	t.Helper()
	resp.OpCode_ = pdu.OpLogoutResp
	raw := buildRawPDU(t, resp)
	if err := transport.WriteRawPDU(conn, raw); err != nil {
		t.Fatalf("write LogoutResp: %v", err)
	}
}
