// Package digest provides CRC32C digest computation for iSCSI header and
// data digests as specified in RFC 7143 Section 12.1.
//
// iSCSI uses the CRC32C (Castagnoli) polynomial for both header and data
// digests. The Castagnoli polynomial (0x82f63b78) is hardware-accelerated
// on amd64 via SSE4.2 and on arm64 via NEON.
package digest

import "hash/crc32"

// crc32cTable is the precomputed CRC32C (Castagnoli) lookup table,
// initialized once at package load time.
var crc32cTable = crc32.MakeTable(crc32.Castagnoli)

// HeaderDigest computes the CRC32C digest over BHS and AHS bytes.
// Per RFC 7143 Section 12.1, the header digest covers the Basic Header
// Segment and all Additional Header Segments. The 4-byte digest value
// itself is not included in the input.
func HeaderDigest(bhsAndAHS []byte) uint32 {
	return crc32.Checksum(bhsAndAHS, crc32cTable)
}

// DataDigest computes the CRC32C digest over a data segment including
// zero-padding to a 4-byte boundary. Per RFC 7143, the data digest covers
// the data segment and its padding bytes (which must be zero).
func DataDigest(data []byte) uint32 {
	padLen := (4 - (len(data) % 4)) % 4
	if padLen == 0 {
		return crc32.Checksum(data, crc32cTable)
	}
	h := crc32.New(crc32cTable)
	h.Write(data)
	h.Write(make([]byte, padLen))
	return h.Sum32()
}
