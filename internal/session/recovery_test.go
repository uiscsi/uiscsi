package session

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rkujawa/uiscsi/internal/login"
	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
)

// recoverableTarget is a minimal iSCSI target that accepts multiple
// connections in sequence (simulating a target that survives initiator
// reconnect). It handles AuthMethod=None login and responds to SCSI
// read commands with configurable data.
type recoverableTarget struct {
	ln       net.Listener
	t        *testing.T
	mu       sync.Mutex
	connCh   chan net.Conn // each accepted conn
	stopCh   chan struct{}
	readData []byte // data to return for read commands
}

func startRecoverableTarget(t *testing.T, readData []byte) *recoverableTarget {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	rt := &recoverableTarget{
		ln:       ln,
		t:        t,
		connCh:   make(chan net.Conn, 4),
		stopCh:   make(chan struct{}),
		readData: readData,
	}

	go rt.acceptLoop()
	t.Cleanup(func() {
		close(rt.stopCh)
		ln.Close()
	})

	return rt
}

func (rt *recoverableTarget) addr() string {
	return rt.ln.Addr().String()
}

func (rt *recoverableTarget) acceptLoop() {
	for {
		conn, err := rt.ln.Accept()
		if err != nil {
			select {
			case <-rt.stopCh:
				return
			default:
				return
			}
		}
		rt.connCh <- conn
		go rt.handleConn(conn)
	}
}

func (rt *recoverableTarget) handleConn(conn net.Conn) {
	defer conn.Close()

	// Handle login (AuthMethod=None, single PDU each way).
	if err := rt.handleLogin(conn); err != nil {
		rt.t.Logf("recoverableTarget: login error: %v", err)
		return
	}

	// Handle SCSI commands.
	rt.handleCommands(conn)
}

func (rt *recoverableTarget) handleLogin(conn net.Conn) error {
	var statSN uint32

	// Read security negotiation PDU.
	raw, err := transport.ReadRawPDU(conn, false, false)
	if err != nil {
		return fmt.Errorf("read security PDU: %w", err)
	}
	req := &pdu.LoginReq{}
	req.UnmarshalBHS(raw.BHS)

	// Send security response with transit to operational.
	secResp := &pdu.LoginResp{
		Header: pdu.Header{
			Final: true,
		},
		Transit:     true,
		CSG:         0,
		NSG:         1,
		VersionMax:  0,
		VersionActive:  0,
		ISID:        req.ISID,
		TSIH:        1, // Assign TSIH
		StatSN:      statSN,
		ExpCmdSN:    req.CmdSN,
		MaxCmdSN:    req.CmdSN + 32,
		StatusClass: 0,
		Data:        login.EncodeTextKV([]login.KeyValue{{Key: "AuthMethod", Value: "None"}}),
	}
	secResp.Header.DataSegmentLen = uint32(len(secResp.Data))
	bhs, err := secResp.MarshalBHS()
	if err != nil {
		return fmt.Errorf("marshal security resp: %w", err)
	}
	rawResp := &transport.RawPDU{BHS: bhs}
	if len(secResp.Data) > 0 {
		rawResp.DataSegment = secResp.Data
	}
	if err := transport.WriteRawPDU(conn, rawResp); err != nil {
		return fmt.Errorf("write security resp: %w", err)
	}
	statSN++

	// Read operational negotiation PDU.
	raw2, err := transport.ReadRawPDU(conn, false, false)
	if err != nil {
		return fmt.Errorf("read op PDU: %w", err)
	}
	req2 := &pdu.LoginReq{}
	req2.UnmarshalBHS(raw2.BHS)

	// Parse initiator's proposals.
	initiatorKVs := login.DecodeTextKV(raw2.DataSegment)
	initiatorMap := make(map[string]string)
	for _, kv := range initiatorKVs {
		initiatorMap[kv.Key] = kv.Value
	}

	// Build operational response -- echo back reasonable values.
	opKeys := []login.KeyValue{
		{Key: "HeaderDigest", Value: "None"},
		{Key: "DataDigest", Value: "None"},
		{Key: "MaxConnections", Value: "1"},
		{Key: "InitialR2T", Value: "Yes"},
		{Key: "ImmediateData", Value: "Yes"},
		{Key: "MaxRecvDataSegmentLength", Value: "8192"},
		{Key: "MaxBurstLength", Value: "262144"},
		{Key: "FirstBurstLength", Value: "65536"},
		{Key: "DefaultTime2Wait", Value: "0"},
		{Key: "DefaultTime2Retain", Value: "0"},
		{Key: "MaxOutstandingR2T", Value: "1"},
		{Key: "DataPDUInOrder", Value: "Yes"},
		{Key: "DataSequenceInOrder", Value: "Yes"},
		{Key: "ErrorRecoveryLevel", Value: "0"},
	}
	opData := login.EncodeTextKV(opKeys)

	opResp := &pdu.LoginResp{
		Header: pdu.Header{
			Final:          true,
			DataSegmentLen: uint32(len(opData)),
		},
		Transit:     true,
		CSG:         1,
		NSG:         3, // Full Feature Phase
		VersionMax:  0,
		VersionActive:  0,
		ISID:        req.ISID,
		TSIH:        1,
		StatSN:      statSN,
		ExpCmdSN:    req.CmdSN,
		MaxCmdSN:    req.CmdSN + 32,
		StatusClass: 0,
		Data:        opData,
	}

	bhs2, err := opResp.MarshalBHS()
	if err != nil {
		return fmt.Errorf("marshal op resp: %w", err)
	}
	rawResp2 := &transport.RawPDU{BHS: bhs2, DataSegment: opData}
	if err := transport.WriteRawPDU(conn, rawResp2); err != nil {
		return fmt.Errorf("write op resp: %w", err)
	}

	return nil
}

func (rt *recoverableTarget) handleCommands(conn net.Conn) {
	var statSN uint32 = 2 // after login
	for {
		raw, err := transport.ReadRawPDU(conn, false, false)
		if err != nil {
			return // connection closed or error
		}

		opcode := pdu.OpCode(raw.BHS[0] & 0x3f)
		itt := binary.BigEndian.Uint32(raw.BHS[16:20])

		switch opcode {
		case pdu.OpSCSICommand:
			cmd := &pdu.SCSICommand{}
			cmd.UnmarshalBHS(raw.BHS)

			if cmd.Read {
				// Send Data-In with status.
				data := rt.readData
				din := &pdu.DataIn{
					Header: pdu.Header{
						OpCode_:          pdu.OpDataIn,
						Final:            true,
						InitiatorTaskTag: itt,
						DataSegmentLen:   uint32(len(data)),
					},
					HasStatus: true,
					Status:    0x00,
					StatSN:    statSN,
					ExpCmdSN:  cmd.CmdSN + 1,
					MaxCmdSN:  cmd.CmdSN + 32,
					DataSN:    0,
					Data:      data,
				}
				bhs, err := din.MarshalBHS()
				if err != nil {
					rt.t.Logf("recoverableTarget: marshal DataIn: %v", err)
					return
				}
				if err := transport.WriteRawPDU(conn, &transport.RawPDU{BHS: bhs, DataSegment: data}); err != nil {
					return
				}
				statSN++
			} else {
				// Send SCSI Response.
				resp := &pdu.SCSIResponse{
					Header: pdu.Header{
						OpCode_:          pdu.OpSCSIResponse,
						Final:            true,
						InitiatorTaskTag: itt,
					},
					Status:   0x00,
					StatSN:   statSN,
					ExpCmdSN: cmd.CmdSN + 1,
					MaxCmdSN: cmd.CmdSN + 32,
				}
				bhs, err := resp.MarshalBHS()
				if err != nil {
					rt.t.Logf("recoverableTarget: marshal resp: %v", err)
					return
				}
				if err := transport.WriteRawPDU(conn, &transport.RawPDU{BHS: bhs}); err != nil {
					return
				}
				statSN++
			}

		case pdu.OpNOPOut:
			// Respond to keepalive NOP-Out with NOP-In.
			nopOut := &pdu.NOPOut{}
			nopOut.UnmarshalBHS(raw.BHS)

			if nopOut.InitiatorTaskTag == 0xFFFFFFFF {
				// Response to target-initiated ping, ignore.
				continue
			}

			nopIn := &pdu.NOPIn{
				Header: pdu.Header{
					OpCode_:          pdu.OpNOPIn,
					Final:            true,
					InitiatorTaskTag: nopOut.InitiatorTaskTag,
				},
				TargetTransferTag: 0xFFFFFFFF,
				StatSN:            statSN,
				ExpCmdSN:          nopOut.CmdSN,
				MaxCmdSN:          nopOut.CmdSN + 32,
			}
			bhs, err := nopIn.MarshalBHS()
			if err != nil {
				return
			}
			if err := transport.WriteRawPDU(conn, &transport.RawPDU{BHS: bhs}); err != nil {
				return
			}
			statSN++

		case pdu.OpLogoutReq:
			// Respond to logout.
			logoutResp := &pdu.LogoutResp{
				Header: pdu.Header{
					OpCode_:          pdu.OpLogoutResp,
					Final:            true,
					InitiatorTaskTag: itt,
				},
				Response: 0,
				StatSN:   statSN,
			}
			bhs, err := logoutResp.MarshalBHS()
			if err != nil {
				return
			}
			if err := transport.WriteRawPDU(conn, &transport.RawPDU{BHS: bhs}); err != nil {
				return
			}
			return

		default:
			rt.t.Logf("recoverableTarget: unhandled opcode 0x%02X", opcode)
		}
	}
}

// connectAndLogin dials the target and performs login, returning the
// transport.Conn and negotiated params.
func connectAndLogin(t *testing.T, addr string) (*transport.Conn, *login.NegotiatedParams) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tc, err := transport.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	params, err := login.Login(ctx, tc)
	if err != nil {
		tc.Close()
		t.Fatalf("login: %v", err)
	}

	return tc, params
}

// nonSeekableReader wraps a reader to strip io.Seeker interface.
type nonSeekableReader struct {
	r io.Reader
}

func (n *nonSeekableReader) Read(p []byte) (int, error) {
	return n.r.Read(p)
}

func TestERL0Reconnect(t *testing.T) {
	t.Run("read_command_completes", func(t *testing.T) {
		expectedData := []byte("recovered-data")
		target := startRecoverableTarget(t, expectedData)

		// Initial connection.
		tc, params := connectAndLogin(t, target.addr())

		sess := NewSession(tc, *params,
			WithReconnectInfo(target.addr()),
			WithReconnectBackoff(10*time.Millisecond),
			WithMaxReconnectAttempts(3),
			WithKeepaliveInterval(60*time.Second), // disable during test
		)
		t.Cleanup(func() { sess.Close() })

		// Submit a read command.
		cmd := Command{
			Read:                    true,
			ExpectedDataTransferLen: uint32(len(expectedData)),
		}
		cmd.CDB[0] = 0x28 // READ(10)

		resultCh, err := sess.Submit(context.Background(), cmd)
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}

		// Forcefully close the underlying connection to simulate drop.
		// This will cause the read pump to error and trigger reconnect.
		tc.NetConn().Close()

		// Wait for the result (should come after reconnect + retry).
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		select {
		case result := <-resultCh:
			if result.Err != nil {
				t.Fatalf("result error: %v", result.Err)
			}
			if result.Status != 0x00 {
				t.Fatalf("status: got 0x%02X, want 0x00", result.Status)
			}
			data, err := io.ReadAll(result.Data)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if string(data) != string(expectedData) {
				t.Fatalf("data: got %q, want %q", data, expectedData)
			}
		case <-ctx.Done():
			t.Fatal("timeout waiting for result after reconnect")
		}
	})

	t.Run("max_attempts_exhausted", func(t *testing.T) {
		expectedData := []byte("data")
		target := startRecoverableTarget(t, expectedData)

		// Initial connection.
		tc, params := connectAndLogin(t, target.addr())

		// Close the target listener so reconnect has nothing to connect to.
		target.ln.Close()

		sess := NewSession(tc, *params,
			WithReconnectInfo(target.addr()),
			WithReconnectBackoff(10*time.Millisecond),
			WithMaxReconnectAttempts(1),
			WithKeepaliveInterval(60*time.Second),
		)
		t.Cleanup(func() { sess.Close() })

		// Submit a command.
		cmd := Command{
			Read:                    true,
			ExpectedDataTransferLen: 4,
		}
		cmd.CDB[0] = 0x28

		resultCh, err := sess.Submit(context.Background(), cmd)
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}

		// Force connection drop.
		tc.NetConn().Close()

		// Wait for result -- should fail.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		select {
		case result := <-resultCh:
			if result.Err == nil {
				t.Fatal("expected error, got success")
			}
			if !strings.Contains(result.Err.Error(), "reconnect failed") {
				t.Fatalf("unexpected error: %v", result.Err)
			}
		case <-ctx.Done():
			t.Fatal("timeout waiting for failed result")
		}

		// Session.Err() should be set.
		if sessErr := sess.Err(); sessErr == nil {
			t.Fatal("expected session error after reconnect failure")
		}
	})

	t.Run("write_seekable_retry", func(t *testing.T) {
		target := startRecoverableTarget(t, nil)

		tc, params := connectAndLogin(t, target.addr())

		sess := NewSession(tc, *params,
			WithReconnectInfo(target.addr()),
			WithReconnectBackoff(10*time.Millisecond),
			WithMaxReconnectAttempts(3),
			WithKeepaliveInterval(60*time.Second),
		)
		t.Cleanup(func() { sess.Close() })

		// Submit write command with seekable reader (bytes.NewReader).
		writeData := []byte("write-me")
		cmd := Command{
			ExpectedDataTransferLen: uint32(len(writeData)),
			Data:                    bytes.NewReader(writeData),
		}
		cmd.CDB[0] = 0x2A // WRITE(10)

		resultCh, err := sess.Submit(context.Background(), cmd)
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}

		// Force connection drop.
		tc.NetConn().Close()

		// Wait for result after reconnect.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		select {
		case result := <-resultCh:
			if result.Err != nil {
				t.Fatalf("result error: %v", result.Err)
			}
			if result.Status != 0x00 {
				t.Fatalf("status: got 0x%02X, want 0x00", result.Status)
			}
		case <-ctx.Done():
			t.Fatal("timeout waiting for write result after reconnect")
		}
	})

	t.Run("write_nonseeakble_fails", func(t *testing.T) {
		target := startRecoverableTarget(t, nil)

		tc, params := connectAndLogin(t, target.addr())

		sess := NewSession(tc, *params,
			WithReconnectInfo(target.addr()),
			WithReconnectBackoff(10*time.Millisecond),
			WithMaxReconnectAttempts(3),
			WithKeepaliveInterval(60*time.Second),
		)
		t.Cleanup(func() { sess.Close() })

		// Submit write command with non-seekable reader.
		writeData := []byte("write-me")
		cmd := Command{
			ExpectedDataTransferLen: uint32(len(writeData)),
			Data:                    &nonSeekableReader{r: bytes.NewReader(writeData)},
		}
		cmd.CDB[0] = 0x2A

		resultCh, err := sess.Submit(context.Background(), cmd)
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}

		// Force connection drop.
		tc.NetConn().Close()

		// Wait for result -- should fail with ErrRetryNotPossible.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		select {
		case result := <-resultCh:
			if result.Err == nil {
				t.Fatal("expected error, got success")
			}
			if !errors.Is(result.Err, ErrRetryNotPossible) {
				t.Fatalf("expected ErrRetryNotPossible, got: %v", result.Err)
			}
		case <-ctx.Done():
			t.Fatal("timeout waiting for non-seekable write result")
		}
	})

	t.Run("submit_during_recovery", func(t *testing.T) {
		// Use a target on a port that will refuse reconnect so recovery
		// takes time (backoff loops). The initial connection uses a real target.
		target := startRecoverableTarget(t, []byte("data"))

		tc, params := connectAndLogin(t, target.addr())

		// Close the target listener so reconnect dials will fail slowly.
		target.ln.Close()

		sess := NewSession(tc, *params,
			WithReconnectInfo(target.addr()),
			WithReconnectBackoff(200*time.Millisecond), // slow enough to ensure recovery is in progress when we check
			WithMaxReconnectAttempts(3),
			WithKeepaliveInterval(60*time.Second),
		)
		t.Cleanup(func() { sess.Close() })

		// Submit a command to have an in-flight task.
		cmd := Command{
			Read:                    true,
			ExpectedDataTransferLen: 4,
		}
		cmd.CDB[0] = 0x28

		_, err := sess.Submit(context.Background(), cmd)
		if err != nil {
			t.Fatalf("Submit: %v", err)
		}

		// Force connection drop to trigger recovery.
		tc.NetConn().Close()

		// Give recovery a moment to start (first dial attempt + first backoff).
		time.Sleep(100 * time.Millisecond)

		// Try to submit during recovery -- should fail.
		cmd2 := Command{CDB: [16]byte{0x00}}
		_, err = sess.Submit(context.Background(), cmd2)
		if err == nil {
			t.Fatal("expected error during recovery")
		}
		if !errors.Is(err, ErrSessionRecovering) {
			t.Fatalf("expected ErrSessionRecovering, got: %v", err)
		}
	})
}
