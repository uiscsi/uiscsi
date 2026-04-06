package conformance_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/rkujawa/uiscsi"
	testutil "github.com/rkujawa/uiscsi/test"
)

// setupFullFeatureTarget creates a MockTarget configured with login, logout,
// and SCSI command handlers using the provided data.
func setupFullFeatureTarget(t *testing.T, data []byte) (*testutil.MockTarget, *uiscsi.Session) {
	t.Helper()
	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()
	tgt.HandleSCSIRead(0, data)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	return tgt, sess
}

// TestRead_SingleBlock tests reading a single 512-byte block.
// IOL: Full-Feature Phase - Single Block Read.
func TestRead_SingleBlock(t *testing.T) {
	blockData := make([]byte, 512)
	for i := range blockData {
		blockData[i] = byte(i % 256)
	}

	_, sess := setupFullFeatureTarget(t, blockData)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	data, err := sess.ReadBlocks(ctx, 0, 0, 1, 512)
	if err != nil {
		t.Fatalf("ReadBlocks: %v", err)
	}
	if len(data) != 512 {
		t.Fatalf("len(data): got %d, want 512", len(data))
	}
	for i := range data {
		if data[i] != byte(i%256) {
			t.Fatalf("data[%d]: got 0x%02X, want 0x%02X", i, data[i], byte(i%256))
		}
	}
}

// TestRead_MultiBlock tests reading multiple blocks.
// IOL: Full-Feature Phase - Multi Block Read.
func TestRead_MultiBlock(t *testing.T) {
	blockData := make([]byte, 2048)
	for i := range blockData {
		blockData[i] = byte(i % 256)
	}

	_, sess := setupFullFeatureTarget(t, blockData)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	data, err := sess.ReadBlocks(ctx, 0, 0, 4, 512)
	if err != nil {
		t.Fatalf("ReadBlocks: %v", err)
	}
	if len(data) != 2048 {
		t.Fatalf("len(data): got %d, want 2048", len(data))
	}
}

// TestWrite_SingleBlock tests writing a single block.
// IOL: Full-Feature Phase - Single Block Write.
func TestWrite_SingleBlock(t *testing.T) {
	// For write, the mock accepts immediate data and returns success.
	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	defer tgt.Close()

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleSCSIWrite(0)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer sess.Close()

	writeData := make([]byte, 512)
	for i := range writeData {
		writeData[i] = byte(i % 256)
	}

	if err := sess.WriteBlocks(ctx, 0, 0, 1, 512, writeData); err != nil {
		t.Fatalf("WriteBlocks: %v", err)
	}
}

// TestInquiry tests the INQUIRY command returns valid InquiryData.
// IOL: Full-Feature Phase - INQUIRY.
func TestInquiry(t *testing.T) {
	inquiryData := testutil.BuildInquiryData("UISCSI", "MOCKTARGET", "1.0")
	_, sess := setupFullFeatureTarget(t, inquiryData)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	inq, err := sess.Inquiry(ctx, 0)
	if err != nil {
		t.Fatalf("Inquiry: %v", err)
	}
	if inq.VendorID == "" {
		t.Fatal("VendorID is empty")
	}
	if inq.ProductID == "" {
		t.Fatal("ProductID is empty")
	}
}

// TestReadCapacity tests READ CAPACITY(16) returns valid capacity data.
// IOL: Full-Feature Phase - READ CAPACITY.
func TestReadCapacity(t *testing.T) {
	capData := testutil.BuildReadCapacity16Data(1048575, 512) // ~512MB
	_, sess := setupFullFeatureTarget(t, capData)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cap, err := sess.ReadCapacity(ctx, 0)
	if err != nil {
		t.Fatalf("ReadCapacity: %v", err)
	}
	if cap.LBA == 0 {
		t.Fatal("LBA is 0")
	}
	if cap.BlockSize == 0 {
		t.Fatal("BlockSize is 0")
	}
	if cap.LogicalBlocks == 0 {
		t.Fatal("LogicalBlocks is 0")
	}
}

// TestTestUnitReady tests TEST UNIT READY returns nil on success.
// IOL: Full-Feature Phase - TEST UNIT READY.
func TestTestUnitReady(t *testing.T) {
	_, sess := setupFullFeatureTarget(t, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady: %v", err)
	}
}

// TestReportLuns tests REPORT LUNS returns at least LUN 0.
// IOL: Full-Feature Phase - REPORT LUNS.
func TestReportLuns(t *testing.T) {
	lunsData := testutil.BuildReportLunsData([]uint64{0})
	_, sess := setupFullFeatureTarget(t, lunsData)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	luns, err := sess.ReportLuns(ctx)
	if err != nil {
		t.Fatalf("ReportLuns: %v", err)
	}
	if len(luns) == 0 {
		t.Fatal("no LUNs returned")
	}
}

// TestExecute_RawCDB tests raw CDB pass-through with TEST UNIT READY.
// IOL: Full-Feature Phase - Raw CDB execution.
func TestExecute_RawCDB(t *testing.T) {
	_, sess := setupFullFeatureTarget(t, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// TEST UNIT READY CDB: opcode 0x00, 6 bytes.
	turCDB := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}

	result, err := sess.Execute(ctx, 0, turCDB)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Status != 0x00 {
		t.Fatalf("Status: got 0x%02X, want 0x00", result.Status)
	}
}

// TestExecute_RawRead tests raw CDB with a read command.
func TestExecute_RawRead(t *testing.T) {
	readData := make([]byte, 512)
	for i := range readData {
		readData[i] = 0xAB
	}
	_, sess := setupFullFeatureTarget(t, readData)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// READ(10) CDB.
	readCDB := make([]byte, 10)
	readCDB[0] = 0x28 // READ(10) opcode

	result, err := sess.Execute(ctx, 0, readCDB, uiscsi.WithDataIn(512))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(result.Data) != 512 {
		t.Fatalf("data len: got %d, want 512", len(result.Data))
	}
}

// TestStreamExecuteRead tests StreamExecute returns a streaming io.Reader
// that yields correct data with bounded memory.
// IOL: Full-Feature Phase - Streaming Read.
func TestStreamExecuteRead(t *testing.T) {
	blockData := make([]byte, 512)
	for i := range blockData {
		blockData[i] = byte(i % 256)
	}
	_, sess := setupFullFeatureTarget(t, blockData)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build a READ(16) CDB manually to exercise StreamExecute.
	cdb := make([]byte, 16)
	cdb[0] = 0x88 // READ(16) opcode
	// LBA = 0 (bytes 2-9), transfer length = 1 block (bytes 10-13)
	cdb[13] = 1

	sr, err := sess.StreamExecute(ctx, 0, cdb, uiscsi.WithDataIn(512))
	if err != nil {
		t.Fatalf("StreamExecute: %v", err)
	}

	data, err := io.ReadAll(sr.Data)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(data) != 512 {
		t.Fatalf("len(data): got %d, want 512", len(data))
	}

	status, _, waitErr := sr.Wait()
	if waitErr != nil {
		t.Fatalf("Wait: %v", waitErr)
	}
	if status != 0 {
		t.Fatalf("status: got 0x%02X, want 0x00", status)
	}
}

// TestContextTimeout tests that a very short context timeout causes an error.
func TestContextTimeout(t *testing.T) {
	// Use a target that's reachable but with tiny timeout for the command.
	blockData := make([]byte, 512)
	_, sess := setupFullFeatureTarget(t, blockData)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	// Let the context expire.
	<-ctx.Done()

	_, err := sess.ReadBlocks(ctx, 0, 0, 1, 512)
	if err == nil {
		t.Fatal("expected error for expired context")
	}
}
