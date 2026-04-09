//go:build e2e

// Package e2e_test contains end-to-end tests that exercise the uiscsi public
// API against a real Linux kernel LIO iSCSI target configured via configfs.
// These tests require root privileges and loaded LIO kernel modules.
//
// Run with:
//
//	sudo go test -tags e2e -v -count=1 ./test/e2e/
package e2e_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/uiscsi/uiscsi"
	"github.com/uiscsi/uiscsi/test/lio"
)

func TestMain(m *testing.M) {
	lio.SweepOrphans()
	os.Exit(m.Run())
}

// TestBasicConnectivity verifies the fundamental iSCSI workflow:
// Discover targets, Dial a session, issue Inquiry, ReadCapacity, and
// TestUnitReady commands, then close the session cleanly.
func TestBasicConnectivity(t *testing.T) {
	lio.RequireRoot(t)
	lio.RequireModules(t)

	tgt, cleanup := lio.Setup(t, lio.Config{
		TargetSuffix: "basic",
		InitiatorIQN: "iqn.2026-04.com.uiscsi.e2e:initiator",
	})
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Step 1: Discover targets.
	targets, err := uiscsi.Discover(ctx, tgt.Addr)
	if err != nil {
		t.Fatalf("Discover(%s): %v", tgt.Addr, err)
	}
	if len(targets) == 0 {
		t.Fatal("Discover returned no targets")
	}

	found := false
	for _, dt := range targets {
		if dt.Name == tgt.IQN {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Discover did not return expected target %s, got %v", tgt.IQN, targets)
	}

	// Step 2: Dial session.
	sess, err := uiscsi.Dial(ctx, tgt.Addr,
		uiscsi.WithTarget(tgt.IQN),
		uiscsi.WithInitiatorName("iqn.2026-04.com.uiscsi.e2e:initiator"),
	)
	if err != nil {
		t.Fatalf("Dial(%s): %v", tgt.Addr, err)
	}
	defer sess.Close()

	// Step 3: Inquiry.
	inq, err := sess.Inquiry(ctx, 0)
	if err != nil {
		t.Fatalf("Inquiry: %v", err)
	}
	if inq.VendorID == "" {
		t.Error("Inquiry VendorID is empty (LIO should report 'LIO-ORG')")
	}
	t.Logf("Inquiry: VendorID=%q ProductID=%q", inq.VendorID, inq.ProductID)

	// Step 4: ReadCapacity.
	cap, err := sess.ReadCapacity(ctx, 0)
	if err != nil {
		t.Fatalf("ReadCapacity: %v", err)
	}
	if cap.BlockSize == 0 {
		t.Error("ReadCapacity BlockSize is 0")
	}
	// 64MB LUN at 512-byte blocks = 131072 blocks = LBA 0..131071.
	expectedBlocks := uint64(64*1024*1024) / uint64(cap.BlockSize)
	if cap.LBA+1 != expectedBlocks {
		t.Errorf("ReadCapacity LBA=%d BlockSize=%d, expected %d blocks for 64MB LUN",
			cap.LBA, cap.BlockSize, expectedBlocks)
	}
	t.Logf("ReadCapacity: LBA=%d BlockSize=%d", cap.LBA, cap.BlockSize)

	// Step 5: TestUnitReady.
	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady: %v", err)
	}

	// Step 6: Explicit close.
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
