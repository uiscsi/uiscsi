package session

import (
	"fmt"
	"io"
	"testing"

	"github.com/rkujawa/uiscsi/internal/pdu"
)

func TestTaskSingleDataIn(t *testing.T) {
	tk := newTask(1, true, false, 0)

	data := []byte("hello iSCSI")
	din := &pdu.DataIn{
		HasStatus:    true,
		Status:       0x00, // GOOD
		DataSN:       0,
		BufferOffset: 0,
		Data:         data,
	}
	tk.handleDataIn(din)

	result := <-tk.resultCh
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.Status != 0x00 {
		t.Fatalf("status: got 0x%02X, want 0x00", result.Status)
	}
	if result.Data == nil {
		t.Fatal("Data is nil for read command")
	}
	got, err := io.ReadAll(result.Data)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != string(data) {
		t.Fatalf("data: got %q, want %q", got, data)
	}
}

func TestTaskMultiDataIn(t *testing.T) {
	tk := newTask(1, true, false, 0)

	// 3 Data-In PDUs without status, then a SCSIResponse.
	chunks := [][]byte{
		[]byte("chunk1"),
		[]byte("chunk2"),
		[]byte("chunk3"),
	}
	offset := uint32(0)
	for i, chunk := range chunks {
		din := &pdu.DataIn{
			DataSN:       uint32(i),
			BufferOffset: offset,
			Data:         chunk,
		}
		tk.handleDataIn(din)
		offset += uint32(len(chunk))
	}

	resp := &pdu.SCSIResponse{
		Status: 0x00,
	}
	tk.handleSCSIResponse(resp)

	result := <-tk.resultCh
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.Status != 0x00 {
		t.Fatalf("status: got 0x%02X, want 0x00", result.Status)
	}

	got, err := io.ReadAll(result.Data)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	want := "chunk1chunk2chunk3"
	if string(got) != want {
		t.Fatalf("data: got %q, want %q", got, want)
	}
}

func TestTaskDataSNGap(t *testing.T) {
	tk := newTask(1, true, false, 0)

	// First Data-In is fine.
	tk.handleDataIn(&pdu.DataIn{
		DataSN:       0,
		BufferOffset: 0,
		Data:         []byte("ok"),
	})

	// Second Data-In has wrong DataSN (skip to 5).
	tk.handleDataIn(&pdu.DataIn{
		DataSN:       5, // expected 1
		BufferOffset: 2,
		Data:         []byte("bad"),
	})

	result := <-tk.resultCh
	if result.Err == nil {
		t.Fatal("expected error from DataSN gap")
	}
}

func TestTaskOffsetMismatch(t *testing.T) {
	tk := newTask(1, true, false, 0)

	// First Data-In.
	tk.handleDataIn(&pdu.DataIn{
		DataSN:       0,
		BufferOffset: 0,
		Data:         []byte("ab"),
	})

	// Second Data-In with wrong offset.
	tk.handleDataIn(&pdu.DataIn{
		DataSN:       1,
		BufferOffset: 99, // expected 2
		Data:         []byte("cd"),
	})

	result := <-tk.resultCh
	if result.Err == nil {
		t.Fatal("expected error from offset mismatch")
	}
}

func TestTaskStreamingReader(t *testing.T) {
	tk := newTask(1, true, false, 0)

	// Feed 3 Data-In PDUs of 8 bytes each, then a SCSIResponse.
	chunks := [][]byte{
		[]byte("AAAABBBB"),
		[]byte("CCCCDDDD"),
		[]byte("EEEEFFFF"),
	}
	offset := uint32(0)
	for i, chunk := range chunks {
		din := &pdu.DataIn{
			DataSN:       uint32(i),
			BufferOffset: offset,
			Data:         chunk,
		}
		tk.handleDataIn(din)
		offset += uint32(len(chunk))
	}

	resp := &pdu.SCSIResponse{Status: 0x00}
	tk.handleSCSIResponse(resp)

	result := <-tk.resultCh
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}

	// Read from result.Data via streaming (4-byte reads to test partial consumption).
	var got []byte
	buf := make([]byte, 4)
	for {
		n, err := result.Data.Read(buf)
		got = append(got, buf[:n]...)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("Read error: %v", err)
		}
	}
	want := "AAAABBBBCCCCDDDDEEEEFFFF"
	if string(got) != want {
		t.Fatalf("data: got %q, want %q", got, want)
	}
}

func TestTaskStreamingSingleDataIn(t *testing.T) {
	tk := newTask(1, true, false, 8) // streaming=true

	data := []byte("streaming hello")

	// Must read from chanReader concurrently since handleDataIn pushes
	// to the channel synchronously.
	done := make(chan []byte, 1)
	go func() {
		got, _ := io.ReadAll(tk.dataReader)
		done <- got
	}()

	din := &pdu.DataIn{
		HasStatus:    true,
		Status:       0x00,
		DataSN:       0,
		BufferOffset: 0,
		Data:         data,
	}
	tk.handleDataIn(din)

	got := <-done
	if string(got) != "streaming hello" {
		t.Fatalf("data: got %q, want %q", got, "streaming hello")
	}

	result := <-tk.resultCh
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.Status != 0x00 {
		t.Fatalf("status: got 0x%02X, want 0x00", result.Status)
	}
	if result.Data != nil {
		t.Fatal("streaming result should have nil Data (data delivered via chanReader)")
	}
}

func TestTaskStreamingMultiDataIn(t *testing.T) {
	tk := newTask(1, true, false, 8) // streaming=true

	chunks := [][]byte{
		[]byte("chunk1"),
		[]byte("chunk2"),
		[]byte("chunk3"),
	}

	done := make(chan []byte, 1)
	go func() {
		got, _ := io.ReadAll(tk.dataReader)
		done <- got
	}()

	offset := uint32(0)
	for i, chunk := range chunks {
		tk.handleDataIn(&pdu.DataIn{
			DataSN:       uint32(i),
			BufferOffset: offset,
			Data:         chunk,
		})
		offset += uint32(len(chunk))
	}

	resp := &pdu.SCSIResponse{Status: 0x00}
	tk.handleSCSIResponse(resp)

	got := <-done
	if string(got) != "chunk1chunk2chunk3" {
		t.Fatalf("data: got %q, want %q", got, "chunk1chunk2chunk3")
	}

	result := <-tk.resultCh
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.Status != 0x00 {
		t.Fatalf("status: got 0x%02X, want 0x00", result.Status)
	}
}

func TestTaskStreamingCancel(t *testing.T) {
	tk := newTask(1, true, false, 8) // streaming=true

	// Feed one chunk then cancel.
	tk.handleDataIn(&pdu.DataIn{
		DataSN:       0,
		BufferOffset: 0,
		Data:         []byte("partial"),
	})

	tk.cancel(fmt.Errorf("connection lost"))

	// chanReader should return the buffered data then the error.
	got, err := io.ReadAll(tk.dataReader)
	if string(got) != "partial" {
		t.Fatalf("data: got %q, want %q", got, "partial")
	}
	if err == nil || err.Error() != "connection lost" {
		t.Fatalf("error: got %v, want 'connection lost'", err)
	}
}

func TestTaskStreamingDataSNGap(t *testing.T) {
	tk := newTask(1, true, false, 8) // streaming=true, ERL 0

	done := make(chan error, 1)
	go func() {
		_, err := io.ReadAll(tk.dataReader)
		done <- err
	}()

	// First PDU is fine.
	tk.handleDataIn(&pdu.DataIn{
		DataSN:       0,
		BufferOffset: 0,
		Data:         []byte("ok"),
	})

	// Second PDU has wrong DataSN (skip to 5).
	tk.handleDataIn(&pdu.DataIn{
		DataSN:       5,
		BufferOffset: 2,
		Data:         []byte("bad"),
	})

	// chanReader should return error from the gap detection.
	err := <-done
	if err == nil {
		t.Fatal("expected error from DataSN gap")
	}
}

func TestTaskNonReadCommand(t *testing.T) {
	tk := newTask(1, false, false, 0) // non-read

	resp := &pdu.SCSIResponse{
		Status: 0x00,
	}
	tk.handleSCSIResponse(resp)

	result := <-tk.resultCh
	if result.Err != nil {
		t.Fatalf("unexpected error: %v", result.Err)
	}
	if result.Data != nil {
		t.Fatal("Data should be nil for non-read command")
	}
	if result.Status != 0x00 {
		t.Fatalf("status: got 0x%02X, want 0x00", result.Status)
	}
}
