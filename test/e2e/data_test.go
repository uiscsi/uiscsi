//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"testing"
	"time"

	"github.com/rkujawa/uiscsi"
	"github.com/rkujawa/uiscsi/test/lio"
)

const initiatorIQN = "iqn.2026-04.com.uiscsi.e2e:initiator"

// TestDataIntegrity verifies write-then-read byte-for-byte data integrity
// against a real LIO iSCSI target. It writes a recognizable pattern at LBA 0,
// reads it back, then repeats at a non-zero LBA to verify offset handling.
func TestDataIntegrity(t *testing.T) {
	lio.RequireRoot(t)
	lio.RequireModules(t)

	tgt, cleanup := lio.Setup(t, lio.Config{
		TargetSuffix: "data",
		InitiatorIQN: initiatorIQN,
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

	// Get block size from ReadCapacity.
	cap, err := sess.ReadCapacity(ctx, 0)
	if err != nil {
		t.Fatalf("ReadCapacity: %v", err)
	}
	if cap.BlockSize == 0 {
		t.Fatal("ReadCapacity returned BlockSize=0")
	}
	blockSize := cap.BlockSize
	t.Logf("ReadCapacity: BlockSize=%d LBA=%d", blockSize, cap.LBA)

	// Create test pattern: 8 blocks with block-index encoding.
	const numBlocks = 8
	testData := make([]byte, numBlocks*int(blockSize))
	for i := range testData {
		// Encode block index in high nibble, byte offset in low byte.
		blockIdx := i / int(blockSize)
		testData[i] = byte((blockIdx << 4) | (i & 0x0F))
	}

	// Write 8 blocks at LBA 0.
	if err := sess.WriteBlocks(ctx, 0, 0, numBlocks, blockSize, testData); err != nil {
		t.Fatalf("WriteBlocks(LBA=0): %v", err)
	}

	// Read back 8 blocks at LBA 0.
	readBack, err := sess.ReadBlocks(ctx, 0, 0, numBlocks, blockSize)
	if err != nil {
		t.Fatalf("ReadBlocks(LBA=0): %v", err)
	}

	if !bytes.Equal(readBack, testData) {
		// Find first differing offset for diagnostic.
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
		t.Fatal("data integrity check failed at LBA 0")
	}
	t.Log("LBA 0: write-then-read byte-for-byte match OK")

	// Test at non-zero LBA to verify offset handling.
	const offsetLBA = 100
	offsetData := make([]byte, numBlocks*int(blockSize))
	for i := range offsetData {
		offsetData[i] = byte(0xAA ^ byte(i&0xFF))
	}

	if err := sess.WriteBlocks(ctx, 0, offsetLBA, numBlocks, blockSize, offsetData); err != nil {
		t.Fatalf("WriteBlocks(LBA=%d): %v", offsetLBA, err)
	}

	readOffset, err := sess.ReadBlocks(ctx, 0, offsetLBA, numBlocks, blockSize)
	if err != nil {
		t.Fatalf("ReadBlocks(LBA=%d): %v", offsetLBA, err)
	}

	if !bytes.Equal(readOffset, offsetData) {
		for i := range offsetData {
			if i >= len(readOffset) {
				t.Errorf("read data too short at LBA %d: got %d bytes, want %d", offsetLBA, len(readOffset), len(offsetData))
				break
			}
			if readOffset[i] != offsetData[i] {
				t.Errorf("data mismatch at LBA %d offset %d: got 0x%02x, want 0x%02x", offsetLBA, i, readOffset[i], offsetData[i])
				break
			}
		}
		t.Fatalf("data integrity check failed at LBA %d", offsetLBA)
	}
	t.Logf("LBA %d: write-then-read byte-for-byte match OK", offsetLBA)
}

// TestStreamExecuteDataIntegrity verifies that StreamExecute delivers data
// byte-for-byte identical to ReadBlocks, exercising the bounded-memory
// chanReader path through a real LIO kernel iSCSI target with real TCP,
// real PDU framing, and real MRDSL negotiation.
func TestStreamExecuteDataIntegrity(t *testing.T) {
	lio.RequireRoot(t)
	lio.RequireModules(t)

	tgt, cleanup := lio.Setup(t, lio.Config{
		TargetSuffix: "stream",
		InitiatorIQN: initiatorIQN,
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

	cap, err := sess.ReadCapacity(ctx, 0)
	if err != nil {
		t.Fatalf("ReadCapacity: %v", err)
	}
	blockSize := cap.BlockSize
	t.Logf("ReadCapacity: BlockSize=%d LBA=%d", blockSize, cap.LBA)

	// Write a recognizable pattern via WriteBlocks (known-good path).
	const numBlocks = 16
	testData := make([]byte, numBlocks*int(blockSize))
	for i := range testData {
		testData[i] = byte((i * 7) ^ 0xA5)
	}

	if err := sess.WriteBlocks(ctx, 0, 0, numBlocks, blockSize, testData); err != nil {
		t.Fatalf("WriteBlocks: %v", err)
	}

	// Read back via StreamExecute with a raw READ(16) CDB.
	cdb := make([]byte, 16)
	cdb[0] = 0x88 // READ(16)
	// LBA = 0 (bytes 2-9 are zero)
	// Transfer length = numBlocks (bytes 10-13, big-endian uint32)
	binary.BigEndian.PutUint32(cdb[10:14], numBlocks)

	totalBytes := uint32(numBlocks) * blockSize
	sr, err := sess.StreamExecute(ctx, 0, cdb, uiscsi.WithDataIn(totalBytes))
	if err != nil {
		t.Fatalf("StreamExecute: %v", err)
	}

	// Stream data through the chanReader in small chunks to exercise
	// the bounded-memory path across multiple PDUs.
	var got []byte
	buf := make([]byte, 4096)
	for {
		n, readErr := sr.Data.Read(buf)
		got = append(got, buf[:n]...)
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			t.Fatalf("StreamExecute Read: %v", readErr)
		}
	}

	status, sense, waitErr := sr.Wait()
	if waitErr != nil {
		t.Fatalf("Wait: %v", waitErr)
	}
	if status != 0 {
		if err := uiscsi.CheckStatus(status, sense); err != nil {
			t.Fatalf("CheckStatus: %v", err)
		}
	}

	if !bytes.Equal(got, testData) {
		t.Errorf("StreamExecute returned %d bytes, want %d", len(got), len(testData))
		for i := range testData {
			if i >= len(got) {
				t.Errorf("stream data too short at offset %d", i)
				break
			}
			if got[i] != testData[i] {
				t.Errorf("stream mismatch at offset %d: got 0x%02x, want 0x%02x", i, got[i], testData[i])
				break
			}
		}
		t.Fatal("StreamExecute data integrity check failed")
	}
	t.Logf("StreamExecute: %d bytes streamed, byte-for-byte match OK", len(got))
}

// TestStreamExecuteWriteRead verifies that StreamExecute works for both
// write and read directions against a real LIO target. Writes data via
// StreamExecute + WithDataOut, reads it back via StreamExecute + WithDataIn,
// and compares byte-for-byte.
func TestStreamExecuteWriteRead(t *testing.T) {
	lio.RequireRoot(t)
	lio.RequireModules(t)

	tgt, cleanup := lio.Setup(t, lio.Config{
		TargetSuffix: "streamwr",
		InitiatorIQN: initiatorIQN,
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

	cap, err := sess.ReadCapacity(ctx, 0)
	if err != nil {
		t.Fatalf("ReadCapacity: %v", err)
	}
	blockSize := cap.BlockSize

	const numBlocks = 8
	totalBytes := uint32(numBlocks) * blockSize

	// Build test pattern.
	testData := make([]byte, totalBytes)
	for i := range testData {
		testData[i] = byte((i * 13) ^ 0x5A)
	}

	// Write via StreamExecute + WithDataOut using raw WRITE(16) CDB.
	writeCDB := make([]byte, 16)
	writeCDB[0] = 0x8A // WRITE(16)
	binary.BigEndian.PutUint32(writeCDB[10:14], numBlocks)

	wsr, err := sess.StreamExecute(ctx, 0, writeCDB,
		uiscsi.WithDataOut(bytes.NewReader(testData), totalBytes),
	)
	if err != nil {
		t.Fatalf("StreamExecute write: %v", err)
	}

	wStatus, wSense, wErr := wsr.Wait()
	if wErr != nil {
		t.Fatalf("write Wait: %v", wErr)
	}
	if wStatus != 0 {
		if err := uiscsi.CheckStatus(wStatus, wSense); err != nil {
			t.Fatalf("write CheckStatus: %v", err)
		}
	}
	t.Log("StreamExecute write: OK")

	// Read back via StreamExecute + WithDataIn using raw READ(16) CDB.
	readCDB := make([]byte, 16)
	readCDB[0] = 0x88 // READ(16)
	binary.BigEndian.PutUint32(readCDB[10:14], numBlocks)

	rsr, err := sess.StreamExecute(ctx, 0, readCDB, uiscsi.WithDataIn(totalBytes))
	if err != nil {
		t.Fatalf("StreamExecute read: %v", err)
	}

	got, readErr := io.ReadAll(rsr.Data)
	if readErr != nil {
		t.Fatalf("StreamExecute Read: %v", readErr)
	}

	rStatus, rSense, rErr := rsr.Wait()
	if rErr != nil {
		t.Fatalf("read Wait: %v", rErr)
	}
	if rStatus != 0 {
		if err := uiscsi.CheckStatus(rStatus, rSense); err != nil {
			t.Fatalf("read CheckStatus: %v", err)
		}
	}

	if !bytes.Equal(got, testData) {
		t.Errorf("got %d bytes, want %d", len(got), len(testData))
		for i := range testData {
			if i >= len(got) {
				break
			}
			if got[i] != testData[i] {
				t.Errorf("mismatch at offset %d: got 0x%02x, want 0x%02x", i, got[i], testData[i])
				break
			}
		}
		t.Fatal("StreamExecute write+read integrity check failed")
	}
	t.Logf("StreamExecute write+read: %d bytes, byte-for-byte match OK", len(got))
}
