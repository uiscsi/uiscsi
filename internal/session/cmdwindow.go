package session

import (
	"context"
	"errors"
	"sync"

	"github.com/uiscsi/uiscsi/internal/serial"
)

// errWindowClosed is returned by acquire when the command window has been closed.
var errWindowClosed = errors.New("session: command window closed")

// cmdWindow implements CmdSN/MaxCmdSN flow control per RFC 7143 Section 3.2.2.
// The initiator MUST NOT send a command with CmdSN outside the
// [ExpCmdSN, MaxCmdSN] window. acquire blocks until a slot is available.
type cmdWindow struct {
	mu       sync.Mutex
	cond     *sync.Cond
	cmdSN    uint32
	expCmdSN uint32
	maxCmdSN uint32
	closed   bool
}

// newCmdWindow creates a command window initialized with the post-login
// sequence number state.
func newCmdWindow(cmdSN, expCmdSN, maxCmdSN uint32) *cmdWindow {
	w := &cmdWindow{
		cmdSN:    cmdSN,
		expCmdSN: expCmdSN,
		maxCmdSN: maxCmdSN,
	}
	w.cond = sync.NewCond(&w.mu)
	return w
}

// windowOpen reports whether the command window is open. Per RFC 7143
// Section 3.2.2.1, the window is open when MaxCmdSN >= ExpCmdSN (serial)
// AND CmdSN is within [ExpCmdSN, MaxCmdSN]. When MaxCmdSN < ExpCmdSN
// (serial), the command window is closed (zero window) regardless of CmdSN.
func (w *cmdWindow) windowOpen() bool {
	if serial.LessThan(w.maxCmdSN, w.expCmdSN) {
		return false // zero window: MaxCmdSN < ExpCmdSN
	}
	return serial.InWindow(w.cmdSN, w.expCmdSN, w.maxCmdSN)
}

// acquire blocks until a CmdSN slot is available within the window, then
// returns the allocated CmdSN and increments the internal counter.
// Returns an error if the window is closed or the context is cancelled.
func (w *cmdWindow) acquire(ctx context.Context) (uint32, error) {
	w.mu.Lock()

	// Fast path: check immediately.
	if w.closed {
		w.mu.Unlock()
		return 0, errWindowClosed
	}
	if w.windowOpen() {
		sn := w.cmdSN
		w.cmdSN = serial.Incr(w.cmdSN)
		w.mu.Unlock()
		return sn, nil
	}

	// Slow path: need to wait. Use a goroutine to bridge sync.Cond and context.
	type result struct {
		sn  uint32
		err error
	}
	ch := make(chan result, 1)

	go func() {
		// We already hold the lock from the caller above.
		for !w.closed && !w.windowOpen() {
			w.cond.Wait()
		}
		if w.closed {
			w.mu.Unlock()
			ch <- result{err: errWindowClosed}
			return
		}
		sn := w.cmdSN
		w.cmdSN = serial.Incr(w.cmdSN)
		w.mu.Unlock()
		ch <- result{sn: sn}
	}()

	select {
	case r := <-ch:
		return r.sn, r.err
	case <-ctx.Done():
		// Wake the waiter so it can exit. It will see closed or re-check.
		w.mu.Lock()
		w.closed = true
		w.cond.Broadcast()
		w.mu.Unlock()
		// Drain the goroutine result.
		<-ch
		return 0, ctx.Err()
	}
}

// update advances the ExpCmdSN and MaxCmdSN from a target response PDU.
// Per RFC 7143 Section 3.2.2.1: if MaxCmdSN < ExpCmdSN-1 (in serial
// arithmetic), both values MUST be ignored (stale/invalid update).
//
// MaxCmdSN may decrease relative to the current value -- this is how the
// target closes the command window (flow control). The only constraint is
// the validity check above. ExpCmdSN must not go backwards (monotonic).
func (w *cmdWindow) update(expCmdSN, maxCmdSN uint32) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// RFC validity check: MaxCmdSN must be >= ExpCmdSN - 1 (serial).
	// If MaxCmdSN < ExpCmdSN-1 in serial space, ignore both values.
	if serial.LessThan(maxCmdSN, expCmdSN-1) {
		return
	}

	updated := false

	// ExpCmdSN is monotonically non-decreasing.
	if serial.GreaterThan(expCmdSN, w.expCmdSN) {
		w.expCmdSN = expCmdSN
		updated = true
	}

	// MaxCmdSN may increase or decrease (target flow control).
	// Accept any valid MaxCmdSN that differs from current.
	if maxCmdSN != w.maxCmdSN {
		w.maxCmdSN = maxCmdSN
		updated = true
	}

	if updated {
		w.cond.Broadcast()
	}
}

// close shuts down the command window, waking all blocked acquirers.
func (w *cmdWindow) close() {
	w.mu.Lock()
	w.closed = true
	w.cond.Broadcast()
	w.mu.Unlock()
}

// current returns the current CmdSN without advancing it.
// Used for immediate commands (e.g., NOP-Out) that carry CmdSN
// but do not consume a window slot per RFC 7143 Section 3.2.2.
func (w *cmdWindow) current() uint32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.cmdSN
}

// maxCmdSNValue returns the current MaxCmdSN. Used for logging
// command window changes.
func (w *cmdWindow) maxCmdSNValue() uint32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.maxCmdSN
}
