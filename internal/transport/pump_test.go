package transport

import (
	"context"
	"encoding/binary"
	"net"
	"sync"
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
		errCh <- WritePump(ctx, wConn, writeCh)
	}()

	// Send 3 PDUs
	for i := 0; i < 3; i++ {
		raw := &RawPDU{}
		raw.BHS[0] = byte(i)
		writeCh <- raw
	}

	// Read 3 PDUs from other end
	for i := 0; i < 3; i++ {
		got, err := ReadRawPDU(rConn, false, false)
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

	router := NewRouter()
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
		errCh <- ReadPump(ctx, rConn, router, unsolicitedCh, false, false)
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

	router := NewRouter()
	unsolicitedCh := make(chan *RawPDU, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		ReadPump(ctx, rConn, router, unsolicitedCh, false, false)
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
		WritePump(ctx, wConn, writeCh)
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
		got, err := ReadRawPDU(rConn, false, false)
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
	router := NewRouter()

	writeErr := make(chan error, 1)
	readErr := make(chan error, 1)

	go func() {
		writeErr <- WritePump(ctx, wConn, writeCh)
	}()
	go func() {
		readErr <- ReadPump(ctx, rConn, router, unsolicitedCh, false, false)
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

	router := NewRouter()
	unsolicitedCh := make(chan *RawPDU, 10)
	writeCh := make(chan *RawPDU, 10)

	// Start write pump on wConn, read pump on rConn
	go WritePump(ctx, wConn, writeCh)
	go ReadPump(ctx, rConn, router, unsolicitedCh, false, false)

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
