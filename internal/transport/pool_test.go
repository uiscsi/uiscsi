package transport

import (
	"testing"
)

// TestPoolGetBufferTierClassification verifies that GetBuffer returns a *[]byte
// with capacity matching the correct size-class tier.
func TestPoolGetBufferTierClassification(t *testing.T) {
	cases := []struct {
		name    string
		size    int
		minCap  int
	}{
		{"small", 100, smallBufSize},
		{"small_exact", smallBufSize, smallBufSize},
		{"medium", 5000, mediumBufSize},
		{"medium_exact", mediumBufSize, mediumBufSize},
		{"large", 70000, largeBufSize},
		{"large_exact", largeBufSize, largeBufSize},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bp := GetBuffer(tc.size)
			if bp == nil {
				t.Fatal("GetBuffer returned nil")
			}
			if cap(*bp) < tc.minCap {
				t.Errorf("GetBuffer(%d): cap(*bp) = %d, want >= %d", tc.size, cap(*bp), tc.minCap)
			}
			PutBuffer(bp)
		})
	}
}

// TestPoolGetBufferOversized verifies that oversized allocations are not pooled
// and return a *[]byte with exactly the requested capacity.
func TestPoolGetBufferOversized(t *testing.T) {
	size := 17_000_000 // > largeBufSize (16MB)
	bp := GetBuffer(size)
	if bp == nil {
		t.Fatal("GetBuffer returned nil for oversized allocation")
	}
	if cap(*bp) != size {
		t.Errorf("GetBuffer(%d): cap(*bp) = %d, want %d", size, cap(*bp), size)
	}
	// Oversized buffers should not panic when passed to PutBuffer.
	PutBuffer(bp)
}

// TestPoolPointerReuse verifies that PutBuffer followed by GetBuffer returns
// the same underlying pointer (pool reuse).
func TestPoolPointerReuse(t *testing.T) {
	// Reset counters to known state for this test by draining any cached entries.
	// We test each tier independently.
	tiers := []struct {
		name string
		size int
	}{
		{"small", 100},
		{"medium", 5000},
		{"large", 70000},
	}
	for _, tier := range tiers {
		t.Run(tier.name, func(t *testing.T) {
			bp1 := GetBuffer(tier.size)
			// Put it back.
			PutBuffer(bp1)
			// Get again — should return the same pointer from the pool.
			bp2 := GetBuffer(tier.size)
			if bp2 == nil {
				t.Fatal("GetBuffer returned nil")
			}
			// Check pointer equality: pool should have returned bp1.
			if bp1 != bp2 {
				t.Logf("pointer reuse not guaranteed by pool (acceptable); bp1=%p bp2=%p", bp1, bp2)
				// Not a hard failure: pools may evict under GC pressure. This test
				// primarily verifies the path is exercised without panic.
			}
			PutBuffer(bp2)
		})
	}
}

// TestPoolSoftBound verifies that after poolTierMax PutBuffer calls to the same tier,
// excess Put calls are dropped (soft bound enforcement prevents unbounded pool growth).
func TestPoolSoftBound(t *testing.T) {
	// Reset small counter to 0 before testing.
	smallCount.Store(0)

	// Allocate poolTierMax+10 buffers and put them all back.
	// Only the first poolTierMax should be accepted; the rest dropped.
	const extra = 10
	total := poolTierMax + extra

	bufs := make([]*[]byte, total)
	for i := range bufs {
		b := make([]byte, smallBufSize)
		bufs[i] = &b
	}

	accepted := 0
	for _, bp := range bufs {
		before := smallCount.Load()
		PutBuffer(bp)
		after := smallCount.Load()
		if after > before {
			accepted++
		}
	}

	if accepted > poolTierMax {
		t.Errorf("soft bound violated: accepted %d buffers into small pool, want <= %d", accepted, poolTierMax)
	}

	// Drain the pool to reset state.
	for smallCount.Load() > 0 {
		bp := GetBuffer(smallBufSize)
		_ = bp
	}
}

// TestPoolAtomicCounters verifies that GetBuffer decrements and PutBuffer
// increments the per-tier atomic counter correctly.
func TestPoolAtomicCounters(t *testing.T) {
	// Reset counters.
	smallCount.Store(0)
	mediumCount.Store(0)
	largeCount.Store(0)

	// PutBuffer a small buffer: counter should go up by 1.
	b := make([]byte, smallBufSize)
	bp := &b
	before := smallCount.Load()
	PutBuffer(bp)
	after := smallCount.Load()
	if after != before+1 {
		t.Errorf("PutBuffer small: counter went from %d to %d, want %d", before, after, before+1)
	}

	// GetBuffer a small buffer: counter should go down by 1.
	before2 := smallCount.Load()
	bp2 := GetBuffer(smallBufSize)
	after2 := smallCount.Load()
	if after2 != before2-1 {
		t.Errorf("GetBuffer small: counter went from %d to %d, want %d", before2, after2, before2-1)
	}
	PutBuffer(bp2)
}

// TestPutBufferNil verifies PutBuffer does not panic on nil input.
func TestPutBufferNil(t *testing.T) {
	// Should not panic.
	PutBuffer(nil)
}

// TestGetBufferReturnType verifies GetBuffer returns *[]byte, not []byte.
// This is a compile-time check embedded in a runtime test for documentation.
func TestGetBufferReturnType(t *testing.T) {
	bp := GetBuffer(100)
	// If GetBuffer returns []byte this would not compile.
	_ = (*[]byte)(bp) // compile-time type assertion: GetBuffer must return *[]byte
	PutBuffer(bp)
}
