package session

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"

	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
)

func TestSNACK(t *testing.T) {
	t.Run("datasn_gap_triggers_snack", func(t *testing.T) {
		writeCh := make(chan *transport.RawPDU, 16)
		expStatSNFunc := func() uint32 { return 42 }
		tk := newTask(10, true, false)
		tk.erl = 1
		tk.getWriteCh = func() chan<- *transport.RawPDU { return writeCh }
		tk.expStatSNFunc = expStatSNFunc
		tk.snackTimeout = 0 // disable timer for this test

		// DataSN=0 is fine.
		tk.handleDataIn(&pdu.DataIn{
			DataSN:       0,
			BufferOffset: 0,
			Data:         []byte("aaa"),
		})

		// Skip DataSN=1, send DataSN=2 -> should trigger SNACK.
		tk.handleDataIn(&pdu.DataIn{
			DataSN:       2,
			BufferOffset: 6,
			Data:         []byte("ccc"),
		})

		// Verify SNACK was sent.
		select {
		case raw := <-writeCh:
			// Decode the SNACK PDU.
			snack := &pdu.SNACKReq{}
			snack.UnmarshalBHS(raw.BHS)

			if snack.Type != SNACKTypeDataR2T {
				t.Fatalf("SNACK Type: got %d, want %d (Data/R2T)", snack.Type, SNACKTypeDataR2T)
			}
			if snack.BegRun != 1 {
				t.Fatalf("SNACK BegRun: got %d, want 1", snack.BegRun)
			}
			if snack.RunLength != 1 {
				t.Fatalf("SNACK RunLength: got %d, want 1", snack.RunLength)
			}
		default:
			t.Fatal("expected SNACK on writeCh but got nothing")
		}

		// Verify DataSN=2 is buffered.
		if tk.snack == nil {
			t.Fatal("snackState is nil")
		}
		if _, ok := tk.snack.pendingDataIn[2]; !ok {
			t.Fatal("DataSN=2 not buffered in pendingDataIn")
		}
	})

	t.Run("gap_fill_completes_reassembly", func(t *testing.T) {
		writeCh := make(chan *transport.RawPDU, 16)
		expStatSNFunc := func() uint32 { return 42 }
		tk := newTask(10, true, false)
		tk.erl = 1
		tk.getWriteCh = func() chan<- *transport.RawPDU { return writeCh }
		tk.expStatSNFunc = expStatSNFunc
		tk.snackTimeout = 0

		// DataSN=0.
		tk.handleDataIn(&pdu.DataIn{
			DataSN:       0,
			BufferOffset: 0,
			Data:         []byte("aaa"),
		})

		// Skip DataSN=1, send DataSN=2 with HasStatus=true.
		tk.handleDataIn(&pdu.DataIn{
			DataSN:       2,
			BufferOffset: 6,
			Data:         []byte("ccc"),
			HasStatus:    true,
			Status:       0x00,
		})

		// Drain the SNACK from writeCh.
		<-writeCh

		// Now feed the retransmitted DataSN=1, which fills the gap.
		tk.handleDataIn(&pdu.DataIn{
			DataSN:       1,
			BufferOffset: 3,
			Data:         []byte("bbb"),
		})

		// The gap fill should cause drainPendingDataIn to process DataSN=2
		// which has HasStatus=true, delivering a Result.
		select {
		case result := <-tk.resultCh:
			if result.Err != nil {
				t.Fatalf("unexpected error: %v", result.Err)
			}
			if result.Status != 0x00 {
				t.Fatalf("status: got 0x%02X, want 0x00", result.Status)
			}
		default:
			t.Fatal("expected Result after gap fill but got nothing")
		}
	})

	t.Run("erl0_gap_is_fatal", func(t *testing.T) {
		tk := newTask(10, true, false)
		tk.erl = 0 // ERL 0 -- no SNACK

		// DataSN=0.
		tk.handleDataIn(&pdu.DataIn{
			DataSN:       0,
			BufferOffset: 0,
			Data:         []byte("ok"),
		})

		// Skip DataSN=1, send DataSN=2.
		tk.handleDataIn(&pdu.DataIn{
			DataSN:       2,
			BufferOffset: 4,
			Data:         []byte("bad"),
		})

		result := <-tk.resultCh
		if result.Err == nil {
			t.Fatal("expected error from DataSN gap at ERL 0")
		}
	})

	t.Run("snack_uses_task_itt", func(t *testing.T) {
		writeCh := make(chan *transport.RawPDU, 16)
		expStatSNFunc := func() uint32 { return 99 }
		tk := newTask(42, true, false)
		tk.erl = 1
		tk.getWriteCh = func() chan<- *transport.RawPDU { return writeCh }
		tk.expStatSNFunc = expStatSNFunc
		tk.snackTimeout = 0

		// DataSN=0 then skip to DataSN=2.
		tk.handleDataIn(&pdu.DataIn{DataSN: 0, BufferOffset: 0, Data: []byte("a")})
		tk.handleDataIn(&pdu.DataIn{DataSN: 2, BufferOffset: 2, Data: []byte("c")})

		raw := <-writeCh
		// ITT is at BHS bytes 16-19.
		gotITT := binary.BigEndian.Uint32(raw.BHS[16:20])
		if gotITT != 42 {
			t.Fatalf("SNACK ITT: got %d, want 42 (Pitfall 4: must use task's own ITT)", gotITT)
		}
	})

	t.Run("multiple_gaps", func(t *testing.T) {
		writeCh := make(chan *transport.RawPDU, 16)
		expStatSNFunc := func() uint32 { return 0 }
		tk := newTask(10, true, false)
		tk.erl = 1
		tk.getWriteCh = func() chan<- *transport.RawPDU { return writeCh }
		tk.expStatSNFunc = expStatSNFunc
		tk.snackTimeout = 0

		// DataSN=0.
		tk.handleDataIn(&pdu.DataIn{DataSN: 0, BufferOffset: 0, Data: []byte("aaa")})

		// Skip 1 and 2, send DataSN=3.
		tk.handleDataIn(&pdu.DataIn{DataSN: 3, BufferOffset: 9, Data: []byte("ddd")})

		raw := <-writeCh
		snack := &pdu.SNACKReq{}
		snack.UnmarshalBHS(raw.BHS)

		if snack.BegRun != 1 {
			t.Fatalf("SNACK BegRun: got %d, want 1", snack.BegRun)
		}
		if snack.RunLength != 2 {
			t.Fatalf("SNACK RunLength: got %d, want 2", snack.RunLength)
		}

		// Fill gaps: DataSN=1, DataSN=2, then DataSN=3 (already buffered) with status.
		// First update DataSN=3 to have status so drain completes.
		tk.snack.pendingDataIn[3] = &pdu.DataIn{
			DataSN:       3,
			BufferOffset: 9,
			Data:         []byte("ddd"),
			HasStatus:    true,
			Status:       0x00,
		}

		tk.handleDataIn(&pdu.DataIn{DataSN: 1, BufferOffset: 3, Data: []byte("bbb")})
		tk.handleDataIn(&pdu.DataIn{DataSN: 2, BufferOffset: 6, Data: []byte("ccc")})

		result := <-tk.resultCh
		if result.Err != nil {
			t.Fatalf("unexpected error: %v", result.Err)
		}
	})

	t.Run("timeout_tail_loss", func(t *testing.T) {
		writeCh := make(chan *transport.RawPDU, 16)
		expStatSNFunc := func() uint32 { return 50 }
		tk := newTask(10, true, false)
		tk.erl = 1
		tk.getWriteCh = func() chan<- *transport.RawPDU { return writeCh }
		tk.expStatSNFunc = expStatSNFunc
		tk.snackTimeout = 100 * time.Millisecond

		// Feed DataSN=0 -- this starts the SNACK timer.
		tk.handleDataIn(&pdu.DataIn{DataSN: 0, BufferOffset: 0, Data: []byte("x")})

		// Start the SNACK timer explicitly (simulating what happens at task creation for ERL >= 1).
		tk.startSnackTimer(tk.snackTimeout, tk.getWriteCh, tk.expStatSNFunc)

		// Wait for timeout to fire.
		time.Sleep(200 * time.Millisecond)

		// Verify Status SNACK was sent.
		select {
		case raw := <-writeCh:
			snack := &pdu.SNACKReq{}
			snack.UnmarshalBHS(raw.BHS)
			if snack.Type != SNACKTypeStatus {
				t.Fatalf("SNACK Type: got %d, want %d (Status)", snack.Type, SNACKTypeStatus)
			}
		default:
			t.Fatal("expected Status SNACK after timeout but got nothing")
		}

		tk.stopSnackTimer()
	})

	t.Run("timeout_reset_on_datain", func(t *testing.T) {
		writeCh := make(chan *transport.RawPDU, 16)
		expStatSNFunc := func() uint32 { return 50 }
		tk := newTask(10, true, false)
		tk.erl = 1
		tk.getWriteCh = func() chan<- *transport.RawPDU { return writeCh }
		tk.expStatSNFunc = expStatSNFunc
		tk.snackTimeout = 200 * time.Millisecond

		// Start the SNACK timer.
		tk.startSnackTimer(tk.snackTimeout, tk.getWriteCh, tk.expStatSNFunc)

		// Feed DataSN=0 at t=0, which resets timer via handleDataIn.
		tk.handleDataIn(&pdu.DataIn{DataSN: 0, BufferOffset: 0, Data: []byte("a")})

		// Wait 100ms, feed DataSN=1, which resets timer again.
		time.Sleep(100 * time.Millisecond)
		tk.handleDataIn(&pdu.DataIn{DataSN: 1, BufferOffset: 1, Data: []byte("b")})

		// Wait 100ms -- timer should NOT have fired (reset at t=100ms, only 100ms passed).
		time.Sleep(100 * time.Millisecond)
		select {
		case <-writeCh:
			t.Fatal("Status SNACK fired too early -- timer was not properly reset")
		default:
			// Good -- no SNACK yet.
		}

		// Now wait another 200ms with no Data-In -- timer should fire.
		time.Sleep(250 * time.Millisecond)
		select {
		case raw := <-writeCh:
			snack := &pdu.SNACKReq{}
			snack.UnmarshalBHS(raw.BHS)
			if snack.Type != SNACKTypeStatus {
				t.Fatalf("SNACK Type: got %d, want %d (Status)", snack.Type, SNACKTypeStatus)
			}
		default:
			t.Fatal("expected Status SNACK after timeout but got nothing")
		}

		tk.stopSnackTimer()
	})
}

// TestSNACK_SendTimeoutOnFullChannel verifies that sendSNACK returns a
// timeout error when the write channel is full (AUDIT-6 regression test).
func TestSNACK_SendTimeoutOnFullChannel(t *testing.T) {
	// Create a zero-buffered channel that will always block.
	writeCh := make(chan *transport.RawPDU)
	tk := newTask(10, true, false)
	tk.erl = 1
	tk.getWriteCh = func() chan<- *transport.RawPDU { return writeCh }
	tk.expStatSNFunc = func() uint32 { return 1 }

	// sendSNACK should timeout (5s in production, but we can't wait that long).
	// Instead, verify it returns an error rather than blocking forever by using
	// a goroutine with a deadline.
	done := make(chan error, 1)
	go func() {
		done <- tk.sendSNACK(tk.getWriteCh, SNACKTypeDataR2T, 0, 1, 1)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error from sendSNACK on full channel")
		}
		if !strings.Contains(err.Error(), "SNACK send timed out") {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("sendSNACK blocked forever instead of timing out")
	}
}
