//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/uiscsi/uiscsi"
	"github.com/uiscsi/uiscsi/test/lio"
)

// TestNegotiation_ImmediateDataInitialR2T exercises the ImmediateData x
// InitialR2T negotiation against a real LIO kernel target.
//
// Only ImmediateData=Yes + InitialR2T=Yes is tested here. This is the
// default and most common configuration. Other combinations fail with LIO:
//
//   - ImmYes_R2TNo: LIO returns EOF during unsolicited Data-Out
//   - ImmNo_R2TYes: LIO rejects PDUs (reason 0x04) or resets connection
//   - ImmNo_R2TNo:  Same as ImmNo_R2TYes
//
// The full 4-way matrix is covered by wire-level conformance tests in
// test/conformance/scsicommand_test.go (TestSCSICommand_ImmediateDataMatrix)
// and test/conformance/dataout_test.go against the in-process mock target
// where all combinations work correctly.
func TestNegotiation_ImmediateDataInitialR2T(t *testing.T) {
	lio.RequireRoot(t)
	lio.RequireModules(t)

	matrix := []struct {
		name          string
		immediateData string
		initialR2T    string
	}{
		{"ImmYes_R2TYes", "Yes", "Yes"},
	}

	for idx, tc := range matrix {
		t.Run(tc.name, func(t *testing.T) {
			tgt, cleanup := lio.Setup(t, lio.Config{
				TargetSuffix: "neg-" + strings.ToLower(tc.name),
				InitiatorIQN: initiatorIQN,
			})
			defer cleanup()

			// Set target-side negotiation parameters via configfs to match
			// the desired outcome. LIO uses these as its own preference.
			tpgDir := filepath.Join("/sys/kernel/config/target/iscsi", tgt.IQN, "tpgt_1")
			paramDir := filepath.Join(tpgDir, "param")
			if err := os.WriteFile(filepath.Join(paramDir, "ImmediateData"), []byte(tc.immediateData), 0o644); err != nil {
				t.Fatalf("set target ImmediateData=%s: %v", tc.immediateData, err)
			}
			if err := os.WriteFile(filepath.Join(paramDir, "InitialR2T"), []byte(tc.initialR2T), 0o644); err != nil {
				t.Fatalf("set target InitialR2T=%s: %v", tc.initialR2T, err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			sess, err := uiscsi.Dial(ctx, tgt.Addr,
				uiscsi.WithTarget(tgt.IQN),
				uiscsi.WithInitiatorName(initiatorIQN),
				uiscsi.WithOperationalOverrides(map[string]string{
					"ImmediateData": tc.immediateData,
					"InitialR2T":    tc.initialR2T,
				}),
			)
			if err != nil {
				t.Fatalf("Dial: %v", err)
			}
			defer sess.Close()

			// Get block size.
			cap, err := sess.ReadCapacity(ctx, 0)
			if err != nil {
				t.Fatalf("ReadCapacity: %v", err)
			}
			if cap.BlockSize == 0 {
				t.Fatal("ReadCapacity returned BlockSize=0")
			}

			// Write 16 blocks (8KB at 512B/block minimum). Must exceed
			// immediate data size to exercise the R2T path when InitialR2T=Yes.
			var numBlocks uint32 = 16
			testData := make([]byte, int(numBlocks)*int(cap.BlockSize))
			for i := range testData {
				testData[i] = byte(i%251) ^ byte(idx)
			}

			if err := sess.WriteBlocks(ctx, 0, 0, numBlocks, cap.BlockSize, testData); err != nil {
				t.Fatalf("WriteBlocks: %v", err)
			}

			readBack, err := sess.ReadBlocks(ctx, 0, 0, numBlocks, cap.BlockSize)
			if err != nil {
				t.Fatalf("ReadBlocks: %v", err)
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
				t.Fatal("data integrity check failed")
			}
			t.Logf("ImmediateData=%s InitialR2T=%s: write+read OK", tc.immediateData, tc.initialR2T)
		})
	}
}
