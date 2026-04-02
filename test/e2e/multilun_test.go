//go:build e2e

package e2e_test

import (
	"context"
	"testing"
	"time"

	"github.com/rkujawa/uiscsi"
	"github.com/rkujawa/uiscsi/test/lio"
)

// TestMultiLUN verifies multi-LUN enumeration via ReportLuns and validates
// each LUN's capacity via ReadCapacity against a real LIO target configured
// with 3 LUNs of different sizes (32MB, 64MB, 128MB).
func TestMultiLUN(t *testing.T) {
	lio.RequireRoot(t)
	lio.RequireModules(t)

	tgt, cleanup := lio.Setup(t, lio.Config{
		TargetSuffix: "multilun",
		InitiatorIQN: initiatorIQN,
		LUNs:         []int64{32 * 1024 * 1024, 64 * 1024 * 1024, 128 * 1024 * 1024},
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr,
		uiscsi.WithTarget(tgt.IQN),
		uiscsi.WithInitiatorName(initiatorIQN),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer sess.Close()

	// Enumerate LUNs.
	luns, err := sess.ReportLuns(ctx)
	if err != nil {
		t.Fatalf("ReportLuns: %v", err)
	}
	t.Logf("ReportLuns returned %d LUNs: %v", len(luns), luns)

	if len(luns) < 3 {
		t.Fatalf("expected at least 3 LUNs, got %d: %v", len(luns), luns)
	}

	// Verify each expected LUN (0, 1, 2) is present.
	expectedLUNs := []uint64{0, 1, 2}
	for _, want := range expectedLUNs {
		found := false
		for _, got := range luns {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("LUN %d not found in ReportLuns result %v", want, luns)
		}
	}

	// Verify capacities match configured sizes (within block alignment).
	expectedSizes := []int64{32 * 1024 * 1024, 64 * 1024 * 1024, 128 * 1024 * 1024}
	for i, lunID := range expectedLUNs {
		cap, err := sess.ReadCapacity(ctx, lunID)
		if err != nil {
			t.Errorf("ReadCapacity(LUN %d): %v", lunID, err)
			continue
		}
		if cap.BlockSize == 0 {
			t.Errorf("LUN %d: BlockSize is 0", lunID)
			continue
		}

		// Total capacity = (LBA + 1) * BlockSize.
		totalBytes := (cap.LBA + 1) * uint64(cap.BlockSize)
		expectedBytes := uint64(expectedSizes[i])

		t.Logf("LUN %d: LBA=%d BlockSize=%d TotalBytes=%d ExpectedBytes=%d",
			lunID, cap.LBA, cap.BlockSize, totalBytes, expectedBytes)

		if totalBytes != expectedBytes {
			t.Errorf("LUN %d: capacity %d bytes, expected %d bytes", lunID, totalBytes, expectedBytes)
		}

		// Inquiry each LUN.
		inq, err := sess.Inquiry(ctx, lunID)
		if err != nil {
			t.Errorf("Inquiry(LUN %d): %v", lunID, err)
			continue
		}
		t.Logf("LUN %d: VendorID=%q ProductID=%q", lunID, inq.VendorID, inq.ProductID)
	}
}
