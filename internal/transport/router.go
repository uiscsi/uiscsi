package transport

import "sync"

// reservedITT is the Initiator Task Tag value reserved by RFC 7143 for
// unsolicited target PDUs (NOP-In pings, async messages). The Router
// never allocates this value.
const reservedITT uint32 = 0xFFFFFFFF

// Router manages ITT-based PDU dispatch. When a command goroutine sends
// a request, it registers an ITT via Register and waits on the returned
// channel. When the read pump receives a response, it calls Dispatch with
// the ITT from the BHS to deliver the response to the correct waiter.
type Router struct {
	mu      sync.Mutex
	pending map[uint32]chan<- *RawPDU
	nextITT uint32
}

// NewRouter creates a Router with an empty pending map.
func NewRouter() *Router {
	return &Router{pending: make(map[uint32]chan<- *RawPDU)}
}

// Register allocates the next available ITT (skipping the reserved value
// 0xFFFFFFFF) and returns it along with a receive-only channel that will
// carry the response PDU. The channel has capacity 1 so the dispatcher
// never blocks.
func (r *Router) Register() (uint32, <-chan *RawPDU) {
	r.mu.Lock()
	defer r.mu.Unlock()

	itt := r.nextITT
	if itt == reservedITT {
		itt = 0
	}
	r.nextITT = itt + 1
	if r.nextITT == reservedITT {
		r.nextITT = 0
	}

	ch := make(chan *RawPDU, 1)
	r.pending[itt] = ch
	return itt, ch
}

// Dispatch delivers pdu to the channel registered for the given ITT.
// It removes the entry from the pending map after delivery. Returns true
// if the ITT was found, false otherwise (unsolicited or unknown ITT).
func (r *Router) Dispatch(itt uint32, pdu *RawPDU) bool {
	r.mu.Lock()
	ch, ok := r.pending[itt]
	if ok {
		delete(r.pending, itt)
	}
	r.mu.Unlock()

	if !ok {
		return false
	}
	ch <- pdu
	return true
}

// Unregister removes a pending ITT entry without delivering a PDU.
// Used for timeout/cancellation cleanup.
func (r *Router) Unregister(itt uint32) {
	r.mu.Lock()
	delete(r.pending, itt)
	r.mu.Unlock()
}

// PendingCount returns the number of ITTs currently awaiting responses.
// Intended for diagnostics and testing.
func (r *Router) PendingCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.pending)
}
