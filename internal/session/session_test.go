package session

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/uiscsi/uiscsi/internal/digest"
	"github.com/uiscsi/uiscsi/internal/login"
	"github.com/uiscsi/uiscsi/internal/pdu"
	"github.com/uiscsi/uiscsi/internal/transport"
)

// captureHandler is a slog.Handler that records all log entries for test assertions.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, r)
	h.mu.Unlock()
	return nil
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler       { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler             { return h }

// hasMessage checks if any captured record has the given message substring.
func (h *captureHandler) hasMessage(msg string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if containsStr(r.Message, msg) {
			return true
		}
	}
	return false
}

// hasLevelMessage checks if any captured record has the given level and message substring.
func (h *captureHandler) hasLevelMessage(level slog.Level, msg string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if r.Level == level && containsStr(r.Message, msg) {
			return true
		}
	}
	return false
}

// containsStr is a simple substring check.
func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// newTestSession creates a Session backed by a net.Pipe for testing.
// Returns the session and the "target" side of the pipe for sending responses.
func newTestSession(t *testing.T) (*Session, net.Conn) {
	t.Helper()
	clientConn, targetConn := net.Pipe()

	tc := transport.NewConnFromNetConn(clientConn)
	params := login.Defaults()
	params.CmdSN = 1
	params.ExpStatSN = 1

	sess := NewSession(tc, params)
	t.Cleanup(func() {
		go respondToLogout(targetConn)
		sess.Close()
		targetConn.Close()
	})

	return sess, targetConn
}

// writeDataInPDU encodes and writes a Data-In PDU to the target conn.
func writeDataInPDU(t *testing.T, conn net.Conn, din *pdu.DataIn) {
	t.Helper()
	din.Header.OpCode_ = pdu.OpDataIn
	din.Header.Final = true
	din.Header.DataSegmentLen = uint32(len(din.Data))
	raw := buildRawPDU(t, din)
	if err := transport.WriteRawPDU(conn, raw); err != nil {
		t.Fatalf("write DataIn: %v", err)
	}
}

// writeSCSIResponsePDU encodes and writes a SCSIResponse PDU to the target conn.
func writeSCSIResponsePDU(t *testing.T, conn net.Conn, resp *pdu.SCSIResponse) {
	t.Helper()
	resp.Header.OpCode_ = pdu.OpSCSIResponse
	resp.Header.Final = true
	resp.Header.DataSegmentLen = uint32(len(resp.Data))
	raw := buildRawPDU(t, resp)
	if err := transport.WriteRawPDU(conn, raw); err != nil {
		t.Fatalf("write SCSIResponse: %v", err)
	}
}

// buildRawPDU marshals a PDU into a RawPDU for wire transmission.
func buildRawPDU(t *testing.T, p pdu.PDU) *transport.RawPDU {
	t.Helper()
	bhs, err := p.MarshalBHS()
	if err != nil {
		t.Fatalf("MarshalBHS: %v", err)
	}
	raw := &transport.RawPDU{BHS: bhs}
	if ds := p.DataSegment(); len(ds) > 0 {
		raw.DataSegment = ds
	}
	return raw
}

// respondToLogout reads PDUs from the target conn and auto-responds to a
// Logout request with a successful LogoutResp. This lets Close() complete
// a real graceful logout instead of waiting 5s for a response that never
// comes. Any non-Logout PDUs (e.g., NOP-Out keepalives) are silently
// consumed. Returns when the conn is closed or after responding to Logout.
func respondToLogout(conn net.Conn) {
	for {
		raw, err := transport.ReadRawPDU(conn, false, false, 0)
		if err != nil {
			return
		}
		opcode := pdu.OpCode(raw.BHS[0] & 0x3f)
		if opcode != pdu.OpLogoutReq {
			continue
		}
		itt := binary.BigEndian.Uint32(raw.BHS[16:20])
		resp := &pdu.LogoutResp{
			Header: pdu.Header{
				OpCode_:          pdu.OpLogoutResp,
				Final:            true,
				InitiatorTaskTag: itt,
			},
			Response: 0,
		}
		bhs, err := resp.MarshalBHS()
		if err != nil {
			return
		}
		_ = transport.WriteRawPDU(conn, &transport.RawPDU{BHS: bhs})
		return
	}
}

// readSCSICommandPDU reads and decodes a SCSICommand PDU from the target conn.
func readSCSICommandPDU(t *testing.T, conn net.Conn) *pdu.SCSICommand {
	t.Helper()
	raw, err := transport.ReadRawPDU(conn, false, false, 0)
	if err != nil {
		t.Fatalf("read SCSICommand: %v", err)
	}
	cmd := &pdu.SCSICommand{}
	cmd.UnmarshalBHS(raw.BHS)
	cmd.ImmediateData = raw.DataSegment
	return cmd
}

func TestSessionSubmitRead(t *testing.T) {
	sess, targetConn := newTestSession(t)

	cmd := Command{
		Read:                    true,
		ExpectedDataTransferLen: 5,
	}
	cmd.CDB[0] = 0x28 // READ(10)

	resultCh, err := sess.Submit(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Read the command PDU from target side.
	scsiCmd := readSCSICommandPDU(t, targetConn)
	if scsiCmd.CDB[0] != 0x28 {
		t.Fatalf("CDB[0]: got 0x%02X, want 0x28", scsiCmd.CDB[0])
	}

	// Send Data-In with S=1 (status).
	writeDataInPDU(t, targetConn, &pdu.DataIn{
		Header:    pdu.Header{InitiatorTaskTag: scsiCmd.InitiatorTaskTag},
		HasStatus: true,
		Status:    0x00,
		StatSN:    1,
		ExpCmdSN:  1,
		MaxCmdSN:  10,
		DataSN:    0,
		Data:      []byte("hello"),
	})

	result := <-resultCh
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
	if string(data) != "hello" {
		t.Fatalf("data: got %q, want %q", data, "hello")
	}
}

func TestSessionSubmitMultiPDURead(t *testing.T) {
	sess, targetConn := newTestSession(t)

	cmd := Command{
		Read:                    true,
		ExpectedDataTransferLen: 15,
	}
	cmd.CDB[0] = 0x28

	resultCh, err := sess.Submit(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	scsiCmd := readSCSICommandPDU(t, targetConn)
	itt := scsiCmd.InitiatorTaskTag

	// 3 Data-In PDUs without status.
	chunks := []string{"chunk", "1chun", "k2end"}
	offset := uint32(0)
	for i, chunk := range chunks {
		writeDataInPDU(t, targetConn, &pdu.DataIn{
			Header:       pdu.Header{InitiatorTaskTag: itt},
			DataSN:       uint32(i),
			BufferOffset: offset,
			StatSN:       uint32(1 + i),
			ExpCmdSN:     1,
			MaxCmdSN:     10,
			Data:         []byte(chunk),
		})
		offset += uint32(len(chunk))
	}

	// Final SCSIResponse.
	writeSCSIResponsePDU(t, targetConn, &pdu.SCSIResponse{
		Header:   pdu.Header{InitiatorTaskTag: itt},
		Status:   0x00,
		StatSN:   4,
		ExpCmdSN: 2,
		MaxCmdSN: 10,
	})

	result := <-resultCh
	if result.Err != nil {
		t.Fatalf("result error: %v", result.Err)
	}
	data, err := io.ReadAll(result.Data)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	want := "chunk1chunk2end"
	if string(data) != want {
		t.Fatalf("data: got %q, want %q", data, want)
	}
}

func TestSessionSubmitNonRead(t *testing.T) {
	sess, targetConn := newTestSession(t)

	cmd := Command{}
	cmd.CDB[0] = 0x00 // TEST UNIT READY

	resultCh, err := sess.Submit(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	scsiCmd := readSCSICommandPDU(t, targetConn)

	writeSCSIResponsePDU(t, targetConn, &pdu.SCSIResponse{
		Header:   pdu.Header{InitiatorTaskTag: scsiCmd.InitiatorTaskTag},
		Status:   0x00,
		StatSN:   1,
		ExpCmdSN: 2,
		MaxCmdSN: 10,
	})

	result := <-resultCh
	if result.Err != nil {
		t.Fatalf("result error: %v", result.Err)
	}
	if result.Data != nil {
		t.Fatal("Data should be nil for non-read command")
	}
	if result.Status != 0x00 {
		t.Fatalf("status: got 0x%02X, want 0x00", result.Status)
	}
}

func TestSessionConcurrentSubmit(t *testing.T) {
	sess, targetConn := newTestSession(t)

	const n = 3

	// Submit n commands concurrently. Each submit may block on CmdSN window.
	type cmdResult struct {
		resultCh <-chan Result
		idx      int
	}
	resultsCh := make(chan cmdResult, n)

	for i := range n {
		go func(idx int) {
			cmd := Command{CDB: [16]byte{byte(idx)}}
			ch, err := sess.Submit(context.Background(), cmd)
			if err != nil {
				t.Errorf("Submit(%d): %v", idx, err)
				return
			}
			resultsCh <- cmdResult{resultCh: ch, idx: idx}
		}(i)
	}

	// Process commands one at a time from the target side.
	// Each response advances the CmdSN window, allowing the next submit to proceed.
	var allResults []cmdResult
	for i := range n {
		raw, err := transport.ReadRawPDU(targetConn, false, false, 0)
		if err != nil {
			t.Fatalf("read command %d: %v", i, err)
		}
		itt := binary.BigEndian.Uint32(raw.BHS[16:20])

		// Advance MaxCmdSN to allow next command.
		writeSCSIResponsePDU(t, targetConn, &pdu.SCSIResponse{
			Header:   pdu.Header{InitiatorTaskTag: itt},
			Status:   0x00,
			StatSN:   uint32(1 + i),
			ExpCmdSN: uint32(2 + i),
			MaxCmdSN: uint32(2 + i),
		})

		// Collect the result channel.
		cr := <-resultsCh
		allResults = append(allResults, cr)
	}

	// Verify all results.
	for _, cr := range allResults {
		result := <-cr.resultCh
		if result.Err != nil {
			t.Errorf("result %d error: %v", cr.idx, result.Err)
		}
	}
}

func TestSessionParams(t *testing.T) {
	sess, _ := newTestSession(t)

	params := sess.Params()
	if params.CmdSN != 1 {
		t.Fatalf("CmdSN: got %d, want 1", params.CmdSN)
	}
	if params.MaxRecvDataSegmentLength != 8192 {
		t.Fatalf("MaxRecvDSL: got %d, want 8192", params.MaxRecvDataSegmentLength)
	}
}

func TestSessionStatSNTracking(t *testing.T) {
	sess, targetConn := newTestSession(t)

	// Initial expStatSN should be 1 (from params).
	if got := sess.getExpStatSN(); got != 1 {
		t.Fatalf("initial expStatSN: got %d, want 1", got)
	}

	// Submit a command and have target respond with StatSN=1.
	cmd := Command{}
	cmd.CDB[0] = 0x00
	resultCh, err := sess.Submit(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	scsiCmd := readSCSICommandPDU(t, targetConn)

	writeSCSIResponsePDU(t, targetConn, &pdu.SCSIResponse{
		Header:   pdu.Header{InitiatorTaskTag: scsiCmd.InitiatorTaskTag},
		Status:   0x00,
		StatSN:   1,
		ExpCmdSN: 2,
		MaxCmdSN: 10,
	})

	<-resultCh

	// After response with StatSN=1, expStatSN should be 2.
	if got := sess.getExpStatSN(); got != 2 {
		t.Fatalf("expStatSN after response: got %d, want 2", got)
	}
}

func TestSessionSubmitWriteImmediateData(t *testing.T) {
	sess, targetConn := newTestSession(t)
	// Default params: ImmediateData=true, FirstBurstLength=65536, MaxRecvDSL=8192

	writeData := []byte("hello write")
	cmd := Command{
		ExpectedDataTransferLen: uint32(len(writeData)),
		Data:                    bytes.NewReader(writeData),
	}
	cmd.CDB[0] = 0x2A // WRITE(10)

	resultCh, err := sess.Submit(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Read the command from target side -- should have immediate data.
	scsiCmd := readSCSICommandPDU(t, targetConn)
	if !scsiCmd.Write {
		t.Fatal("W-bit not set on write command")
	}
	if string(scsiCmd.ImmediateData) != "hello write" {
		t.Fatalf("immediate data: got %q, want %q", scsiCmd.ImmediateData, "hello write")
	}

	// Send SCSI Response to complete the command.
	writeSCSIResponsePDU(t, targetConn, &pdu.SCSIResponse{
		Header:   pdu.Header{InitiatorTaskTag: scsiCmd.InitiatorTaskTag},
		Status:   0x00,
		StatSN:   1,
		ExpCmdSN: 2,
		MaxCmdSN: 10,
	})

	result := <-resultCh
	if result.Err != nil {
		t.Fatalf("result error: %v", result.Err)
	}
	if result.Status != 0x00 {
		t.Fatalf("status: got 0x%02X, want 0x00", result.Status)
	}
	if result.Data != nil {
		t.Fatal("Data should be nil for write command result")
	}
}

func TestDigestErrorConnectionFatal(t *testing.T) {
	clientConn, targetConn := net.Pipe()

	// Create transport.Conn with header digest enabled so ReadPump expects
	// a 4-byte header digest after BHS.
	tc := transport.NewConnFromNetConn(clientConn)
	tc.SetDigests(true, false) // header digest on, data digest off

	handler := &captureHandler{}
	logger := slog.New(handler)

	params := login.Defaults()
	params.CmdSN = 1
	params.ExpStatSN = 1

	// Create session WITH reconnect info to verify DigestError does NOT reconnect.
	sess := NewSession(tc, params,
		WithLogger(logger),
		WithReconnectInfo("127.0.0.1:9999"), // dummy addr
	)

	// Write a BHS with a wrong header digest from the target side.
	// This will cause ReadPump to get a DigestError.
	go func() {
		// Build a NOP-In PDU BHS (48 bytes) + wrong 4-byte header digest.
		var bhs [pdu.BHSLength]byte
		bhs[0] = byte(pdu.OpNOPIn) | 0x40 // opcode + reserved bit
		bhs[1] = 0x80                      // Final=1
		// ITT = 0xFFFFFFFF (unsolicited)
		binary.BigEndian.PutUint32(bhs[16:20], 0xFFFFFFFF)
		// DataSegmentLength = 0 (no data segment, no data digest)

		// Write BHS
		targetConn.Write(bhs[:])
		// Write wrong header digest (all zeros, will not match CRC32C of BHS)
		targetConn.Write([]byte{0x00, 0x00, 0x00, 0x00})
	}()

	// Wait for the session to detect the error.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for session to detect digest error")
		default:
		}
		if err := sess.Err(); err != nil {
			// Verify the error wraps DigestError.
			var de *digest.DigestError
			if !errors.As(err, &de) {
				t.Fatalf("session error should wrap *digest.DigestError, got: %v", err)
			}
			if de.Type != digest.DigestHeader {
				t.Fatalf("digest type: got %v, want DigestHeader", de.Type)
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Verify the Warn log was emitted.
	if !handler.hasLevelMessage(slog.LevelWarn, "session: digest mismatch") {
		t.Error("expected Warn log with 'session: digest mismatch'")
	}

	// Verify no reconnect was attempted (session should NOT have logged reconnect start).
	if handler.hasMessage("session: reconnect started") {
		t.Error("DigestError should not trigger reconnect (connection-fatal per D-03)")
	}

	// Cleanup.
	sess.Close()
	targetConn.Close()
}

func TestSessionLifecycleLogging(t *testing.T) {
	clientConn, targetConn := net.Pipe()

	tc := transport.NewConnFromNetConn(clientConn)
	handler := &captureHandler{}
	logger := slog.New(handler)

	params := login.Defaults()
	params.CmdSN = 1
	params.ExpStatSN = 1

	sess := NewSession(tc, params, WithLogger(logger))

	// Verify "session: opened" was logged.
	if !handler.hasLevelMessage(slog.LevelInfo, "session: opened") {
		t.Error("expected Info log with 'session: opened' after NewSession")
	}

	// Close session.
	go respondToLogout(targetConn)
	sess.Close()
	targetConn.Close()

	// Verify "session: closing" was logged.
	if !handler.hasLevelMessage(slog.LevelInfo, "session: closing") {
		t.Error("expected Info log with 'session: closing' after Close")
	}
}

func TestSCSIResponseSenseDataExtraction(t *testing.T) {
	// Build fixed-format sense data: response code 0x70, SenseKey 0x05
	// (ILLEGAL REQUEST), ASC 0x21 (LBA out of range), ASCQ 0x00.
	senseBytes := make([]byte, 18)
	senseBytes[0] = 0x70  // response code: current errors, fixed format
	senseBytes[2] = 0x05  // sense key: ILLEGAL REQUEST
	senseBytes[7] = 10    // additional sense length (18 - 8 = 10)
	senseBytes[12] = 0x21 // ASC: LBA out of range
	senseBytes[13] = 0x00 // ASCQ

	// Build the SCSI Response data segment per RFC 7143 Section 11.4.7.2:
	// [SenseLength (2 bytes, big-endian)] [Sense Data (SenseLength bytes)]
	dataSegment := make([]byte, 2+len(senseBytes))
	binary.BigEndian.PutUint16(dataSegment[0:2], uint16(len(senseBytes)))
	copy(dataSegment[2:], senseBytes)

	resp := &pdu.SCSIResponse{
		Status: 0x02, // CHECK CONDITION
		Data:   dataSegment,
	}

	tk := newTask(1, false, false, 0)
	tk.handleSCSIResponse(resp)

	result := <-tk.resultCh
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.Status != 0x02 {
		t.Fatalf("expected status 0x02, got 0x%02X", result.Status)
	}
	if len(result.SenseData) != 18 {
		t.Fatalf("expected 18 sense bytes, got %d", len(result.SenseData))
	}
	// Verify the SenseLength prefix was stripped: first byte should be
	// the response code (0x70), not the length MSB (0x00).
	if result.SenseData[0] != 0x70 {
		t.Errorf("expected response code 0x70 at SenseData[0], got 0x%02X", result.SenseData[0])
	}
	if result.SenseData[2] != 0x05 {
		t.Errorf("expected sense key 0x05 at SenseData[2], got 0x%02X", result.SenseData[2])
	}
	if result.SenseData[12] != 0x21 {
		t.Errorf("expected ASC 0x21 at SenseData[12], got 0x%02X", result.SenseData[12])
	}
}

func TestSCSIResponseSenseDataEmpty(t *testing.T) {
	// When data segment is nil or too short, SenseData should be nil.
	tests := []struct {
		name string
		data []byte
		wantLen int
	}{
		{"nil data", nil, 0},
		{"empty data", []byte{}, 0},
		{"one byte", []byte{0x00}, 0},
		{"zero length", []byte{0x00, 0x00}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &pdu.SCSIResponse{
				Status: 0x00,
				Data:   tt.data,
			}
			tk := newTask(1, false, false, 0)
			tk.handleSCSIResponse(resp)

			result := <-tk.resultCh
			if len(result.SenseData) != tt.wantLen {
				t.Errorf("expected SenseData len %d, got %d", tt.wantLen, len(result.SenseData))
			}
		})
	}
}

func TestSessionSubmitStreamingRead(t *testing.T) {
	sess, targetConn := newTestSession(t)

	cmd := Command{
		Read:                    true,
		ExpectedDataTransferLen: 24,
	}
	cmd.CDB[0] = 0x08 // READ(6) — tape-style

	resultCh, dataReader, err := sess.SubmitStreaming(context.Background(), cmd)
	if err != nil {
		t.Fatalf("SubmitStreaming: %v", err)
	}
	if dataReader == nil {
		t.Fatal("dataReader is nil for read command")
	}

	scsiCmd := readSCSICommandPDU(t, targetConn)

	// Read from dataReader concurrently — required because the chanReader
	// will backpressure if the caller isn't consuming.
	done := make(chan []byte, 1)
	go func() {
		got, _ := io.ReadAll(dataReader)
		done <- got
	}()

	// Send 3 Data-In PDUs without status.
	chunks := [][]byte{
		[]byte("AAAABBBB"),
		[]byte("CCCCDDDD"),
		[]byte("EEEEFFFF"),
	}
	offset := uint32(0)
	for i, chunk := range chunks {
		writeDataInPDU(t, targetConn, &pdu.DataIn{
			Header:       pdu.Header{InitiatorTaskTag: scsiCmd.InitiatorTaskTag},
			DataSN:       uint32(i),
			BufferOffset: offset,
			ExpCmdSN:     1,
			MaxCmdSN:     10,
			Data:         chunk,
		})
		offset += uint32(len(chunk))
	}

	// Send SCSIResponse with status.
	writeSCSIResponsePDU(t, targetConn, &pdu.SCSIResponse{
		Header: pdu.Header{InitiatorTaskTag: scsiCmd.InitiatorTaskTag},
		Status: 0x00,
		StatSN: 1,
	})

	got := <-done
	want := "AAAABBBBCCCCDDDDEEEEFFFF"
	if string(got) != want {
		t.Fatalf("data: got %q, want %q", got, want)
	}

	result := <-resultCh
	if result.Err != nil {
		t.Fatalf("result error: %v", result.Err)
	}
	if result.Status != 0x00 {
		t.Fatalf("status: got 0x%02X, want 0x00", result.Status)
	}
	if result.Data != nil {
		t.Fatal("streaming result should have nil Data")
	}
}
