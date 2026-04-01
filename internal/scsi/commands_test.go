package scsi

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/rkujawa/uiscsi/internal/session"
)

func TestTestUnitReady(t *testing.T) {
	cmd := TestUnitReady(3)

	// CDB: opcode 0x00, bytes 1-5 all zero
	wantCDB := [16]byte{OpTestUnitReady}
	if cmd.CDB != wantCDB {
		t.Errorf("CDB = %X, want %X", cmd.CDB, wantCDB)
	}
	if cmd.Read {
		t.Error("Read should be false")
	}
	if cmd.Write {
		t.Error("Write should be false")
	}
	if cmd.ExpectedDataTransferLen != 0 {
		t.Errorf("ExpectedDataTransferLen = %d, want 0", cmd.ExpectedDataTransferLen)
	}
	if cmd.LUN != 3 {
		t.Errorf("LUN = %d, want 3", cmd.LUN)
	}
}

func TestRequestSense(t *testing.T) {
	cmd := RequestSense(1, 252)

	if cmd.CDB[0] != OpRequestSense {
		t.Errorf("CDB[0] = 0x%02X, want 0x%02X", cmd.CDB[0], OpRequestSense)
	}
	if cmd.CDB[4] != 252 {
		t.Errorf("CDB[4] = %d, want 252 (allocation length)", cmd.CDB[4])
	}
	if !cmd.Read {
		t.Error("Read should be true")
	}
	if cmd.Write {
		t.Error("Write should be false")
	}
	if cmd.ExpectedDataTransferLen != 252 {
		t.Errorf("ExpectedDataTransferLen = %d, want 252", cmd.ExpectedDataTransferLen)
	}
	if cmd.LUN != 1 {
		t.Errorf("LUN = %d, want 1", cmd.LUN)
	}
}

func TestReportLuns(t *testing.T) {
	cmd := ReportLuns(1024)

	if cmd.CDB[0] != OpReportLuns {
		t.Errorf("CDB[0] = 0x%02X, want 0x%02X", cmd.CDB[0], OpReportLuns)
	}
	// Allocation length in bytes 6-9
	allocLen := binary.BigEndian.Uint32(cmd.CDB[6:10])
	if allocLen != 1024 {
		t.Errorf("allocation length = %d, want 1024", allocLen)
	}
	if !cmd.Read {
		t.Error("Read should be true")
	}
	if cmd.LUN != 0 {
		t.Errorf("LUN = %d, want 0 (REPORT LUNS targets the target)", cmd.LUN)
	}
	if cmd.ExpectedDataTransferLen != 1024 {
		t.Errorf("ExpectedDataTransferLen = %d, want 1024", cmd.ExpectedDataTransferLen)
	}
}

func TestParseReportLuns(t *testing.T) {
	tests := []struct {
		name    string
		result  session.Result
		want    []uint64
		wantErr bool
	}{
		{
			name: "two LUNs",
			result: session.Result{
				Status: StatusGood,
				Data: bytes.NewReader(func() []byte {
					// 4-byte LUN list length (16 = 2 LUNs * 8 bytes each)
					// 4-byte reserved
					// LUN 0: 0x0000000000000000
					// LUN 1: 0x0001000000000000
					b := make([]byte, 24)
					binary.BigEndian.PutUint32(b[0:4], 16) // length
					// LUN 0 at offset 8
					binary.BigEndian.PutUint64(b[8:16], 0)
					// LUN 1 at offset 16
					binary.BigEndian.PutUint64(b[16:24], 0x0001000000000000)
					return b
				}()),
			},
			want: []uint64{0, 0x0001000000000000},
		},
		{
			name: "empty list",
			result: session.Result{
				Status: StatusGood,
				Data: bytes.NewReader(func() []byte {
					b := make([]byte, 8)
					binary.BigEndian.PutUint32(b[0:4], 0) // length = 0
					return b
				}()),
			},
			want: []uint64{},
		},
		{
			name: "short data",
			result: session.Result{
				Status: StatusGood,
				Data:   bytes.NewReader([]byte{0x00, 0x00}),
			},
			wantErr: true,
		},
		{
			name: "check condition",
			result: session.Result{
				Status: StatusCheckCondition,
				SenseData: []byte{
					0x70, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00, 0x0A,
					0x00, 0x00, 0x00, 0x00, 0x20, 0x00, 0x00, 0x00,
					0x00, 0x00,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			luns, err := ParseReportLuns(tt.result)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(luns) != len(tt.want) {
				t.Fatalf("got %d LUNs, want %d", len(luns), len(tt.want))
			}
			for i, lun := range luns {
				if lun != tt.want[i] {
					t.Errorf("LUN[%d] = 0x%016X, want 0x%016X", i, lun, tt.want[i])
				}
			}
		})
	}
}

// helper to make a Result with data reader
func goodResult(data []byte) session.Result {
	var r io.Reader
	if data != nil {
		r = bytes.NewReader(data)
	}
	return session.Result{
		Status: StatusGood,
		Data:   r,
	}
}

func checkConditionResult(senseKey SenseKey, asc, ascq uint8) session.Result {
	sense := make([]byte, 18)
	sense[0] = 0x70
	sense[2] = byte(senseKey)
	sense[7] = 0x0A
	sense[12] = asc
	sense[13] = ascq
	return session.Result{
		Status:    StatusCheckCondition,
		SenseData: sense,
	}
}
