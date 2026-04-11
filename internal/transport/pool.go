// Package transport implements the iSCSI TCP transport layer: connection
// management, PDU framing over TCP streams, concurrent read/write pumps,
// ITT-based response routing, and buffer pool management.
package transport

import (
	"sync"

	"github.com/uiscsi/uiscsi/internal/pdu"
)

// bhsPool reuses 48-byte BHS buffers to reduce GC pressure during PDU framing.
var bhsPool = sync.Pool{
	New: func() any { return new([pdu.BHSLength]byte) },
}

// GetBHS returns a reusable 48-byte BHS buffer from the pool.
// The caller must call PutBHS when done.
func GetBHS() *[pdu.BHSLength]byte {
	return bhsPool.Get().(*[pdu.BHSLength]byte)
}

// PutBHS returns a BHS buffer to the pool for reuse.
func PutBHS(b *[pdu.BHSLength]byte) {
	bhsPool.Put(b)
}

// Size classes for data segment buffer pooling. Tiers match common
// MaxRecvDataSegmentLength values: 4KB (small responses), 64KB (typical
// high-throughput MRDSL), 16MB (RFC 7143 maximum, 24-bit DS length field).
const (
	smallBufSize  = 4096    // <= 4KB: status responses, sense data
	mediumBufSize = 65536   // <= 64KB: default MRDSL range
	largeBufSize  = 1 << 24 // <= 16MB: RFC 7143 max data segment length
)

// SA6002: store *[]byte (pointer to slice header) in sync.Pool instead of
// []byte (slice header value). A []byte is a 3-word struct; passing it to
// Pool.Put boxes it into an interface, causing an allocation on every Put and
// defeating the pool's purpose. Storing a pointer avoids this allocation.
var (
	smallPool = sync.Pool{
		New: func() any {
			b := make([]byte, smallBufSize)
			return &b
		},
	}
	mediumPool = sync.Pool{
		New: func() any {
			b := make([]byte, mediumBufSize)
			return &b
		},
	}
	largePool = sync.Pool{
		New: func() any {
			b := make([]byte, largeBufSize)
			return &b
		},
	}
)

// GetBuffer returns a byte slice of at least size bytes from a size-class pool.
// The returned slice may be larger than requested. The caller must call PutBuffer
// when done. The returned slice is NOT zeroed.
func GetBuffer(size int) []byte {
	switch {
	case size <= smallBufSize:
		bp := smallPool.Get().(*[]byte)
		return (*bp)[:size]
	case size <= mediumBufSize:
		bp := mediumPool.Get().(*[]byte)
		return (*bp)[:size]
	case size <= largeBufSize:
		bp := largePool.Get().(*[]byte)
		return (*bp)[:size]
	default:
		// Oversized: allocate directly, not pooled.
		return make([]byte, size)
	}
}

// PutBuffer returns a buffer to the appropriate size-class pool.
// Buffers larger than the largest pool class are not returned.
func PutBuffer(b []byte) {
	c := cap(b)
	b = b[:c]
	switch {
	case c >= largeBufSize:
		largePool.Put(&b)
	case c >= mediumBufSize:
		mediumPool.Put(&b)
	case c >= smallBufSize:
		smallPool.Put(&b)
	// Smaller than smallest pool class: drop it.
	}
}
