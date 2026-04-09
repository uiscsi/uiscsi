//go:build e2e

package e2e_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/uiscsi/uiscsi"
	"github.com/uiscsi/uiscsi/test/lio"
)

// TestSCSIError_OutOfRangeLBA verifies that writing to an LBA beyond the LUN
// capacity returns a SCSIError with ILLEGAL_REQUEST sense key (0x05) and
// ASC/ASCQ 0x21/0x00 ("Logical block address out of range"). This covers
// UNH-IOL compliance for SCSI CHECK CONDITION handling.
func TestSCSIError_OutOfRangeLBA(t *testing.T) {
	lio.RequireRoot(t)
	lio.RequireModules(t)

	tgt, cleanup := lio.Setup(t, lio.Config{
		TargetSuffix: "scsierr",
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

	// Write 1 block of zeroes to LBA 200000, well beyond the 64MB LUN
	// capacity of 131072 blocks at 512 bytes.
	oneBlock := make([]byte, 512)
	err = sess.WriteBlocks(ctx, 0, 200000, 1, 512, oneBlock)
	if err == nil {
		t.Fatal("expected error for out-of-range LBA, got nil")
	}

	var scsiErr *uiscsi.SCSIError
	if !errors.As(err, &scsiErr) {
		t.Fatalf("expected *uiscsi.SCSIError, got %T: %v", err, err)
	}

	if scsiErr.SenseKey != 0x05 {
		t.Errorf("SenseKey: got 0x%02X, want 0x05 (ILLEGAL_REQUEST)", scsiErr.SenseKey)
	}

	if scsiErr.ASC != 0x21 || scsiErr.ASCQ != 0x00 {
		t.Errorf("ASC/ASCQ: got 0x%02X/0x%02X, want 0x21/0x00", scsiErr.ASC, scsiErr.ASCQ)
	}

	// Verify human-readable error message includes sense key name.
	errMsg := scsiErr.Error()
	if !strings.Contains(errMsg, "ILLEGAL REQUEST") {
		t.Errorf("error message should contain 'ILLEGAL REQUEST': %s", errMsg)
	}

	t.Logf("Out-of-range LBA: got expected SCSIError: %v", scsiErr)
}

// TestSCSIError_SenseDataParsing verifies that sense data is properly
// extracted from a CHECK CONDITION response. It reads from an out-of-range
// LBA and confirms that SenseKey is non-zero and Message is non-empty,
// proving the library parses sense data rather than returning just a status byte.
func TestSCSIError_SenseDataParsing(t *testing.T) {
	lio.RequireRoot(t)
	lio.RequireModules(t)

	tgt, cleanup := lio.Setup(t, lio.Config{
		TargetSuffix: "sense",
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

	// Read 1 block from LBA 200000, beyond the 64MB LUN capacity.
	_, err = sess.ReadBlocks(ctx, 0, 200000, 1, 512)
	if err == nil {
		t.Fatal("expected error for out-of-range LBA read, got nil")
	}

	var scsiErr *uiscsi.SCSIError
	if !errors.As(err, &scsiErr) {
		t.Fatalf("expected *uiscsi.SCSIError, got %T: %v", err, err)
	}

	if scsiErr.SenseKey == 0 {
		t.Error("SenseKey is 0x00 (NO SENSE); expected non-zero for out-of-range LBA")
	}

	if scsiErr.Message == "" {
		t.Error("Message is empty; expected parsed sense data description")
	}

	t.Logf("Sense data parsing: SenseKey=0x%02X ASC=0x%02X ASCQ=0x%02X Message=%q",
		scsiErr.SenseKey, scsiErr.ASC, scsiErr.ASCQ, scsiErr.Message)
}
