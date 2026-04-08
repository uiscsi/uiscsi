package transport

import (
	"context"
	"encoding/binary"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rkujawa/uiscsi/internal/pdu"
)

func TestWritePump_BasicWrite(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writeCh := make(chan *RawPDU, 10)

	// Start write pump
	errCh := make(chan error, 1)
	go func() {
		errCh <- WritePump(ctx, wConn, writeCh, slog.Default(), nil, nil)
	}()

	// Send 3 PDUs
	for i := 0; i < 3; i++ {
		raw := &RawPDU{}
		raw.BHS[0] = byte(i)
		writeCh <- raw
	}

	// Read 3 PDUs from other end
	for i := 0; i < 3; i++ {
		got, err := ReadRawPDU(rConn, false, false, 0)
		if err != nil {
			t.Fatalf("ReadRawPDU #%d: %v", i, err)
		}
		if got.BHS[0] != byte(i) {
			t.Errorf("PDU %d: BHS[0] = %d, want %d", i, got.BHS[0], i)
		}
	}

	cancel()
}

func TestReadPump_BasicDispatch(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	router := NewRouter(0)
	unsolicitedCh := make(chan *RawPDU, 10)

	// Register 3 ITTs and record the channels
	type entry struct {
		itt uint32
		ch  <-chan *RawPDU
	}
	entries := make([]entry, 3)
	for i := range entries {
		itt, ch := router.Register()
		entries[i] = entry{itt, ch}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start read pump
	errCh := make(chan error, 1)
	go func() {
		errCh <- ReadPump(ctx, rConn, router, unsolicitedCh, false, false, slog.Default(), nil, 0, nil)
	}()

	// Write 3 PDUs with matching ITTs
	for _, e := range entries {
		bhs := makeBHS(pdu.OpNOPIn, 0, 0)
		binary.BigEndian.PutUint32(bhs[16:20], e.itt)
		writeRawBytes(wConn, bhs, nil, nil, false, false)
	}

	// Verify all 3 dispatched
	for i, e := range entries {
		select {
		case got := <-e.ch:
			gotITT := binary.BigEndian.Uint32(got.BHS[16:20])
			if gotITT != e.itt {
				t.Errorf("PDU %d: ITT 0x%08X, want 0x%08X", i, gotITT, e.itt)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("PDU %d: timeout waiting for dispatch", i)
		}
	}
}

func TestReadPump_UnsolicitedITT(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	router := NewRouter(0)
	unsolicitedCh := make(chan *RawPDU, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		ReadPump(ctx, rConn, router, unsolicitedCh, false, false, slog.Default(), nil, 0, nil)
	}()

	// Write PDU with reserved ITT 0xFFFFFFFF
	bhs := makeBHS(pdu.OpNOPIn, 0, 0)
	binary.BigEndian.PutUint32(bhs[16:20], 0xFFFFFFFF)
	writeRawBytes(wConn, bhs, nil, nil, false, false)

	select {
	case got := <-unsolicitedCh:
		gotITT := binary.BigEndian.Uint32(got.BHS[16:20])
		if gotITT != 0xFFFFFFFF {
			t.Errorf("unsolicited PDU ITT: 0x%08X, want 0xFFFFFFFF", gotITT)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for unsolicited PDU")
	}
}

func TestPump_ConcurrentWriters(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writeCh := make(chan *RawPDU, 100)

	go func() {
		WritePump(ctx, wConn, writeCh, slog.Default(), nil, nil)
	}()

	const writers = 10
	var wg sync.WaitGroup
	wg.Add(writers)

	// 10 goroutines each send 1 PDU through the write channel
	for i := 0; i < writers; i++ {
		go func(id int) {
			defer wg.Done()
			raw := &RawPDU{}
			raw.BHS[0] = byte(id)
			writeCh <- raw
		}(i)
	}

	// Read all 10 from the other end
	seen := make(map[byte]bool)
	for i := 0; i < writers; i++ {
		got, err := ReadRawPDU(rConn, false, false, 0)
		if err != nil {
			t.Fatalf("ReadRawPDU %d: %v", i, err)
		}
		seen[got.BHS[0]] = true
	}

	wg.Wait()

	if len(seen) != writers {
		t.Errorf("received %d unique PDUs, want %d", len(seen), writers)
	}
}

func TestPump_Shutdown(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	ctx, cancel := context.WithCancel(context.Background())

	writeCh := make(chan *RawPDU, 10)
	unsolicitedCh := make(chan *RawPDU, 10)
	router := NewRouter(0)

	writeErr := make(chan error, 1)
	readErr := make(chan error, 1)

	go func() {
		writeErr <- WritePump(ctx, wConn, writeCh, slog.Default(), nil, nil)
	}()
	go func() {
		readErr <- ReadPump(ctx, rConn, router, unsolicitedCh, false, false, slog.Default(), nil, 0, nil)
	}()

	// Cancel context to trigger shutdown
	cancel()

	// Close pipes to unblock ReadPump
	wConn.Close()

	select {
	case <-writeErr:
		// OK
	case <-time.After(2 * time.Second):
		t.Error("WritePump did not shut down in time")
	}

	select {
	case <-readErr:
		// OK
	case <-time.After(2 * time.Second):
		t.Error("ReadPump did not shut down in time")
	}
}

func TestPump_FullRoundTrip(t *testing.T) {
	// Create a pipe for each direction to simulate full-duplex
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	router := NewRouter(0)
	unsolicitedCh := make(chan *RawPDU, 10)
	writeCh := make(chan *RawPDU, 10)

	// Start write pump on wConn, read pump on rConn
	go WritePump(ctx, wConn, writeCh, slog.Default(), nil, nil)
	go ReadPump(ctx, rConn, router, unsolicitedCh, false, false, slog.Default(), nil, 0, nil)

	// Register an ITT and send a PDU through the write pump
	itt, respCh := router.Register()

	outPDU := &RawPDU{}
	outPDU.BHS = makeBHS(pdu.OpNOPOut, 0, 0)
	binary.BigEndian.PutUint32(outPDU.BHS[16:20], itt)
	writeCh <- outPDU

	// The PDU goes through wConn -> rConn, read pump reads it and dispatches
	select {
	case got := <-respCh:
		gotITT := binary.BigEndian.Uint32(got.BHS[16:20])
		if gotITT != itt {
			t.Errorf("round-trip ITT: 0x%08X, want 0x%08X", gotITT, itt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout on full round trip")
	}
}

func TestReadPumpPDUHook(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	router := NewRouter(0)
	unsolicitedCh := make(chan *RawPDU, 10)

	itt, respCh := router.Register()

	var hookDir uint8
	var hookRaw *RawPDU
	var hookCalled atomic.Bool

	hook := func(dir uint8, raw *RawPDU) {
		hookDir = dir
		hookRaw = raw
		hookCalled.Store(true)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ReadPump(ctx, rConn, router, unsolicitedCh, false, false, slog.Default(), hook, 0, nil)

	// Send a PDU with matching ITT.
	bhs := makeBHS(pdu.OpSCSIResponse, 0, 0)
	binary.BigEndian.PutUint32(bhs[16:20], itt)
	writeRawBytes(wConn, bhs, nil, nil, false, false)

	// Wait for dispatch.
	select {
	case <-respCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for dispatch")
	}

	if !hookCalled.Load() {
		t.Fatal("ReadPump PDU hook was not called")
	}
	if hookDir != HookReceive {
		t.Errorf("hook direction = %d, want %d (HookReceive)", hookDir, HookReceive)
	}
	if hookRaw == nil {
		t.Fatal("hook received nil RawPDU")
	}
	gotOpcode := hookRaw.BHS[0] & 0x3f
	if pdu.OpCode(gotOpcode) != pdu.OpSCSIResponse {
		t.Errorf("hook opcode = 0x%02x, want 0x%02x", gotOpcode, pdu.OpSCSIResponse)
	}
}

func TestWritePumpPDUHook(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	var hookDir uint8
	var hookRaw *RawPDU
	var hookCalled atomic.Bool

	hook := func(dir uint8, raw *RawPDU) {
		hookDir = dir
		hookRaw = raw
		hookCalled.Store(true)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writeCh := make(chan *RawPDU, 10)

	go WritePump(ctx, wConn, writeCh, slog.Default(), hook, nil)

	outPDU := &RawPDU{}
	outPDU.BHS = makeBHS(pdu.OpSCSICommand, 0, 0)
	writeCh <- outPDU

	// Read the PDU from the other side to ensure WritePump processed it.
	_, err := ReadRawPDU(rConn, false, false, 0)
	if err != nil {
		t.Fatalf("ReadRawPDU: %v", err)
	}

	if !hookCalled.Load() {
		t.Fatal("WritePump PDU hook was not called")
	}
	if hookDir != HookSend {
		t.Errorf("hook direction = %d, want %d (HookSend)", hookDir, HookSend)
	}
	if hookRaw == nil {
		t.Fatal("hook received nil RawPDU")
	}
	gotOpcode := hookRaw.BHS[0] & 0x3f
	if pdu.OpCode(gotOpcode) != pdu.OpSCSICommand {
		t.Errorf("hook opcode = 0x%02x, want 0x%02x", gotOpcode, pdu.OpSCSICommand)
	}
}

// logRecord captures slog records for test assertions.
type logRecord struct {
	Level   slog.Level
	Message string
	Attrs   map[string]any
}

type captureHandler struct {
	records []logRecord
	mu      sync.Mutex
}

func (h *captureHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	rec := logRecord{Level: r.Level, Message: r.Message, Attrs: make(map[string]any)}
	r.Attrs(func(a slog.Attr) bool {
		rec.Attrs[a.Key] = a.Value.Any()
		return true
	})
	h.mu.Lock()
	h.records = append(h.records, rec)
	h.mu.Unlock()
	return nil
}

func (h *captureHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *captureHandler) getRecords() []logRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	cp := make([]logRecord, len(h.records))
	copy(cp, h.records)
	return cp
}

func TestReadPumpLogger(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	router := NewRouter(0)
	unsolicitedCh := make(chan *RawPDU, 10)

	itt, respCh := router.Register()

	handler := &captureHandler{}
	logger := slog.New(handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go ReadPump(ctx, rConn, router, unsolicitedCh, false, false, logger, nil, 0, nil)

	bhs := makeBHS(pdu.OpSCSIResponse, 0, 0)
	binary.BigEndian.PutUint32(bhs[16:20], itt)
	binary.BigEndian.PutUint32(bhs[24:28], 42) // StatSN = 42
	writeRawBytes(wConn, bhs, nil, nil, false, false)

	select {
	case <-respCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for dispatch")
	}

	records := handler.getRecords()
	found := false
	for _, rec := range records {
		if rec.Message == "pdu received" {
			found = true
			if _, ok := rec.Attrs["stat_sn"]; !ok {
				t.Error("pdu received log missing stat_sn attribute")
			}
			if _, ok := rec.Attrs["opcode"]; !ok {
				t.Error("pdu received log missing opcode attribute")
			}
			if _, ok := rec.Attrs["itt"]; !ok {
				t.Error("pdu received log missing itt attribute")
			}
			break
		}
	}
	if !found {
		t.Error("no 'pdu received' log record found")
	}
}
