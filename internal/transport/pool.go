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

// Size classes for data segment buffer pooling.
const (
	smallBufSize  = 4096    // <= 4KB
	mediumBufSize = 65536   // <= 64KB
	largeBufSize  = 1 << 24 // <= 16MB
)

var (
	smallPool = sync.Pool{
		New: func() any { return make([]byte, smallBufSize) },
	}
	mediumPool = sync.Pool{
		New: func() any { return make([]byte, mediumBufSize) },
	}
	largePool = sync.Pool{
		New: func() any { return make([]byte, largeBufSize) },
	}
)

// GetBuffer returns a byte slice of at least size bytes from a size-class pool.
// The returned slice may be larger than requested. The caller must call PutBuffer
// when done. The returned slice is NOT zeroed.
func GetBuffer(size int) []byte {
	switch {
	case size <= smallBufSize:
		buf := smallPool.Get().([]byte)
		return buf[:size]
	case size <= mediumBufSize:
		buf := mediumPool.Get().([]byte)
		return buf[:size]
	case size <= largeBufSize:
		buf := largePool.Get().([]byte)
		return buf[:size]
	default:
		// Oversized: allocate directly, not pooled.
		return make([]byte, size)
	}
}

// PutBuffer returns a buffer to the appropriate size-class pool.
// Buffers larger than the largest pool class are not returned.
func PutBuffer(b []byte) {
	c := cap(b)
	switch {
	case c >= largeBufSize:
		largePool.Put(b[:c])
	case c >= mediumBufSize:
		mediumPool.Put(b[:c])
	case c >= smallBufSize:
		smallPool.Put(b[:c])
	// Smaller than smallest pool class: drop it.
	}
}
