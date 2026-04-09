//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/uiscsi/uiscsi"
	"github.com/uiscsi/uiscsi/test/lio"
)

// TestLargeWrite_MultiR2T verifies that a 1MB write completes with data
// integrity when the payload exceeds MaxBurstLength (262144 bytes default),
// triggering multiple R2T sequences. This exercises the R2T/Data-Out engine
// under sustained multi-burst conditions per UNH-IOL Full Feature Phase tests.
func TestLargeWrite_MultiR2T(t *testing.T) {
	lio.RequireRoot(t)
	lio.RequireModules(t)

	tgt, cleanup := lio.Setup(t, lio.Config{
		TargetSuffix: "large",
		InitiatorIQN: initiatorIQN,
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr,
		uiscsi.WithTarget(tgt.IQN),
		uiscsi.WithInitiatorName(initiatorIQN),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer sess.Close()

	// Get block size from ReadCapacity.
	cap, err := sess.ReadCapacity(ctx, 0)
	if err != nil {
		t.Fatalf("ReadCapacity: %v", err)
	}
	if cap.BlockSize == 0 {
		t.Fatal("ReadCapacity returned BlockSize=0")
	}
	t.Logf("ReadCapacity: BlockSize=%d LBA=%d", cap.BlockSize, cap.LBA)

	// 1MB write: 2048 blocks at 512B/block = 1,048,576 bytes.
	// With default MaxBurstLength=262144, this triggers ~4 R2T sequences.
	var numBlocks uint32 = 2048
	testData := make([]byte, int(numBlocks)*int(cap.BlockSize))
	for i := range testData {
		testData[i] = byte(i % 251) // prime modulus for non-repeating pattern
	}

	if err := sess.WriteBlocks(ctx, 0, 0, numBlocks, cap.BlockSize, testData); err != nil {
		t.Fatalf("WriteBlocks(1MB): %v", err)
	}

	readBack, err := sess.ReadBlocks(ctx, 0, 0, numBlocks, cap.BlockSize)
	if err != nil {
		t.Fatalf("ReadBlocks(1MB): %v", err)
	}

	if !bytes.Equal(readBack, testData) {
		for i := range testData {
			if i >= len(readBack) {
				t.Errorf("read data too short: got %d bytes, want %d", len(readBack), len(testData))
				break
			}
			if readBack[i] != testData[i] {
				t.Errorf("data mismatch at offset %d: got 0x%02x, want 0x%02x", i, readBack[i], testData[i])
				break
			}
		}
		t.Fatal("1MB multi-R2T write: data integrity check failed")
	}
	t.Log("1MB multi-R2T write: data integrity OK")
}
