package transport

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/uiscsi/uiscsi/internal/digest"
	"github.com/uiscsi/uiscsi/internal/pdu"
)

// makeBHS constructs a minimal 48-byte BHS with the given opcode, totalAHSLength (in 4-byte words),
// and dataSegmentLength. All other fields are zero.
func makeBHS(opcode pdu.OpCode, totalAHSLen uint8, dsLen uint32) [pdu.BHSLength]byte {
	var bhs [pdu.BHSLength]byte
	bhs[0] = byte(opcode)
	bhs[4] = totalAHSLen
	// 24-bit DataSegmentLength in bytes 5-7
	bhs[5] = byte(dsLen >> 16)
	bhs[6] = byte(dsLen >> 8)
	bhs[7] = byte(dsLen)
	return bhs
}

// writeRawBytes writes a hand-crafted PDU to w: BHS + AHS + optional header digest +
// data segment + padding + optional data digest.
func writeRawBytes(w io.Writer, bhs [pdu.BHSLength]byte, ahs []byte, data []byte, hdigest, ddigest bool) {
	w.Write(bhs[:])
	if len(ahs) > 0 {
		w.Write(ahs)
	}
	if hdigest {
		var hd [4]byte
		binary.LittleEndian.PutUint32(hd[:], 0xDEADBEEF) // dummy digest
		w.Write(hd[:])
	}
	if len(data) > 0 {
		w.Write(data)
		padLen := pdu.PadLen(uint32(len(data)))
		if padLen > 0 {
			w.Write(make([]byte, padLen))
		}
	}
	if ddigest && len(data) > 0 {
		var dd [4]byte
		binary.LittleEndian.PutUint32(dd[:], 0xCAFEBABE) // dummy digest
		w.Write(dd[:])
	}
}

func TestFramerReadRawPDU_Basic(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	data := []byte{0x01, 0x02, 0x03, 0x04, 0x05} // 5 bytes, needs 3 padding
	bhs := makeBHS(pdu.OpNOPOut, 0, uint32(len(data)))

	go writeRawBytes(wConn, bhs, nil, data, false, false)

	raw, err := ReadRawPDU(rConn, false, false, 0)
	if err != nil {
		t.Fatalf("ReadRawPDU: %v", err)
	}
	if raw.BHS != bhs {
		t.Error("BHS mismatch")
	}
	if !bytes.Equal(raw.DataSegment, data) {
		t.Errorf("data segment: got %x, want %x", raw.DataSegment, data)
	}
	if raw.AHS != nil {
		t.Error("AHS should be nil")
	}
}

func TestFramerReadRawPDU_BackToBack(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	data1 := []byte{0xAA, 0xBB}            // 2 bytes, 2 pad
	data2 := []byte{0xCC, 0xDD, 0xEE, 0xFF} // 4 bytes, 0 pad
	bhs1 := makeBHS(pdu.OpNOPOut, 0, uint32(len(data1)))
	bhs2 := makeBHS(pdu.OpNOPIn, 0, uint32(len(data2)))

	go func() {
		writeRawBytes(wConn, bhs1, nil, data1, false, false)
		writeRawBytes(wConn, bhs2, nil, data2, false, false)
	}()

	raw1, err := ReadRawPDU(rConn, false, false, 0)
	if err != nil {
		t.Fatalf("ReadRawPDU #1: %v", err)
	}
	if !bytes.Equal(raw1.DataSegment, data1) {
		t.Errorf("PDU 1 data: got %x, want %x", raw1.DataSegment, data1)
	}

	raw2, err := ReadRawPDU(rConn, false, false, 0)
	if err != nil {
		t.Fatalf("ReadRawPDU #2: %v", err)
	}
	if !bytes.Equal(raw2.DataSegment, data2) {
		t.Errorf("PDU 2 data: got %x, want %x", raw2.DataSegment, data2)
	}
}

func TestFramerReadRawPDU_WithAHS(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	// AHS is 8 bytes (TotalAHSLength = 2 words)
	ahs := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	bhs := makeBHS(pdu.OpSCSICommand, 2, 0) // 2 words AHS, no data

	go writeRawBytes(wConn, bhs, ahs, nil, false, false)

	raw, err := ReadRawPDU(rConn, false, false, 0)
	if err != nil {
		t.Fatalf("ReadRawPDU: %v", err)
	}
	if !bytes.Equal(raw.AHS, ahs) {
		t.Errorf("AHS: got %x, want %x", raw.AHS, ahs)
	}
}

func TestFramerReadRawPDU_HeaderDigest(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	bhs := makeBHS(pdu.OpNOPOut, 0, 0)
	go writeRawBytesWithDigests(wConn, bhs, nil, nil, true, false)

	raw, err := ReadRawPDU(rConn, true, false, 0)
	if err != nil {
		t.Fatalf("ReadRawPDU: %v", err)
	}
	if !raw.HasHDigest {
		t.Error("expected HasHDigest=true")
	}
	expected := digest.HeaderDigest(bhs[:])
	if raw.HeaderDigest != expected {
		t.Errorf("header digest: got 0x%08X, want 0x%08X", raw.HeaderDigest, expected)
	}
}

func TestFramerReadRawPDU_DataDigest(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	data := []byte{0x01, 0x02, 0x03} // 3 bytes, 1 pad
	bhs := makeBHS(pdu.OpNOPOut, 0, uint32(len(data)))

	go writeRawBytesWithDigests(wConn, bhs, nil, data, false, true)

	raw, err := ReadRawPDU(rConn, false, true, 0)
	if err != nil {
		t.Fatalf("ReadRawPDU: %v", err)
	}
	if !raw.HasDDigest {
		t.Error("expected HasDDigest=true")
	}
	expected := digest.DataDigest(data)
	if raw.DataDigest != expected {
		t.Errorf("data digest: got 0x%08X, want 0x%08X", raw.DataDigest, expected)
	}
}

func TestFramerReadRawPDU_ZeroLengthData(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	bhs := makeBHS(pdu.OpNOPOut, 0, 0)
	go writeRawBytes(wConn, bhs, nil, nil, false, false)

	raw, err := ReadRawPDU(rConn, false, false, 0)
	if err != nil {
		t.Fatalf("ReadRawPDU: %v", err)
	}
	if len(raw.DataSegment) != 0 {
		t.Errorf("expected empty data segment, got %d bytes", len(raw.DataSegment))
	}
}

func TestFramerReadRawPDU_PaddingVariants(t *testing.T) {
	tests := []struct {
		name    string
		dataLen int
		padLen  int
	}{
		{"1 byte pad", 3, 1},
		{"2 bytes pad", 2, 2},
		{"3 bytes pad", 1, 3},
		{"0 bytes pad", 4, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rConn, wConn := net.Pipe()
			defer rConn.Close()
			defer wConn.Close()

			data := make([]byte, tt.dataLen)
			for i := range data {
				data[i] = byte(i + 1)
			}
			bhs := makeBHS(pdu.OpNOPOut, 0, uint32(tt.dataLen))

			go writeRawBytes(wConn, bhs, nil, data, false, false)

			raw, err := ReadRawPDU(rConn, false, false, 0)
			if err != nil {
				t.Fatalf("ReadRawPDU: %v", err)
			}
			if !bytes.Equal(raw.DataSegment, data) {
				t.Errorf("data: got %x, want %x", raw.DataSegment, data)
			}
		})
	}
}

func TestFramerReadRawPDU_TruncatedBHS(t *testing.T) {
	// Write only 10 bytes then close -- less than 48 byte BHS
	rConn, wConn := net.Pipe()
	defer rConn.Close()

	go func() {
		wConn.Write(make([]byte, 10))
		wConn.Close()
	}()

	_, err := ReadRawPDU(rConn, false, false, 0)
	if err == nil {
		t.Fatal("expected error on truncated BHS")
	}
}

func TestFramerWriteRawPDU_RoundTrip(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	data := []byte{0x10, 0x20, 0x30, 0x40, 0x50} // 5 bytes
	bhs := makeBHS(pdu.OpNOPIn, 0, uint32(len(data)))

	original := &RawPDU{
		BHS:         bhs,
		DataSegment: data,
	}

	go func() {
		if err := WriteRawPDU(wConn, original); err != nil {
			t.Errorf("WriteRawPDU: %v", err)
		}
	}()

	got, err := ReadRawPDU(rConn, false, false, 0)
	if err != nil {
		t.Fatalf("ReadRawPDU: %v", err)
	}
	if got.BHS != bhs {
		t.Error("BHS mismatch in round trip")
	}
	if !bytes.Equal(got.DataSegment, data) {
		t.Errorf("data mismatch: got %x, want %x", got.DataSegment, data)
	}
}

func TestFramerWriteRawPDU_WithDigests(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	data := []byte{0xAB, 0xCD}
	bhs := makeBHS(pdu.OpNOPOut, 0, uint32(len(data)))

	// Use real CRC32C digests so verification passes on read-back.
	hd := digest.HeaderDigest(bhs[:])
	dd := digest.DataDigest(data)

	original := &RawPDU{
		BHS:          bhs,
		DataSegment:  data,
		HeaderDigest: hd,
		DataDigest:   dd,
		HasHDigest:   true,
		HasDDigest:   true,
	}

	go func() {
		if err := WriteRawPDU(wConn, original); err != nil {
			t.Errorf("WriteRawPDU: %v", err)
		}
	}()

	got, err := ReadRawPDU(rConn, true, true, 0)
	if err != nil {
		t.Fatalf("ReadRawPDU: %v", err)
	}
	if got.HeaderDigest != hd {
		t.Errorf("header digest: got 0x%08X, want 0x%08X", got.HeaderDigest, hd)
	}
	if got.DataDigest != dd {
		t.Errorf("data digest: got 0x%08X, want 0x%08X", got.DataDigest, dd)
	}
}

// writeRawBytesWithDigests writes a PDU with correct CRC32C digests on the wire.
func writeRawBytesWithDigests(w io.Writer, bhs [pdu.BHSLength]byte, ahs []byte, data []byte, hdigest, ddigest bool) {
	w.Write(bhs[:])
	if len(ahs) > 0 {
		w.Write(ahs)
	}
	if hdigest {
		input := bhs[:]
		if len(ahs) > 0 {
			input = append(bhs[:], ahs...)
		}
		var hd [4]byte
		binary.LittleEndian.PutUint32(hd[:], digest.HeaderDigest(input))
		w.Write(hd[:])
	}
	if len(data) > 0 {
		w.Write(data)
		padLen := pdu.PadLen(uint32(len(data)))
		if padLen > 0 {
			w.Write(make([]byte, padLen))
		}
	}
	if ddigest && len(data) > 0 {
		var dd [4]byte
		binary.LittleEndian.PutUint32(dd[:], digest.DataDigest(data))
		w.Write(dd[:])
	}
}

func TestFramerReadRawPDU_HeaderDigestMismatch(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	bhs := makeBHS(pdu.OpNOPOut, 0, 0)

	go func() {
		// Write BHS then a wrong header digest
		wConn.Write(bhs[:])
		var hd [4]byte
		binary.LittleEndian.PutUint32(hd[:], 0xBAD0BAD0) // wrong digest
		wConn.Write(hd[:])
	}()

	_, err := ReadRawPDU(rConn, true, false, 0)
	if err == nil {
		t.Fatal("expected error on header digest mismatch")
	}
	var de *digest.DigestError
	if !errors.As(err, &de) {
		t.Fatalf("expected *digest.DigestError, got %T: %v", err, err)
	}
	if de.Type != digest.DigestHeader {
		t.Errorf("DigestError.Type = %v, want DigestHeader", de.Type)
	}
	if de.Actual != 0xBAD0BAD0 {
		t.Errorf("DigestError.Actual = 0x%08X, want 0xBAD0BAD0", de.Actual)
	}
	// Expected should be the real CRC32C of the BHS
	expected := digest.HeaderDigest(bhs[:])
	if de.Expected != expected {
		t.Errorf("DigestError.Expected = 0x%08X, want 0x%08X", de.Expected, expected)
	}
}

func TestFramerReadRawPDU_DataDigestMismatch(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	data := []byte{0x01, 0x02, 0x03} // 3 bytes, 1 pad
	bhs := makeBHS(pdu.OpNOPOut, 0, uint32(len(data)))

	go func() {
		wConn.Write(bhs[:])
		wConn.Write(data)
		padLen := pdu.PadLen(uint32(len(data)))
		if padLen > 0 {
			wConn.Write(make([]byte, padLen))
		}
		var dd [4]byte
		binary.LittleEndian.PutUint32(dd[:], 0xDEAD0000) // wrong digest
		wConn.Write(dd[:])
	}()

	_, err := ReadRawPDU(rConn, false, true, 0)
	if err == nil {
		t.Fatal("expected error on data digest mismatch")
	}
	var de *digest.DigestError
	if !errors.As(err, &de) {
		t.Fatalf("expected *digest.DigestError, got %T: %v", err, err)
	}
	if de.Type != digest.DigestData {
		t.Errorf("DigestError.Type = %v, want DigestData", de.Type)
	}
	if de.Actual != 0xDEAD0000 {
		t.Errorf("DigestError.Actual = 0x%08X, want 0xDEAD0000", de.Actual)
	}
	expected := digest.DataDigest(data)
	if de.Expected != expected {
		t.Errorf("DigestError.Expected = 0x%08X, want 0x%08X", de.Expected, expected)
	}
}

func TestFramerReadRawPDU_CorrectDigestsNoError(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	data := []byte{0xAA, 0xBB, 0xCC, 0xDD}
	bhs := makeBHS(pdu.OpNOPOut, 0, uint32(len(data)))

	go writeRawBytesWithDigests(wConn, bhs, nil, data, true, true)

	raw, err := ReadRawPDU(rConn, true, true, 0)
	if err != nil {
		t.Fatalf("ReadRawPDU with correct digests: %v", err)
	}
	if !bytes.Equal(raw.DataSegment, data) {
		t.Errorf("data: got %x, want %x", raw.DataSegment, data)
	}
}

func TestFramerReadRawPDU_DigestDisabledNoVerify(t *testing.T) {
	// When digestHeader=false, wrong digest bytes are NOT on the wire,
	// so no verification happens (existing behavior preserved).
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	bhs := makeBHS(pdu.OpNOPOut, 0, 0)
	go func() {
		wConn.Write(bhs[:])
	}()

	raw, err := ReadRawPDU(rConn, false, false, 0)
	if err != nil {
		t.Fatalf("ReadRawPDU: %v", err)
	}
	if raw.HasHDigest {
		t.Error("HasHDigest should be false when digestHeader=false")
	}
}

func TestFramerReadRawPDU_HeaderDigestMismatchWithAHS(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	ahs := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	bhs := makeBHS(pdu.OpSCSICommand, 2, 0) // 2 words AHS

	go func() {
		wConn.Write(bhs[:])
		wConn.Write(ahs)
		var hd [4]byte
		binary.LittleEndian.PutUint32(hd[:], 0xBAD0BAD0) // wrong digest
		wConn.Write(hd[:])
	}()

	_, err := ReadRawPDU(rConn, true, false, 0)
	if err == nil {
		t.Fatal("expected error on header digest mismatch with AHS")
	}
	var de *digest.DigestError
	if !errors.As(err, &de) {
		t.Fatalf("expected *digest.DigestError, got %T: %v", err, err)
	}
	// Expected should cover BHS+AHS
	expected := digest.HeaderDigest(append(bhs[:], ahs...))
	if de.Expected != expected {
		t.Errorf("DigestError.Expected = 0x%08X, want 0x%08X", de.Expected, expected)
	}
}

func TestReadRawPDU_ExceedsMaxRecvDSL(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	// Build a PDU with dsLen=1024.
	bhs := makeBHS(0x25, 0, 1024)
	data := make([]byte, 1024)

	go func() {
		wConn.Write(bhs[:])
		wConn.Write(data)
	}()

	// maxRecvDSL=512 should reject the PDU.
	_, err := ReadRawPDU(rConn, false, false, 512)
	if err == nil {
		t.Fatal("expected error for dsLen exceeding maxRecvDSL")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("exceeds MaxRecvDataSegmentLength")) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReadRawPDU_MaxRecvDSLZeroUnlimited(t *testing.T) {
	rConn, wConn := net.Pipe()
	defer rConn.Close()
	defer wConn.Close()

	bhs := makeBHS(0x25, 0, 64)
	data := make([]byte, 64)

	go func() {
		wConn.Write(bhs[:])
		wConn.Write(data)
	}()

	// maxRecvDSL=0 means unlimited — should succeed.
	raw, err := ReadRawPDU(rConn, false, false, 0)
	if err != nil {
		t.Fatalf("maxRecvDSL=0 should allow any size: %v", err)
	}
	if len(raw.DataSegment) != 64 {
		t.Fatalf("expected 64-byte data segment, got %d", len(raw.DataSegment))
	}
}
