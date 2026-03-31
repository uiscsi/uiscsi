package transport

import (
	"sync"
	"testing"
)

func TestRouterRegister_MonotonicallyIncreasing(t *testing.T) {
	r := NewRouter()
	var prev uint32
	for i := 0; i < 10; i++ {
		itt, ch := r.Register()
		if ch == nil {
			t.Fatal("Register returned nil channel")
		}
		if i > 0 && itt <= prev {
			t.Errorf("ITT not increasing: prev=%d, got=%d", prev, itt)
		}
		prev = itt
	}
}

func TestRouterRegister_Skips0xFFFFFFFF(t *testing.T) {
	r := NewRouter()
	// Set nextITT to just before the reserved value.
	r.mu.Lock()
	r.nextITT = 0xFFFFFFFE
	r.mu.Unlock()

	itt1, _ := r.Register()
	if itt1 != 0xFFFFFFFE {
		t.Errorf("expected 0xFFFFFFFE, got 0x%08X", itt1)
	}

	itt2, _ := r.Register()
	if itt2 == 0xFFFFFFFF {
		t.Error("Router must not allocate reserved ITT 0xFFFFFFFF")
	}
	// Should have skipped to 0x00000000
	if itt2 != 0x00000000 {
		t.Errorf("expected 0x00000000 after skip, got 0x%08X", itt2)
	}
}

func TestRouterDispatch_RegisteredITT(t *testing.T) {
	r := NewRouter()
	itt, ch := r.Register()

	raw := &RawPDU{}
	raw.BHS[0] = 0x20 // NOPIn opcode

	ok := r.Dispatch(itt, raw)
	if !ok {
		t.Error("Dispatch returned false for registered ITT")
	}

	got := <-ch
	if got != raw {
		t.Error("received different PDU than dispatched")
	}
}

func TestRouterDispatch_UnregisteredITT(t *testing.T) {
	r := NewRouter()
	ok := r.Dispatch(999, &RawPDU{})
	if ok {
		t.Error("Dispatch returned true for unregistered ITT")
	}
}

func TestRouterUnregister(t *testing.T) {
	r := NewRouter()
	itt, _ := r.Register()
	r.Unregister(itt)

	ok := r.Dispatch(itt, &RawPDU{})
	if ok {
		t.Error("Dispatch should return false after Unregister")
	}
}

func TestRouterPendingCount(t *testing.T) {
	r := NewRouter()
	if r.PendingCount() != 0 {
		t.Errorf("initial PendingCount: got %d, want 0", r.PendingCount())
	}

	itt1, _ := r.Register()
	itt2, _ := r.Register()
	if r.PendingCount() != 2 {
		t.Errorf("PendingCount after 2 Register: got %d, want 2", r.PendingCount())
	}

	r.Dispatch(itt1, &RawPDU{})
	if r.PendingCount() != 1 {
		t.Errorf("PendingCount after Dispatch: got %d, want 1", r.PendingCount())
	}

	r.Unregister(itt2)
	if r.PendingCount() != 0 {
		t.Errorf("PendingCount after Unregister: got %d, want 0", r.PendingCount())
	}
}

func TestRouterConcurrent(t *testing.T) {
	r := NewRouter()
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			itt, ch := r.Register()
			r.Dispatch(itt, &RawPDU{})
			<-ch
		}()
	}
	wg.Wait()

	if r.PendingCount() != 0 {
		t.Errorf("PendingCount after concurrent test: got %d, want 0", r.PendingCount())
	}
}
