package transport

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/uiscsi/uiscsi/internal/pdu"
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
		errCh <- ReadPump(ctx, rConn, router, unsolicitedCh, false, false, slog.Default(), nil, 0, nil, nil)
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
		ReadPump(ctx, rConn, router, unsolicitedCh, false, false, slog.Default(), nil, 0, nil, nil)
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
		readErr <- ReadPump(ctx, rConn, router, unsolicitedCh, false, false, slog.Default(), nil, 0, nil, nil)
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
	go ReadPump(ctx, rConn, router, unsolicitedCh, false, false, slog.Default(), nil, 0, nil, nil)

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

	go ReadPump(ctx, rConn, router, unsolicitedCh, false, false, slog.Default(), hook, 0, nil, nil)

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

	go ReadPump(ctx, rConn, router, unsolicitedCh, false, false, logger, nil, 0, nil, nil)

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

// makeUnsolicitedPDU creates a minimal unsolicited PDU (ITT=0xFFFFFFFF) with
// the given opcode for testing ReadPump classification logic.
func makeUnsolicitedPDU(opcode pdu.OpCode) [pdu.BHSLength]byte {
	bhs := makeBHS(opcode, 0, 0)
	// ITT = 0xFFFFFFFF (reserved / unsolicited)
	binary.BigEndian.PutUint32(bhs[16:20], 0xFFFFFFFF)
	return bhs
}

// TestReadPump_NOPInNeverDropped verifies that NOP-In PDUs (opcode 0x20) are
// never silently dropped even when unsolicitedCh is full. The blocking send
// must hold the PDU until the channel drains.
func TestReadPump_NOPInNeverDropped(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	router := NewRouter(0)
	// Capacity 1, pre-fill with a dummy PDU so it is "full".
	unsolicitedCh := make(chan *RawPDU, 1)
	unsolicitedCh <- &RawPDU{} // fill it

	dropCounter := &atomic.Uint64{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pumpDone := make(chan error, 1)
	go func() {
		pumpDone <- ReadPump(ctx, rConn, router, unsolicitedCh, false, false,
			slog.Default(), nil, 0, nil, dropCounter)
	}()

	// Send a NOP-In with ITT=0xFFFFFFFF.
	bhs := makeUnsolicitedPDU(pdu.OpNOPIn)
	writeRawBytes(wConn, bhs, nil, nil, false, false)

	// Drain the pre-filled dummy, which unblocks ReadPump's blocking send.
	// No sleep needed: the drain itself creates the backpressure-release that
	// allows ReadPump to deliver the NOP-In. If ReadPump incorrectly dropped
	// the NOP-In instead of blocking, it will never arrive (timeout below).
	select {
	case <-unsolicitedCh: // remove the pre-filled dummy
	case <-time.After(2 * time.Second):
		t.Fatal("timeout draining pre-filled PDU")
	}

	// NOP-In must be delivered — it must not have been dropped.
	select {
	case got := <-unsolicitedCh:
		opcode := pdu.OpCode(got.BHS[0] & 0x3f)
		if opcode != pdu.OpNOPIn {
			t.Errorf("delivered opcode = %v, want OpNOPIn", opcode)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("NOP-In not delivered — was incorrectly dropped instead of blocked")
	}

	// Drop counter must be zero throughout.
	if got := dropCounter.Load(); got != 0 {
		t.Errorf("drop counter = %d, want 0 — NOP-In must never be dropped", got)
	}

	// Shut down cleanly.
	cancel()
	wConn.Close()
	select {
	case <-pumpDone:
	case <-time.After(2 * time.Second):
		t.Fatal("ReadPump did not exit after cancel + close")
	}
}

// TestReadPump_OptionalPDUDropCounted verifies that non-NOP-In unsolicited PDUs
// (e.g. vendor-specific) are counted when dropped due to a full channel.
func TestReadPump_OptionalPDUDropCounted(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	router := NewRouter(0)
	// Capacity 1, pre-fill so it's full.
	unsolicitedCh := make(chan *RawPDU, 1)
	unsolicitedCh <- &RawPDU{}

	dropCounter := &atomic.Uint64{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pumpDone := make(chan error, 1)
	go func() {
		pumpDone <- ReadPump(ctx, rConn, router, unsolicitedCh, false, false,
			slog.Default(), nil, 0, nil, dropCounter)
	}()

	// Send a vendor-specific async PDU (opcode 0x3e is not a known opcode,
	// any non-NOP-In opcode with ITT=0xFFFFFFFF qualifies as optional/droppable).
	// We use OpAsyncMsg (0x32) since it's a real optional async PDU.
	bhs := makeUnsolicitedPDU(pdu.OpAsyncMsg)
	writeRawBytes(wConn, bhs, nil, nil, false, false)

	// Wait for ReadPump to process the PDU and drop it.
	deadline := time.After(2 * time.Second)
	for {
		if dropCounter.Load() >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for drop counter to reach 1; got %d", dropCounter.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}

	if got := dropCounter.Load(); got != 1 {
		t.Errorf("drop counter = %d, want 1", got)
	}

	cancel()
	wConn.Close()
	select {
	case <-pumpDone:
	case <-time.After(2 * time.Second):
		t.Fatal("ReadPump did not exit")
	}
}

// TestReadPump_DropCounterAccumulates verifies that multiple dropped optional
// PDUs accumulate correctly in the drop counter.
func TestReadPump_DropCounterAccumulates(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	router := NewRouter(0)
	// Capacity 1, pre-fill so it's always full.
	unsolicitedCh := make(chan *RawPDU, 1)
	unsolicitedCh <- &RawPDU{}

	dropCounter := &atomic.Uint64{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pumpDone := make(chan error, 1)
	go func() {
		pumpDone <- ReadPump(ctx, rConn, router, unsolicitedCh, false, false,
			slog.Default(), nil, 0, nil, dropCounter)
	}()

	const numDropped = 3
	for i := 0; i < numDropped; i++ {
		bhs := makeUnsolicitedPDU(pdu.OpAsyncMsg)
		writeRawBytes(wConn, bhs, nil, nil, false, false)
	}

	// Wait for all drops to register.
	deadline := time.After(5 * time.Second)
	for {
		if dropCounter.Load() >= numDropped {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for drop counter = %d; got %d", numDropped, dropCounter.Load())
		case <-time.After(10 * time.Millisecond):
		}
	}

	if got := dropCounter.Load(); got != numDropped {
		t.Errorf("drop counter = %d, want %d", got, numDropped)
	}

	cancel()
	wConn.Close()
	select {
	case <-pumpDone:
	case <-time.After(2 * time.Second):
		t.Fatal("ReadPump did not exit")
	}
}

// TestReadPump_DropCounterNilSafe verifies that ReadPump handles a nil
// dropCounter gracefully (no panic on optional PDU drop).
func TestReadPump_DropCounterNilSafe(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	router := NewRouter(0)
	unsolicitedCh := make(chan *RawPDU, 1)
	unsolicitedCh <- &RawPDU{} // full

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pumpDone := make(chan error, 1)
	go func() {
		// nil dropCounter — must not panic
		pumpDone <- ReadPump(ctx, rConn, router, unsolicitedCh, false, false,
			slog.Default(), nil, 0, nil, nil)
	}()

	bhs := makeUnsolicitedPDU(pdu.OpAsyncMsg)
	writeRawBytes(wConn, bhs, nil, nil, false, false)

	// Give pump time to process without panicking.
	time.Sleep(100 * time.Millisecond)

	cancel()
	wConn.Close()
	select {
	case <-pumpDone:
	case <-time.After(2 * time.Second):
		t.Fatal("ReadPump did not exit")
	}
}

// --- Task 2: FaultConn exit path tests and single-writer invariant ---

// TestPump_NormalClose verifies that cancelling the context causes both pumps
// to exit cleanly. No goroutine leak.
func TestPump_NormalClose(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	router := NewRouter(16)
	unsolCh := make(chan *RawPDU, 64)
	dropCounter := &atomic.Uint64{}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ReadPump(ctx, client, router, unsolCh, false, false,
			slog.Default(), nil, 0, binary.LittleEndian, dropCounter)
	}()

	cancel()
	client.Close() // unblock ReadPump blocked in io.ReadFull
	wg.Wait()
	// goleak in TestMain verifies no goroutine leak.
}

// TestPump_ReadError verifies that a read error from the connection causes
// ReadPump to return the error without a goroutine leak.
func TestPump_ReadError(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	faultClient := NewFaultConn(client, WithReadFaultAfter(0, errReadFault), nil)
	defer faultClient.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	router := NewRouter(0)
	unsolCh := make(chan *RawPDU, 64)
	dropCounter := &atomic.Uint64{}

	pumpDone := make(chan error, 1)
	go func() {
		pumpDone <- ReadPump(ctx, faultClient, router, unsolCh, false, false,
			slog.Default(), nil, 0, nil, dropCounter)
	}()

	select {
	case err := <-pumpDone:
		if err == nil {
			t.Error("ReadPump returned nil, want read fault error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ReadPump did not exit after read error")
	}
	server.Close()
}

// TestPump_WriteError verifies that a write error from the connection causes
// WritePump to return the error without a goroutine leak.
func TestPump_WriteError(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	faultClient := NewFaultConn(client, nil, WithWriteFaultAfter(0, errWriteFault))
	defer faultClient.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writeCh := make(chan *RawPDU, 10)

	pumpDone := make(chan error, 1)
	go func() {
		pumpDone <- WritePump(ctx, faultClient, writeCh, slog.Default(), nil, nil)
	}()

	// Send a PDU to trigger the write fault.
	writeCh <- &RawPDU{}

	select {
	case err := <-pumpDone:
		if err == nil {
			t.Error("WritePump returned nil, want write fault error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WritePump did not exit after write error")
	}
	server.Close()
}

// TestPump_TCPReset verifies that closing the remote side mid-read causes
// ReadPump to return with an error.
func TestPump_TCPReset(t *testing.T) {
	server, client := net.Pipe()
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	router := NewRouter(0)
	unsolCh := make(chan *RawPDU, 64)

	pumpDone := make(chan error, 1)
	go func() {
		pumpDone <- ReadPump(ctx, client, router, unsolCh, false, false,
			slog.Default(), nil, 0, nil, nil)
	}()

	// Close the remote side to simulate TCP reset/EOF.
	server.Close()

	select {
	case err := <-pumpDone:
		if err == nil {
			t.Error("ReadPump returned nil, want EOF or closed pipe error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ReadPump did not exit after TCP reset")
	}
}

// TestPump_ContextCancelDuringRead verifies that cancelling the context while
// ReadPump is blocked in io.ReadFull causes it to return (after conn.Close
// unblocks the syscall). Returns ctx.Err() or a connection error.
func TestPump_ContextCancelDuringRead(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())

	router := NewRouter(0)
	unsolCh := make(chan *RawPDU, 64)

	pumpDone := make(chan error, 1)
	go func() {
		pumpDone <- ReadPump(ctx, client, router, unsolCh, false, false,
			slog.Default(), nil, 0, nil, nil)
	}()

	// Cancel the context, then close the conn to unblock io.ReadFull.
	cancel()
	client.Close()

	select {
	case err := <-pumpDone:
		// Acceptable: either context.Canceled or a connection error.
		_ = err
	case <-time.After(2 * time.Second):
		t.Fatal("ReadPump did not exit after context cancel + close")
	}
}

// TestWritePump_ConcurrentWriters verifies that multiple goroutines can send
// PDUs to WritePump via writeCh concurrently without data races.
// The -race detector catches any violations of the single-writer invariant.
func TestWritePump_ConcurrentWriters(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	writeCh := make(chan *RawPDU, 100)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		WritePump(ctx, client, writeCh, slog.Default(), nil, nil)
	}()

	// Drain server side so WritePump never blocks on TCP backpressure.
	go func() {
		buf := make([]byte, 65536)
		for {
			if _, err := server.Read(buf); err != nil {
				return
			}
		}
	}()

	const numWriters = 10
	const pdusPerWriter = 50
	var senderWg sync.WaitGroup
	for i := 0; i < numWriters; i++ {
		senderWg.Add(1)
		go func() {
			defer senderWg.Done()
			for j := 0; j < pdusPerWriter; j++ {
				writeCh <- &RawPDU{}
			}
		}()
	}
	senderWg.Wait()

	cancel()
	client.Close()
	wg.Wait()
	// If we reach here without a -race complaint, the single-writer invariant holds.
}

// sentinel errors for FaultConn injection in pump tests.
var (
	errReadFault  = fmt.Errorf("injected read fault")
	errWriteFault = fmt.Errorf("injected write fault")
)
