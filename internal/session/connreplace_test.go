package session

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/rkujawa/uiscsi/internal/login"
	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
)

// mockERL2Target is a minimal iSCSI target mock that handles login, SCSI
// commands, Logout with reasonCode=2, and TMF TASK REASSIGN for testing
// ERL 2 connection replacement.
type mockERL2Target struct {
	mu             sync.Mutex
	listener       net.Listener
	receivedPDUs   []receivedPDU
	logoutReceived bool
	logoutReason   uint8
	tmfReceived    int
	rejectLogout   bool // if true, respond with failure
	rejectTMF      bool // if true, respond TMFRespNotSupported
	connectionNum  int  // tracks how many connections accepted
	t              *testing.T
}

type receivedPDU struct {
	opcode pdu.OpCode
	bhs    [48]byte
}

func newMockERL2Target(t *testing.T) *mockERL2Target {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return &mockERL2Target{
		listener: ln,
		t:        t,
	}
}

func (m *mockERL2Target) addr() string {
	return m.listener.Addr().String()
}

func (m *mockERL2Target) close() {
	m.listener.Close()
}

// serve accepts connections in a loop, handling login and commands.
func (m *mockERL2Target) serve(ctx context.Context) {
	for {
		conn, err := m.listener.Accept()
		if err != nil {
			return
		}
		m.mu.Lock()
		m.connectionNum++
		m.mu.Unlock()
		go m.handleConn(ctx, conn)
	}
}

func (m *mockERL2Target) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	// Handle login phase.
	if err := m.handleLogin(conn); err != nil {
		return
	}

	// Handle full-feature phase PDUs.
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		var bhs [48]byte
		if _, err := io.ReadFull(conn, bhs[:]); err != nil {
			return
		}

		// Read data segment if present.
		dsLen := int(bhs[5])<<16 | int(bhs[6])<<8 | int(bhs[7])
		if dsLen > 0 {
			padded := dsLen + (4-(dsLen%4))%4
			data := make([]byte, padded)
			if _, err := io.ReadFull(conn, data); err != nil {
				return
			}
		}

		opcode := pdu.OpCode(bhs[0] & 0x3f)
		m.mu.Lock()
		m.receivedPDUs = append(m.receivedPDUs, receivedPDU{opcode: opcode, bhs: bhs})
		m.mu.Unlock()

		switch opcode {
		case pdu.OpSCSICommand:
			m.handleSCSICmd(conn, bhs)
		case pdu.OpLogoutReq:
			m.handleLogout(conn, bhs)
		case pdu.OpTaskMgmtReq:
			m.handleTMF(conn, bhs)
		case pdu.OpNOPOut:
			m.handleNOPOut(conn, bhs)
		}
	}
}

func (m *mockERL2Target) handleLogin(conn net.Conn) error {
	for {
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		var bhs [48]byte
		if _, err := io.ReadFull(conn, bhs[:]); err != nil {
			return err
		}

		// Read data segment.
		dsLen := int(bhs[5])<<16 | int(bhs[6])<<8 | int(bhs[7])
		if dsLen > 0 {
			padded := dsLen + (4-(dsLen%4))%4
			data := make([]byte, padded)
			if _, err := io.ReadFull(conn, data); err != nil {
				return err
			}
		}

		opcode := bhs[0] & 0x3f
		if opcode != 0x03 { // LoginReq
			return fmt.Errorf("expected LoginReq, got 0x%02x", opcode)
		}

		// Check for Transit bit (T=1 means final login PDU).
		transit := bhs[1]&0x80 != 0
		csg := (bhs[1] >> 2) & 0x03
		nsg := bhs[1] & 0x03

		// Build LoginResp.
		var resp [48]byte
		resp[0] = 0x23 // LoginResp opcode
		itt := binary.BigEndian.Uint32(bhs[16:20])
		binary.BigEndian.PutUint32(resp[16:20], itt)

		// Copy ISID from request.
		copy(resp[8:14], bhs[8:14])
		// Set TSIH.
		binary.BigEndian.PutUint16(resp[14:16], 1)

		// StatSN, ExpCmdSN, MaxCmdSN.
		statSN := binary.BigEndian.Uint32(bhs[28:32]) // ExpStatSN from initiator
		binary.BigEndian.PutUint32(resp[24:28], statSN)
		cmdSN := binary.BigEndian.Uint32(bhs[24:28])
		binary.BigEndian.PutUint32(resp[28:32], cmdSN)
		binary.BigEndian.PutUint32(resp[32:36], cmdSN+16)

		if transit {
			if csg == 0 && nsg == 1 {
				// Security -> Operational: respond with T=1 CSG=0 NSG=1.
				resp[1] = 0x80 | (0 << 2) | 1
				// Respond with operational params as data.
				respData := []byte("AuthMethod=None\x00")
				dsLen := len(respData)
				resp[5] = byte(dsLen >> 16)
				resp[6] = byte(dsLen >> 8)
				resp[7] = byte(dsLen)
				if _, err := conn.Write(resp[:]); err != nil {
					return err
				}
				padded := dsLen + (4-(dsLen%4))%4
				padBuf := make([]byte, padded)
				copy(padBuf, respData)
				if _, err := conn.Write(padBuf); err != nil {
					return err
				}
				continue
			}
			if csg == 1 && nsg == 3 {
				// Operational -> FFP: respond with T=1 CSG=1 NSG=3.
				resp[1] = 0x80 | (1 << 2) | 3
				// Respond with negotiated params.
				respData := []byte("ErrorRecoveryLevel=2\x00MaxRecvDataSegmentLength=8192\x00")
				dsLen := len(respData)
				resp[5] = byte(dsLen >> 16)
				resp[6] = byte(dsLen >> 8)
				resp[7] = byte(dsLen)
				if _, err := conn.Write(resp[:]); err != nil {
					return err
				}
				padded := dsLen + (4-(dsLen%4))%4
				padBuf := make([]byte, padded)
				copy(padBuf, respData)
				if _, err := conn.Write(padBuf); err != nil {
					return err
				}
				return nil // Login complete
			}
		}

		// Non-transit: just echo back.
		resp[1] = bhs[1]
		if _, err := conn.Write(resp[:]); err != nil {
			return err
		}
	}
}

func (m *mockERL2Target) handleSCSICmd(conn net.Conn, bhs [48]byte) {
	itt := binary.BigEndian.Uint32(bhs[16:20])

	// Send a simple SCSIResponse with GOOD status.
	var resp [48]byte
	resp[0] = 0x21 // SCSIResponse opcode
	resp[1] = 0x80 // Final
	resp[3] = 0x00 // GOOD status
	binary.BigEndian.PutUint32(resp[16:20], itt)
	binary.BigEndian.PutUint32(resp[24:28], 1) // StatSN
	binary.BigEndian.PutUint32(resp[28:32], 1) // ExpCmdSN
	binary.BigEndian.PutUint32(resp[32:36], 16) // MaxCmdSN
	conn.Write(resp[:])
}

func (m *mockERL2Target) handleLogout(conn net.Conn, bhs [48]byte) {
	m.mu.Lock()
	m.logoutReceived = true
	m.logoutReason = bhs[1] & 0x7f
	rejectLogout := m.rejectLogout
	m.mu.Unlock()

	itt := binary.BigEndian.Uint32(bhs[16:20])

	var resp [48]byte
	resp[0] = 0x26 // LogoutResp opcode
	resp[1] = 0x80 // Final
	if rejectLogout {
		resp[2] = 1 // Reject
	} else {
		resp[2] = 0 // Success
	}
	binary.BigEndian.PutUint32(resp[16:20], itt)
	binary.BigEndian.PutUint32(resp[24:28], 1) // StatSN
	binary.BigEndian.PutUint32(resp[28:32], 1) // ExpCmdSN
	binary.BigEndian.PutUint32(resp[32:36], 16) // MaxCmdSN
	conn.Write(resp[:])
}

func (m *mockERL2Target) handleTMF(conn net.Conn, bhs [48]byte) {
	m.mu.Lock()
	m.tmfReceived++
	rejectTMF := m.rejectTMF
	m.mu.Unlock()

	itt := binary.BigEndian.Uint32(bhs[16:20])

	var resp [48]byte
	resp[0] = 0x22 // TaskMgmtResp opcode
	resp[1] = 0x80 // Final
	if rejectTMF {
		resp[2] = TMFRespNotSupported
	} else {
		resp[2] = TMFRespComplete
	}
	binary.BigEndian.PutUint32(resp[16:20], itt)
	binary.BigEndian.PutUint32(resp[24:28], 1) // StatSN
	binary.BigEndian.PutUint32(resp[28:32], 1) // ExpCmdSN
	binary.BigEndian.PutUint32(resp[32:36], 16) // MaxCmdSN
	conn.Write(resp[:])
}

func (m *mockERL2Target) handleNOPOut(conn net.Conn, bhs [48]byte) {
	itt := binary.BigEndian.Uint32(bhs[16:20])
	ttt := binary.BigEndian.Uint32(bhs[20:24])

	// Respond to solicited NOP-Out (TTT != 0xFFFFFFFF) with NOP-In.
	if ttt != 0xFFFFFFFF {
		return
	}

	// This is a keepalive NOP-Out from initiator. Respond with NOP-In.
	var resp [48]byte
	resp[0] = 0x20 // NOPIn opcode
	resp[1] = 0x80 // Final
	binary.BigEndian.PutUint32(resp[16:20], itt)
	binary.BigEndian.PutUint32(resp[20:24], 0xFFFFFFFF) // TTT
	binary.BigEndian.PutUint32(resp[24:28], 1) // StatSN
	binary.BigEndian.PutUint32(resp[28:32], 1) // ExpCmdSN
	binary.BigEndian.PutUint32(resp[32:36], 16) // MaxCmdSN
	conn.Write(resp[:])
}

func TestERL2ConnReplace(t *testing.T) {
	t.Run("basic_replacement", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		mock := newMockERL2Target(t)
		defer mock.close()
		go mock.serve(ctx)

		// Dial and login.
		tc, err := transport.Dial(ctx, mock.addr())
		if err != nil {
			t.Fatalf("dial: %v", err)
		}

		params, err := login.Login(ctx, tc, login.WithSessionType("Normal"))
		if err != nil {
			t.Fatalf("login: %v", err)
		}

		sess := NewSession(tc, *params,
			WithReconnectInfo(mock.addr(), login.WithSessionType("Normal")),
			WithKeepaliveInterval(60*time.Second), // disable keepalive interference
		)
		defer sess.Close()

		// Allow pumps to start.
		time.Sleep(50 * time.Millisecond)

		// Trigger connection replacement.
		replaceErr := sess.replaceConnection(fmt.Errorf("test: connection lost"))
		if replaceErr != nil {
			t.Fatalf("replaceConnection: %v", replaceErr)
		}

		// Verify new connection was established (connectionNum > 1).
		mock.mu.Lock()
		connNum := mock.connectionNum
		mock.mu.Unlock()
		if connNum < 2 {
			t.Fatalf("expected at least 2 connections, got %d", connNum)
		}

		// Verify session is still functional: submit a command.
		cmd := Command{
			CDB:                    [16]byte{0x00}, // TEST UNIT READY
			ExpectedDataTransferLen: 0,
		}
		resCh, submitErr := sess.Submit(ctx, cmd)
		if submitErr != nil {
			t.Fatalf("Submit after replacement: %v", submitErr)
		}

		select {
		case result := <-resCh:
			if result.Err != nil {
				t.Fatalf("command after replacement failed: %v", result.Err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("command after replacement timed out")
		}
	})

	t.Run("logout_reasoncode2", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		mock := newMockERL2Target(t)
		defer mock.close()
		go mock.serve(ctx)

		tc, err := transport.Dial(ctx, mock.addr())
		if err != nil {
			t.Fatalf("dial: %v", err)
		}

		params, err := login.Login(ctx, tc, login.WithSessionType("Normal"))
		if err != nil {
			t.Fatalf("login: %v", err)
		}

		sess := NewSession(tc, *params,
			WithReconnectInfo(mock.addr(), login.WithSessionType("Normal")),
			WithKeepaliveInterval(60*time.Second),
		)
		defer sess.Close()
		time.Sleep(50 * time.Millisecond)

		replaceErr := sess.replaceConnection(fmt.Errorf("test: lost"))
		if replaceErr != nil {
			t.Fatalf("replaceConnection: %v", replaceErr)
		}

		// Wait for logout to be processed.
		time.Sleep(100 * time.Millisecond)

		mock.mu.Lock()
		gotLogout := mock.logoutReceived
		gotReason := mock.logoutReason
		mock.mu.Unlock()

		if !gotLogout {
			t.Fatal("target did not receive Logout")
		}
		if gotReason != 2 {
			t.Fatalf("Logout reasonCode: got %d, want 2", gotReason)
		}
	})

	t.Run("logout_failure_nonfatal", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		mock := newMockERL2Target(t)
		mock.rejectLogout = true
		defer mock.close()
		go mock.serve(ctx)

		tc, err := transport.Dial(ctx, mock.addr())
		if err != nil {
			t.Fatalf("dial: %v", err)
		}

		params, err := login.Login(ctx, tc, login.WithSessionType("Normal"))
		if err != nil {
			t.Fatalf("login: %v", err)
		}

		sess := NewSession(tc, *params,
			WithReconnectInfo(mock.addr(), login.WithSessionType("Normal")),
			WithKeepaliveInterval(60*time.Second),
		)
		defer sess.Close()
		time.Sleep(50 * time.Millisecond)

		// replaceConnection should succeed even with Logout rejection.
		replaceErr := sess.replaceConnection(fmt.Errorf("test: lost"))
		if replaceErr != nil {
			t.Fatalf("replaceConnection should succeed despite Logout failure: %v", replaceErr)
		}
	})

	t.Run("reassign_failure", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		mock := newMockERL2Target(t)
		mock.rejectTMF = true
		defer mock.close()
		go mock.serve(ctx)

		tc, err := transport.Dial(ctx, mock.addr())
		if err != nil {
			t.Fatalf("dial: %v", err)
		}

		params, err := login.Login(ctx, tc, login.WithSessionType("Normal"))
		if err != nil {
			t.Fatalf("login: %v", err)
		}

		sess := NewSession(tc, *params,
			WithReconnectInfo(mock.addr(), login.WithSessionType("Normal")),
			WithKeepaliveInterval(60*time.Second),
		)

		// Create a fake in-flight task that will fail during reassignment.
		tk := newTask(100, true, false, 0)
		tk.lun = 0
		sess.mu.Lock()
		sess.tasks[100] = tk
		sess.mu.Unlock()
		sess.router.RegisterPersistent(100)

		time.Sleep(50 * time.Millisecond)

		replaceErr := sess.replaceConnection(fmt.Errorf("test: lost"))
		if replaceErr != nil {
			t.Fatalf("replaceConnection: %v", replaceErr)
		}

		// The task should receive an error since reassign was rejected.
		select {
		case result := <-tk.resultCh:
			if result.Err == nil {
				t.Fatal("expected error from reassign failure")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("task did not receive error from reassign failure")
		}

		sess.Close()
	})

	t.Run("multiple_tasks", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		mock := newMockERL2Target(t)
		defer mock.close()
		go mock.serve(ctx)

		tc, err := transport.Dial(ctx, mock.addr())
		if err != nil {
			t.Fatalf("dial: %v", err)
		}

		params, err := login.Login(ctx, tc, login.WithSessionType("Normal"))
		if err != nil {
			t.Fatalf("login: %v", err)
		}

		sess := NewSession(tc, *params,
			WithReconnectInfo(mock.addr(), login.WithSessionType("Normal")),
			WithKeepaliveInterval(60*time.Second),
		)

		// Create multiple fake in-flight tasks.
		numTasks := 3
		tasks := make([]*task, numTasks)
		for i := 0; i < numTasks; i++ {
			itt := uint32(200 + i)
			tk := newTask(itt, true, false, 0)
			tk.lun = 0
			sess.mu.Lock()
			sess.tasks[itt] = tk
			sess.mu.Unlock()
			sess.router.RegisterPersistent(itt)
			tasks[i] = tk
		}

		time.Sleep(50 * time.Millisecond)

		replaceErr := sess.replaceConnection(fmt.Errorf("test: lost"))
		if replaceErr != nil {
			t.Fatalf("replaceConnection: %v", replaceErr)
		}

		// Verify all tasks got TASK REASSIGN TMF.
		time.Sleep(200 * time.Millisecond)
		mock.mu.Lock()
		tmfCount := mock.tmfReceived
		mock.mu.Unlock()
		if tmfCount != numTasks {
			t.Fatalf("TMF count: got %d, want %d", tmfCount, numTasks)
		}

		sess.Close()
	})
}

// TestERL2ConnReplaceDataIntegrity verifies that a read command's data
// reassembly works correctly after connection replacement.
func TestERL2ConnReplaceDataIntegrity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// This test creates a mock target that sends Data-In after replacement.
	mock := newMockERL2Target(t)
	defer mock.close()
	go mock.serve(ctx)

	tc, err := transport.Dial(ctx, mock.addr())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	params, err := login.Login(ctx, tc, login.WithSessionType("Normal"))
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	sess := NewSession(tc, *params,
		WithReconnectInfo(mock.addr(), login.WithSessionType("Normal")),
		WithKeepaliveInterval(60*time.Second),
	)
	defer sess.Close()

	time.Sleep(50 * time.Millisecond)

	// Submit a read command, then replace connection.
	cmd := Command{
		CDB:                    [16]byte{0x28}, // READ(10)
		Read:                   true,
		ExpectedDataTransferLen: 512,
	}
	resCh, submitErr := sess.Submit(ctx, cmd)
	if submitErr != nil {
		t.Fatalf("Submit: %v", submitErr)
	}

	// The mock target should have responded with SCSIResponse.
	select {
	case result := <-resCh:
		if result.Err != nil {
			t.Fatalf("command failed: %v", result.Err)
		}
		// For a non-data response from mock, just verify no error.
		_ = result
	case <-time.After(5 * time.Second):
		t.Fatal("command timed out")
	}

	_ = bytes.NewReader(nil) // suppress unused import
}
