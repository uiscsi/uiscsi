package transport

import (
	"encoding/binary"
	"io"

	"github.com/rkujawa/uiscsi/internal/digest"
	"github.com/rkujawa/uiscsi/internal/pdu"
)

// RawPDU holds the raw bytes of an iSCSI PDU as read from or to be written
// to the wire. The DataSegment is copied into caller-owned memory (not pooled).
type RawPDU struct {
	BHS          [pdu.BHSLength]byte
	AHS          []byte // nil if no AHS
	DataSegment  []byte // copied out, caller-owned
	HeaderDigest uint32 // 0 if not present
	DataDigest   uint32 // 0 if not present
	HasHDigest   bool
	HasDDigest   bool
}

// ReadRawPDU reads a complete iSCSI PDU from r. It uses io.ReadFull exclusively
// to handle partial TCP reads correctly (Pitfall 6). The digest booleans control
// whether header and data digests are expected on the wire.
//
// The returned RawPDU's DataSegment is a freshly allocated slice (caller-owned).
// Pool scratch buffers are used internally and returned after copying.
func ReadRawPDU(r io.Reader, digestHeader, digestData bool) (*RawPDU, error) {
	// Stage 1: Read exactly 48 bytes BHS.
	bhsBuf := GetBHS()
	defer PutBHS(bhsBuf)

	if _, err := io.ReadFull(r, bhsBuf[:]); err != nil {
		return nil, err
	}

	raw := &RawPDU{}
	raw.BHS = *bhsBuf

	// Stage 2: Parse lengths from BHS.
	ahsLen := uint32(raw.BHS[4]) * 4 // TotalAHSLength is in 4-byte words
	dsLen := uint32(raw.BHS[5])<<16 | uint32(raw.BHS[6])<<8 | uint32(raw.BHS[7])

	// Stage 3: Compute total remaining bytes after BHS.
	remaining := ahsLen
	if digestHeader {
		remaining += 4
	}
	padLen := pdu.PadLen(dsLen)
	remaining += dsLen + padLen
	if digestData && dsLen > 0 {
		remaining += 4
	}

	// Stage 4: Read all remaining in one io.ReadFull call.
	if remaining == 0 {
		return raw, nil
	}

	payload := GetBuffer(int(remaining))
	if _, err := io.ReadFull(r, payload[:remaining]); err != nil {
		PutBuffer(payload)
		return nil, err
	}

	// Stage 5: Slice payload into components.
	off := uint32(0)

	// AHS
	if ahsLen > 0 {
		raw.AHS = make([]byte, ahsLen)
		copy(raw.AHS, payload[off:off+ahsLen])
		off += ahsLen
	}

	// Header digest — stored as native u32 (little-endian on x86).
	if digestHeader {
		raw.HeaderDigest = binary.LittleEndian.Uint32(payload[off : off+4])
		raw.HasHDigest = true
		off += 4
	}

	// Data segment (copy out to caller-owned memory)
	if dsLen > 0 {
		raw.DataSegment = make([]byte, dsLen)
		copy(raw.DataSegment, payload[off:off+dsLen])
		off += dsLen

		// Skip padding
		off += padLen

		// Data digest — same byte order as header digest.
		if digestData {
			raw.DataDigest = binary.LittleEndian.Uint32(payload[off : off+4])
			raw.HasDDigest = true
		}
	}

	// Stage 6: Verify digests before returning.
	if digestHeader {
		var input []byte
		if len(raw.AHS) > 0 {
			input = make([]byte, pdu.BHSLength+len(raw.AHS))
			copy(input, raw.BHS[:])
			copy(input[pdu.BHSLength:], raw.AHS)
		} else {
			input = raw.BHS[:]
		}
		expected := digest.HeaderDigest(input)
		if expected != raw.HeaderDigest {
			PutBuffer(payload)
			return nil, &digest.DigestError{
				Type:     digest.DigestHeader,
				Expected: expected,
				Actual:   raw.HeaderDigest,
			}
		}
	}
	if digestData && dsLen > 0 {
		expected := digest.DataDigest(raw.DataSegment)
		if expected != raw.DataDigest {
			PutBuffer(payload)
			return nil, &digest.DigestError{
				Type:     digest.DigestData,
				Expected: expected,
				Actual:   raw.DataDigest,
			}
		}
	}

	PutBuffer(payload)
	return raw, nil
}

// WriteRawPDU writes a complete iSCSI PDU to w as a single contiguous write.
// This prevents TCP byte interleaving when used with the write pump (Pitfall 7).
func WriteRawPDU(w io.Writer, p *RawPDU) error {
	dsLen := uint32(len(p.DataSegment))
	padLen := pdu.PadLen(dsLen)

	// Compute total wire size.
	total := pdu.BHSLength + uint32(len(p.AHS))
	if p.HasHDigest {
		total += 4
	}
	total += dsLen + padLen
	if p.HasDDigest && dsLen > 0 {
		total += 4
	}

	buf := GetBuffer(int(total))
	defer PutBuffer(buf)

	off := 0

	// BHS
	copy(buf[off:], p.BHS[:])
	off += pdu.BHSLength

	// AHS
	if len(p.AHS) > 0 {
		copy(buf[off:], p.AHS)
		off += len(p.AHS)
	}

	// Header digest — stored as native u32 on the wire (little-endian on
	// x86) per Linux iSCSI target implementation. RFC 7143 Section 12.1
	// is ambiguous but all major implementations use host byte order.
	if p.HasHDigest {
		binary.LittleEndian.PutUint32(buf[off:off+4], p.HeaderDigest)
		off += 4
	}

	// Data segment
	if dsLen > 0 {
		copy(buf[off:], p.DataSegment)
		off += int(dsLen)

		// Zero padding
		for i := 0; i < int(padLen); i++ {
			buf[off+i] = 0
		}
		off += int(padLen)

		// Data digest — same byte order as header digest.
		if p.HasDDigest {
			binary.LittleEndian.PutUint32(buf[off:off+4], p.DataDigest)
			off += 4
		}
	}

	_, err := w.Write(buf[:off])
	return err
}
