package scsi

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/rkujawa/uiscsi/internal/session"
)

func TestPersistReserveIn(t *testing.T) {
	tests := []struct {
		name          string
		serviceAction uint8
		allocLen      uint16
		wantByte1     uint8
	}{
		{
			name:          "READ KEYS",
			serviceAction: PRInReadKeys,
			allocLen:      256,
			wantByte1:     0x00,
		},
		{
			name:          "READ RESERVATION",
			serviceAction: PRInReadReservation,
			allocLen:      256,
			wantByte1:     0x01,
		},
		{
			name:          "REPORT CAPABILITIES",
			serviceAction: PRInReportCapabilities,
			allocLen:      512,
			wantByte1:     0x02,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := PersistReserveIn(1, tt.serviceAction, tt.allocLen)

			if cmd.CDB[0] != OpPersistReserveIn {
				t.Errorf("CDB[0] = 0x%02X, want 0x%02X", cmd.CDB[0], OpPersistReserveIn)
			}
			if cmd.CDB[1] != tt.wantByte1 {
				t.Errorf("CDB[1] = 0x%02X, want 0x%02X", cmd.CDB[1], tt.wantByte1)
			}
			allocLen := binary.BigEndian.Uint16(cmd.CDB[7:9])
			if allocLen != tt.allocLen {
				t.Errorf("allocation length = %d, want %d", allocLen, tt.allocLen)
			}
			if !cmd.Read {
				t.Error("Read should be true")
			}
			if cmd.Write {
				t.Error("Write should be false")
			}
			if cmd.ExpectedDataTransferLen != uint32(tt.allocLen) {
				t.Errorf("ExpectedDataTransferLen = %d, want %d", cmd.ExpectedDataTransferLen, tt.allocLen)
			}
			if cmd.LUN != 1 {
				t.Errorf("LUN = %d, want 1", cmd.LUN)
			}
		})
	}
}

func TestParsePersistReserveInKeys(t *testing.T) {
	t.Run("header with 2 keys", func(t *testing.T) {
		data := make([]byte, 24) // 8 header + 2*8 keys
		binary.BigEndian.PutUint32(data[0:4], 42)  // generation
		binary.BigEndian.PutUint32(data[4:8], 16)   // additional length = 16
		binary.BigEndian.PutUint64(data[8:16], 0xDEADBEEF00000001)
		binary.BigEndian.PutUint64(data[16:24], 0xCAFEBABE00000002)

		result := session.Result{
			Status: StatusGood,
			Data:   bytes.NewReader(data),
		}
		resp, err := ParsePersistReserveInKeys(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Generation != 42 {
			t.Errorf("Generation = %d, want 42", resp.Generation)
		}
		if len(resp.Keys) != 2 {
			t.Fatalf("got %d keys, want 2", len(resp.Keys))
		}
		if resp.Keys[0] != 0xDEADBEEF00000001 {
			t.Errorf("Keys[0] = 0x%016X, want 0xDEADBEEF00000001", resp.Keys[0])
		}
		if resp.Keys[1] != 0xCAFEBABE00000002 {
			t.Errorf("Keys[1] = 0x%016X, want 0xCAFEBABE00000002", resp.Keys[1])
		}
	})

	t.Run("empty keys list", func(t *testing.T) {
		data := make([]byte, 8)
		binary.BigEndian.PutUint32(data[0:4], 1) // generation
		binary.BigEndian.PutUint32(data[4:8], 0) // additional length = 0

		result := session.Result{
			Status: StatusGood,
			Data:   bytes.NewReader(data),
		}
		resp, err := ParsePersistReserveInKeys(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.Generation != 1 {
			t.Errorf("Generation = %d, want 1", resp.Generation)
		}
		if len(resp.Keys) != 0 {
			t.Errorf("got %d keys, want 0", len(resp.Keys))
		}
	})

	t.Run("short data", func(t *testing.T) {
		result := session.Result{
			Status: StatusGood,
			Data:   bytes.NewReader([]byte{0, 0, 0}),
		}
		_, err := ParsePersistReserveInKeys(result)
		if err == nil {
			t.Fatal("expected error for short data")
		}
	})

	t.Run("check condition", func(t *testing.T) {
		result := checkConditionResult(SenseIllegalRequest, 0x24, 0x00)
		_, err := ParsePersistReserveInKeys(result)
		if err == nil {
			t.Fatal("expected error for check condition")
		}
	})
}

func TestParsePersistReserveInReservation(t *testing.T) {
	t.Run("active reservation", func(t *testing.T) {
		data := make([]byte, 24) // 8 header + 16 descriptor
		binary.BigEndian.PutUint32(data[0:4], 5)  // generation
		binary.BigEndian.PutUint32(data[4:8], 16)  // additional length
		binary.BigEndian.PutUint64(data[8:16], 0x1234567890ABCDEF) // key
		data[21] = 0x31 // scope=3 (bits 7-4), type=1 (bits 3-0)

		result := session.Result{
			Status: StatusGood,
			Data:   bytes.NewReader(data),
		}
		res, err := ParsePersistReserveInReservation(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res == nil {
			t.Fatal("expected non-nil reservation")
		}
		if res.Key != 0x1234567890ABCDEF {
			t.Errorf("Key = 0x%016X, want 0x1234567890ABCDEF", res.Key)
		}
		if res.ScopeType != 0x31 {
			t.Errorf("ScopeType = 0x%02X, want 0x31", res.ScopeType)
		}
	})

	t.Run("no reservation held", func(t *testing.T) {
		data := make([]byte, 8)
		binary.BigEndian.PutUint32(data[0:4], 3) // generation
		binary.BigEndian.PutUint32(data[4:8], 0) // additional length = 0

		result := session.Result{
			Status: StatusGood,
			Data:   bytes.NewReader(data),
		}
		res, err := ParsePersistReserveInReservation(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res != nil {
			t.Error("expected nil reservation when no reservation held")
		}
	})
}

func TestPersistReserveOut(t *testing.T) {
	tests := []struct {
		name          string
		serviceAction uint8
		scopeType     uint8
		key           uint64
		saKey         uint64
	}{
		{
			name:          "REGISTER",
			serviceAction: PROutRegister,
			scopeType:     0x00,
			key:           0,
			saKey:         0xAABBCCDD00112233,
		},
		{
			name:          "RESERVE",
			serviceAction: PROutReserve,
			scopeType:     0x01,
			key:           0xAABBCCDD00112233,
			saKey:         0,
		},
		{
			name:          "RELEASE",
			serviceAction: PROutRelease,
			scopeType:     0x01,
			key:           0xAABBCCDD00112233,
			saKey:         0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := PersistReserveOut(2, tt.serviceAction, tt.scopeType, tt.key, tt.saKey)

			if cmd.CDB[0] != OpPersistReserveOut {
				t.Errorf("CDB[0] = 0x%02X, want 0x%02X", cmd.CDB[0], OpPersistReserveOut)
			}
			if cmd.CDB[1] != tt.serviceAction&0x1F {
				t.Errorf("CDB[1] = 0x%02X, want 0x%02X", cmd.CDB[1], tt.serviceAction&0x1F)
			}
			if cmd.CDB[2] != tt.scopeType {
				t.Errorf("CDB[2] = 0x%02X, want 0x%02X", cmd.CDB[2], tt.scopeType)
			}

			// Parameter list length in bytes 5-8 = 24
			actualParamLen := binary.BigEndian.Uint32(cmd.CDB[5:9])
			if actualParamLen != 24 {
				t.Errorf("parameter list length = %d, want 24", actualParamLen)
			}

			if !cmd.Write {
				t.Error("Write should be true")
			}
			if cmd.ExpectedDataTransferLen != 24 {
				t.Errorf("ExpectedDataTransferLen = %d, want 24", cmd.ExpectedDataTransferLen)
			}

			// Verify parameter data
			data, err := io.ReadAll(cmd.Data)
			if err != nil {
				t.Fatalf("reading data: %v", err)
			}
			if len(data) != 24 {
				t.Fatalf("data length = %d, want 24", len(data))
			}

			// bytes 0-7 = reservation key
			gotKey := binary.BigEndian.Uint64(data[0:8])
			if gotKey != tt.key {
				t.Errorf("key = 0x%016X, want 0x%016X", gotKey, tt.key)
			}

			// bytes 8-15 = service action reservation key
			gotSAKey := binary.BigEndian.Uint64(data[8:16])
			if gotSAKey != tt.saKey {
				t.Errorf("saKey = 0x%016X, want 0x%016X", gotSAKey, tt.saKey)
			}

			// bytes 16-23 should be zero
			for i := 16; i < 24; i++ {
				if data[i] != 0 {
					t.Errorf("data[%d] = 0x%02X, want 0x00", i, data[i])
				}
			}

			if cmd.LUN != 2 {
				t.Errorf("LUN = %d, want 2", cmd.LUN)
			}
		})
	}
}
