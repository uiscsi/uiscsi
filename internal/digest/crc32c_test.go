package digest

import (
	"bytes"
	"hash/crc32"
	"testing"
)

func TestHeaderDigest(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  uint32
	}{
		{
			name:  "RFC test vector: 123456789",
			input: []byte("123456789"),
			want:  0xe3069283,
		},
		{
			name:  "empty input",
			input: []byte{},
			want:  0x00000000,
		},
		{
			name:  "32 zero bytes",
			input: bytes.Repeat([]byte{0x00}, 32),
			want:  0x8a9136aa,
		},
		{
			name:  "32 0xFF bytes",
			input: bytes.Repeat([]byte{0xFF}, 32),
			want:  0x62a8ab43,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HeaderDigest(tt.input); got != tt.want {
				t.Errorf("HeaderDigest(%q) = 0x%08x, want 0x%08x", tt.input, got, tt.want)
			}
		})
	}
}

func TestDataDigest(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		if got := DataDigest([]byte{}); got != 0x00000000 {
			t.Errorf("DataDigest(empty) = 0x%08x, want 0x00000000", got)
		}
	})

	t.Run("4-byte aligned equals HeaderDigest", func(t *testing.T) {
		data := []byte{0x01, 0x02, 0x03, 0x04}
		dd := DataDigest(data)
		hd := HeaderDigest(data)
		if dd != hd {
			t.Errorf("DataDigest(4 bytes) = 0x%08x, want HeaderDigest = 0x%08x (no padding needed)", dd, hd)
		}
	})

	t.Run("5-byte input includes 3 padding zeros", func(t *testing.T) {
		data := []byte{0x01, 0x02, 0x03, 0x04, 0x05}
		got := DataDigest(data)

		// Manually compute CRC32C over data + 3 zero padding bytes.
		table := crc32.MakeTable(crc32.Castagnoli)
		h := crc32.New(table)
		h.Write(data)
		h.Write([]byte{0x00, 0x00, 0x00})
		want := h.Sum32()

		if got != want {
			t.Errorf("DataDigest(5 bytes) = 0x%08x, want 0x%08x (data + 3 zero padding)", got, want)
		}
	})

	t.Run("8-byte aligned no padding", func(t *testing.T) {
		data := []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE, 0xBA, 0xBE}
		got := DataDigest(data)
		// Already aligned, should equal simple CRC32C.
		table := crc32.MakeTable(crc32.Castagnoli)
		want := crc32.Checksum(data, table)
		if got != want {
			t.Errorf("DataDigest(8 bytes aligned) = 0x%08x, want 0x%08x", got, want)
		}
	})

	t.Run("7-byte input includes 1 padding zero", func(t *testing.T) {
		data := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07}
		got := DataDigest(data)

		table := crc32.MakeTable(crc32.Castagnoli)
		h := crc32.New(table)
		h.Write(data)
		h.Write([]byte{0x00})
		want := h.Sum32()

		if got != want {
			t.Errorf("DataDigest(7 bytes) = 0x%08x, want 0x%08x (data + 1 zero padding)", got, want)
		}
	})
}
