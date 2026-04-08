package transport

import "sync"

// reservedITT is the Initiator Task Tag value reserved by RFC 7143 for
// unsolicited target PDUs (NOP-In pings, async messages). The Router
// never allocates this value.
const reservedITT uint32 = 0xFFFFFFFF

// routerEntry holds a pending registration in the Router. Persistent entries
// survive Dispatch calls (used by session layer for multi-PDU commands).
type routerEntry struct {
	ch         chan<- *RawPDU
	persistent bool
}

// Router manages ITT-based PDU dispatch. When a command goroutine sends
// a request, it registers an ITT via Register and waits on the returned
// channel. When the read pump receives a response, it calls Dispatch with
// the ITT from the BHS to deliver the response to the correct waiter.
// DefaultRouterBufDepth is the default persistent channel buffer depth.
const DefaultRouterBufDepth = 64

type Router struct {
	mu              sync.Mutex
	pending         map[uint32]*routerEntry
	nextITT         uint32
	persistentDepth int // configurable persistent channel depth
}

// NewRouter creates a Router with an empty pending map.
// persistentDepth sets the buffer depth for persistent registrations
// (0 = DefaultRouterBufDepth).
func NewRouter(persistentDepth int) *Router {
	if persistentDepth <= 0 {
		persistentDepth = DefaultRouterBufDepth
	}
	return &Router{
		pending:         make(map[uint32]*routerEntry),
		persistentDepth: persistentDepth,
	}
}

// Register allocates the next available ITT (skipping the reserved value
// 0xFFFFFFFF) and returns it along with a receive-only channel that will
// carry the response PDU. The channel has capacity 1 so the dispatcher
// never blocks. The entry is removed after the first Dispatch.
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
	r.pending[itt] = &routerEntry{ch: ch, persistent: false}
	return itt, ch
}

// RegisterPersistent creates a persistent registration for the given ITT.
// Unlike Register, the entry is NOT removed after Dispatch -- it survives
// multiple PDU deliveries. The session layer uses this for SCSI commands
// that receive multiple Data-In PDUs before completion. The caller must
// call Unregister when the command completes.
func (r *Router) RegisterPersistent(itt uint32) <-chan *RawPDU {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch := make(chan *RawPDU, r.persistentDepth)
	r.pending[itt] = &routerEntry{ch: ch, persistent: true}
	return ch
}

// AllocateITT allocates the next available ITT without registering a channel.
// The caller registers separately via RegisterPersistent.
func (r *Router) AllocateITT() uint32 {
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
	return itt
}

// Dispatch delivers pdu to the channel registered for the given ITT.
// For non-persistent entries, it removes the entry after delivery.
// Returns true if the ITT was found, false otherwise.
func (r *Router) Dispatch(itt uint32, pdu *RawPDU) bool {
	r.mu.Lock()
	entry, ok := r.pending[itt]
	if ok && !entry.persistent {
		delete(r.pending, itt)
	}
	r.mu.Unlock()

	if !ok {
		return false
	}
	entry.ch <- pdu
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
