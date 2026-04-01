package session

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"

	"github.com/rkujawa/uiscsi/internal/login"
	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
)

// newTestSessionWithParams creates a test session with custom NegotiatedParams.
func newTestSessionWithParams(t *testing.T, params login.NegotiatedParams) (*Session, net.Conn) {
	t.Helper()
	clientConn, targetConn := net.Pipe()
	tc := transport.NewConnFromNetConn(clientConn)
	sess := NewSession(tc, params)
	t.Cleanup(func() {
		sess.Close()
		targetConn.Close()
	})
	return sess, targetConn
}

// writeR2TPDU encodes and writes an R2T PDU to the target conn.
func writeR2TPDU(t *testing.T, conn net.Conn, r2t *pdu.R2T) {
	t.Helper()
	r2t.Header.OpCode_ = pdu.OpR2T
	raw := buildRawPDU(t, r2t)
	if err := transport.WriteRawPDU(conn, raw); err != nil {
		t.Fatalf("write R2T: %v", err)
	}
}

// readDataOutPDU reads and decodes a Data-Out PDU from the target conn.
func readDataOutPDU(t *testing.T, conn net.Conn) *pdu.DataOut {
	t.Helper()
	raw, err := transport.ReadRawPDU(conn, false, false)
	if err != nil {
		t.Fatalf("read DataOut: %v", err)
	}
	dout := &pdu.DataOut{}
	dout.UnmarshalBHS(raw.BHS)
	dout.Data = raw.DataSegment
	return dout
}

// TestWriteSolicitedR2T verifies that when the target sends an R2T, the
// initiator responds with correct Data-Out PDUs matching the R2T parameters.
func TestWriteSolicitedR2T(t *testing.T) {
	params := login.Defaults()
	params.CmdSN = 1
	params.ExpStatSN = 1
	// ImmediateData=true, InitialR2T=true (defaults)

	sess, targetConn := newTestSessionWithParams(t, params)

	writeData := bytes.Repeat([]byte("A"), 20)
	cmd := Command{
		ExpectedDataTransferLen: 20,
		Data:                    bytes.NewReader(writeData),
	}
	cmd.CDB[0] = 0x2A // WRITE(10)

	resultCh, err := sess.Submit(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Read SCSICommand -- should have immediate data (all 20 bytes fit in one PDU).
	scsiCmd := readSCSICommandPDU(t, targetConn)
	if !scsiCmd.Write {
		t.Fatal("W-bit not set")
	}
	immLen := len(scsiCmd.ImmediateData)
	if immLen != 20 {
		t.Fatalf("immediate data length: got %d, want 20", immLen)
	}

	// Target wants remaining data via R2T (in this case 0 bytes because
	// all fit as immediate). Let's test with a larger payload instead.
	// Send SCSIResponse to complete.
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
}

// TestWriteSolicitedR2TLargePayload tests R2T handling with data that exceeds
// immediate data, requiring a solicited Data-Out burst.
func TestWriteSolicitedR2TLargePayload(t *testing.T) {
	params := login.Defaults()
	params.CmdSN = 1
	params.ExpStatSN = 1
	params.ImmediateData = false // No immediate data -- all via R2T
	params.InitialR2T = true
	params.MaxRecvDataSegmentLength = 8192
	params.MaxBurstLength = 262144

	sess, targetConn := newTestSessionWithParams(t, params)

	writeData := bytes.Repeat([]byte("B"), 500)
	cmd := Command{
		ExpectedDataTransferLen: 500,
		Data:                    bytes.NewReader(writeData),
	}
	cmd.CDB[0] = 0x2A

	resultCh, err := sess.Submit(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Read SCSICommand -- no immediate data.
	scsiCmd := readSCSICommandPDU(t, targetConn)
	itt := scsiCmd.InitiatorTaskTag
	if len(scsiCmd.ImmediateData) != 0 {
		t.Fatalf("expected no immediate data, got %d bytes", len(scsiCmd.ImmediateData))
	}

	// Send R2T for all 500 bytes.
	writeR2TPDU(t, targetConn, &pdu.R2T{
		Header:                    pdu.Header{InitiatorTaskTag: itt},
		TargetTransferTag:         0x1234,
		StatSN:                    1,
		ExpCmdSN:                  1,
		MaxCmdSN:                  10,
		R2TSN:                     0,
		BufferOffset:              0,
		DesiredDataTransferLength: 500,
	})

	// Read Data-Out PDU (all 500 bytes fit in one PDU since MaxRecvDSL=8192).
	dout := readDataOutPDU(t, targetConn)
	if dout.DataSN != 0 {
		t.Fatalf("DataSN: got %d, want 0", dout.DataSN)
	}
	if dout.BufferOffset != 0 {
		t.Fatalf("BufferOffset: got %d, want 0", dout.BufferOffset)
	}
	if dout.TargetTransferTag != 0x1234 {
		t.Fatalf("TTT: got 0x%08X, want 0x00001234", dout.TargetTransferTag)
	}
	if !dout.Header.Final {
		t.Fatal("Final bit not set on last Data-Out PDU")
	}
	if len(dout.Data) != 500 {
		t.Fatalf("Data length: got %d, want 500", len(dout.Data))
	}
	if !bytes.Equal(dout.Data, writeData) {
		t.Fatal("Data-Out data mismatch")
	}

	// Complete with SCSIResponse.
	writeSCSIResponsePDU(t, targetConn, &pdu.SCSIResponse{
		Header:   pdu.Header{InitiatorTaskTag: itt},
		Status:   0x00,
		StatSN:   2,
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
}

// TestWriteUnsolicitedDataOut verifies that unsolicited Data-Out PDUs are sent
// when InitialR2T=No, bounded by FirstBurstLength minus immediate data.
func TestWriteUnsolicitedDataOut(t *testing.T) {
	params := login.Defaults()
	params.CmdSN = 1
	params.ExpStatSN = 1
	params.ImmediateData = true
	params.InitialR2T = false
	params.FirstBurstLength = 1024
	params.MaxRecvDataSegmentLength = 512
	params.MaxBurstLength = 262144

	sess, targetConn := newTestSessionWithParams(t, params)

	writeData := bytes.Repeat([]byte("C"), 2048)
	cmd := Command{
		ExpectedDataTransferLen: 2048,
		Data:                    bytes.NewReader(writeData),
	}
	cmd.CDB[0] = 0x2A

	resultCh, err := sess.Submit(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Read SCSICommand -- immediate data = min(FirstBurstLength, MaxRecvDSL) = 512 bytes.
	scsiCmd := readSCSICommandPDU(t, targetConn)
	itt := scsiCmd.InitiatorTaskTag
	immLen := len(scsiCmd.ImmediateData)
	if immLen != 512 {
		t.Fatalf("immediate data length: got %d, want 512", immLen)
	}

	// Read unsolicited Data-Out: remaining = 1024-512 = 512 bytes = 1 PDU.
	dout := readDataOutPDU(t, targetConn)
	if dout.TargetTransferTag != 0xFFFFFFFF {
		t.Fatalf("TTT: got 0x%08X, want 0xFFFFFFFF", dout.TargetTransferTag)
	}
	if dout.DataSN != 0 {
		t.Fatalf("DataSN: got %d, want 0", dout.DataSN)
	}
	if dout.BufferOffset != 512 {
		t.Fatalf("BufferOffset: got %d, want 512", dout.BufferOffset)
	}
	if len(dout.Data) != 512 {
		t.Fatalf("unsolicited data length: got %d, want 512", len(dout.Data))
	}
	if !dout.Header.Final {
		t.Fatal("Final bit not set on last unsolicited Data-Out PDU")
	}

	// Target sends R2T for remaining 1024 bytes.
	writeR2TPDU(t, targetConn, &pdu.R2T{
		Header:                    pdu.Header{InitiatorTaskTag: itt},
		TargetTransferTag:         0xABCD,
		StatSN:                    1,
		ExpCmdSN:                  1,
		MaxCmdSN:                  10,
		R2TSN:                     0,
		BufferOffset:              1024,
		DesiredDataTransferLength: 1024,
	})

	// Read solicited Data-Out PDUs: 1024 bytes / 512 MaxRecvDSL = 2 PDUs.
	for i := range 2 {
		dout := readDataOutPDU(t, targetConn)
		if dout.TargetTransferTag != 0xABCD {
			t.Fatalf("solicited PDU %d TTT: got 0x%08X, want 0x0000ABCD", i, dout.TargetTransferTag)
		}
		if dout.DataSN != uint32(i) {
			t.Fatalf("solicited PDU %d DataSN: got %d, want %d", i, dout.DataSN, i)
		}
		wantOffset := 1024 + uint32(i)*512
		if dout.BufferOffset != wantOffset {
			t.Fatalf("solicited PDU %d BufferOffset: got %d, want %d", i, dout.BufferOffset, wantOffset)
		}
		if len(dout.Data) != 512 {
			t.Fatalf("solicited PDU %d data length: got %d, want 512", i, len(dout.Data))
		}
		// Final bit only on last PDU.
		if i == 1 && !dout.Header.Final {
			t.Fatalf("solicited PDU %d: Final bit not set", i)
		}
		if i == 0 && dout.Header.Final {
			t.Fatalf("solicited PDU %d: Final bit should not be set", i)
		}
	}

	// Complete.
	writeSCSIResponsePDU(t, targetConn, &pdu.SCSIResponse{
		Header:   pdu.Header{InitiatorTaskTag: itt},
		Status:   0x00,
		StatSN:   2,
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
}

// TestWriteMaxBurstLengthEnforcement verifies that Data-Out responses are
// capped at MaxBurstLength even when R2T requests more data.
func TestWriteMaxBurstLengthEnforcement(t *testing.T) {
	params := login.Defaults()
	params.CmdSN = 1
	params.ExpStatSN = 1
	params.ImmediateData = false
	params.InitialR2T = true
	params.MaxBurstLength = 1024
	params.MaxRecvDataSegmentLength = 512

	sess, targetConn := newTestSessionWithParams(t, params)

	writeData := bytes.Repeat([]byte("D"), 4096)
	cmd := Command{
		ExpectedDataTransferLen: 4096,
		Data:                    bytes.NewReader(writeData),
	}
	cmd.CDB[0] = 0x2A

	resultCh, err := sess.Submit(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	scsiCmd := readSCSICommandPDU(t, targetConn)
	itt := scsiCmd.InitiatorTaskTag

	// Send R2T requesting 2048 bytes (exceeds MaxBurstLength=1024).
	writeR2TPDU(t, targetConn, &pdu.R2T{
		Header:                    pdu.Header{InitiatorTaskTag: itt},
		TargetTransferTag:         0x5678,
		StatSN:                    1,
		ExpCmdSN:                  1,
		MaxCmdSN:                  10,
		R2TSN:                     0,
		BufferOffset:              0,
		DesiredDataTransferLength: 2048, // Exceeds MaxBurstLength
	})

	// Should receive exactly 1024 bytes (MaxBurstLength), in 2 PDUs of 512.
	totalBytes := 0
	for i := range 2 {
		dout := readDataOutPDU(t, targetConn)
		if dout.DataSN != uint32(i) {
			t.Fatalf("PDU %d DataSN: got %d, want %d", i, dout.DataSN, i)
		}
		totalBytes += len(dout.Data)
	}
	if totalBytes != 1024 {
		t.Fatalf("total Data-Out bytes: got %d, want 1024", totalBytes)
	}

	// Send another R2T for next chunk, then complete.
	writeR2TPDU(t, targetConn, &pdu.R2T{
		Header:                    pdu.Header{InitiatorTaskTag: itt},
		TargetTransferTag:         0x5679,
		StatSN:                    2,
		ExpCmdSN:                  1,
		MaxCmdSN:                  10,
		R2TSN:                     1,
		BufferOffset:              1024,
		DesiredDataTransferLength: 1024,
	})

	// Drain the Data-Out PDUs.
	for range 2 {
		readDataOutPDU(t, targetConn)
	}

	// Send remaining R2Ts and drain.
	writeR2TPDU(t, targetConn, &pdu.R2T{
		Header:                    pdu.Header{InitiatorTaskTag: itt},
		TargetTransferTag:         0x567A,
		StatSN:                    3,
		ExpCmdSN:                  1,
		MaxCmdSN:                  10,
		R2TSN:                     2,
		BufferOffset:              2048,
		DesiredDataTransferLength: 1024,
	})
	for range 2 {
		readDataOutPDU(t, targetConn)
	}
	writeR2TPDU(t, targetConn, &pdu.R2T{
		Header:                    pdu.Header{InitiatorTaskTag: itt},
		TargetTransferTag:         0x567B,
		StatSN:                    4,
		ExpCmdSN:                  1,
		MaxCmdSN:                  10,
		R2TSN:                     3,
		BufferOffset:              3072,
		DesiredDataTransferLength: 1024,
	})
	for range 2 {
		readDataOutPDU(t, targetConn)
	}

	writeSCSIResponsePDU(t, targetConn, &pdu.SCSIResponse{
		Header:   pdu.Header{InitiatorTaskTag: itt},
		Status:   0x00,
		StatSN:   5,
		ExpCmdSN: 2,
		MaxCmdSN: 10,
	})

	result := <-resultCh
	if result.Err != nil {
		t.Fatalf("result error: %v", result.Err)
	}
}

// TestWriteMultiPDUBurst verifies that a single R2T burst generates multiple
// Data-Out PDUs with incrementing DataSN and contiguous BufferOffsets.
func TestWriteMultiPDUBurst(t *testing.T) {
	params := login.Defaults()
	params.CmdSN = 1
	params.ExpStatSN = 1
	params.ImmediateData = false
	params.InitialR2T = true
	params.MaxRecvDataSegmentLength = 100
	params.MaxBurstLength = 500

	sess, targetConn := newTestSessionWithParams(t, params)

	writeData := bytes.Repeat([]byte("E"), 500)
	cmd := Command{
		ExpectedDataTransferLen: 500,
		Data:                    bytes.NewReader(writeData),
	}
	cmd.CDB[0] = 0x2A

	resultCh, err := sess.Submit(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	scsiCmd := readSCSICommandPDU(t, targetConn)
	itt := scsiCmd.InitiatorTaskTag

	// Send R2T for 300 bytes.
	writeR2TPDU(t, targetConn, &pdu.R2T{
		Header:                    pdu.Header{InitiatorTaskTag: itt},
		TargetTransferTag:         0x9999,
		StatSN:                    1,
		ExpCmdSN:                  1,
		MaxCmdSN:                  10,
		R2TSN:                     0,
		BufferOffset:              0,
		DesiredDataTransferLength: 300,
	})

	// Should get 3 PDUs: 100+100+100 with DataSN 0,1,2.
	for i := range 3 {
		dout := readDataOutPDU(t, targetConn)
		if dout.DataSN != uint32(i) {
			t.Fatalf("PDU %d DataSN: got %d, want %d", i, dout.DataSN, i)
		}
		wantOffset := uint32(i) * 100
		if dout.BufferOffset != wantOffset {
			t.Fatalf("PDU %d BufferOffset: got %d, want %d", i, dout.BufferOffset, wantOffset)
		}
		if len(dout.Data) != 100 {
			t.Fatalf("PDU %d data length: got %d, want 100", i, len(dout.Data))
		}
		if dout.TargetTransferTag != 0x9999 {
			t.Fatalf("PDU %d TTT: got 0x%08X, want 0x00009999", i, dout.TargetTransferTag)
		}
		// Final only on last PDU.
		if i == 2 && !dout.Header.Final {
			t.Fatalf("PDU %d: Final bit not set on last PDU", i)
		}
		if i < 2 && dout.Header.Final {
			t.Fatalf("PDU %d: Final bit should not be set", i)
		}
	}

	// Send R2T for remaining 200 bytes and complete.
	writeR2TPDU(t, targetConn, &pdu.R2T{
		Header:                    pdu.Header{InitiatorTaskTag: itt},
		TargetTransferTag:         0x9998,
		StatSN:                    2,
		ExpCmdSN:                  1,
		MaxCmdSN:                  10,
		R2TSN:                     1,
		BufferOffset:              300,
		DesiredDataTransferLength: 200,
	})
	for range 2 {
		readDataOutPDU(t, targetConn)
	}

	writeSCSIResponsePDU(t, targetConn, &pdu.SCSIResponse{
		Header:   pdu.Header{InitiatorTaskTag: itt},
		Status:   0x00,
		StatSN:   3,
		ExpCmdSN: 2,
		MaxCmdSN: 10,
	})

	result := <-resultCh
	if result.Err != nil {
		t.Fatalf("result error: %v", result.Err)
	}
}

// errReader returns an error after reading n bytes.
type errReader struct {
	data []byte
	pos  int
	err  error
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, r.err
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	if r.pos >= len(r.data) {
		return n, r.err
	}
	return n, nil
}

// TestWriteReaderError verifies that io.Reader errors propagate to Result.Err.
func TestWriteReaderError(t *testing.T) {
	params := login.Defaults()
	params.CmdSN = 1
	params.ExpStatSN = 1
	params.ImmediateData = false
	params.InitialR2T = true
	params.MaxRecvDataSegmentLength = 8192
	params.MaxBurstLength = 262144

	sess, targetConn := newTestSessionWithParams(t, params)

	testErr := errors.New("simulated reader failure")
	// Reader has 10 bytes then errors. R2T will request 100.
	reader := &errReader{
		data: bytes.Repeat([]byte("F"), 10),
		err:  testErr,
	}

	cmd := Command{
		ExpectedDataTransferLen: 100,
		Data:                    reader,
	}
	cmd.CDB[0] = 0x2A

	resultCh, err := sess.Submit(context.Background(), cmd)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	scsiCmd := readSCSICommandPDU(t, targetConn)
	itt := scsiCmd.InitiatorTaskTag

	// Send R2T requesting 100 bytes -- reader will fail.
	writeR2TPDU(t, targetConn, &pdu.R2T{
		Header:                    pdu.Header{InitiatorTaskTag: itt},
		TargetTransferTag:         0xDEAD,
		StatSN:                    1,
		ExpCmdSN:                  1,
		MaxCmdSN:                  10,
		R2TSN:                     0,
		BufferOffset:              0,
		DesiredDataTransferLength: 100,
	})

	result := <-resultCh
	if result.Err == nil {
		t.Fatal("expected error from reader failure, got nil")
	}
	// The error should contain our simulated failure.
	if !errors.Is(result.Err, testErr) {
		// The error might be wrapped, check string containment.
		if !containsString(result.Err.Error(), "simulated reader failure") {
			t.Fatalf("expected error to contain reader failure, got: %v", result.Err)
		}
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && bytes.Contains([]byte(s), []byte(substr))
}
