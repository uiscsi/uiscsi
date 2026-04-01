package session

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"

	"github.com/rkujawa/uiscsi/internal/login"
	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
)

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
		raw, err := transport.ReadRawPDU(conn, false, false)
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
	raw, err := transport.ReadRawPDU(conn, false, false)
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
		raw, err := transport.ReadRawPDU(targetConn, false, false)
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
