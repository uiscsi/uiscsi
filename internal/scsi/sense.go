package scsi

import (
	"encoding/binary"
	"fmt"
)

// SenseData represents parsed SCSI sense data in either fixed or descriptor
// format per SPC-4 Section 4.5.
type SenseData struct {
	ResponseCode uint8
	Key          SenseKey
	ASC          uint8
	ASCQ         uint8
	// Information holds the sense information value. For fixed-format sense
	// (0x70/0x71) this is the 4-byte Information field (SPC-4 Table 28).
	// For descriptor-format sense (0x72/0x73) this is the 8-byte Information
	// field from the Information descriptor (SPC-4 4.5.2.1). Fixed-format
	// values fit in the lower 32 bits.
	Information uint64
	Valid        bool
	Filemark     bool
	EOM          bool
	ILI          bool
	Raw          []byte
}

// String returns a human-readable representation: "SENSE_KEY: asc description".
func (sd *SenseData) String() string {
	desc := ascLookup(sd.ASC, sd.ASCQ)
	if desc != "" {
		return fmt.Sprintf("%s: %s", sd.Key, desc)
	}
	return fmt.Sprintf("%s: UNKNOWN (0x%02X/0x%02X)", sd.Key, sd.ASC, sd.ASCQ)
}

// ParseSense parses raw sense data in either fixed (0x70/0x71) or descriptor
// (0x72/0x73) format per SPC-4 Section 4.5.
func ParseSense(data []byte) (*SenseData, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("scsi: sense data too short (%d bytes)", len(data))
	}

	responseCode := data[0] & 0x7F

	sd := &SenseData{
		ResponseCode: responseCode,
		Raw:          make([]byte, len(data)),
	}
	copy(sd.Raw, data)

	switch responseCode {
	case 0x70, 0x71: // fixed format (current or deferred)
		if len(data) < 18 {
			return nil, fmt.Errorf("scsi: fixed-format sense data too short (%d bytes, need 18)", len(data))
		}
		sd.Valid = data[0]&0x80 != 0
		sd.Filemark = data[2]&0x80 != 0
		sd.EOM = data[2]&0x40 != 0
		sd.ILI = data[2]&0x20 != 0
		sd.Key = SenseKey(data[2] & 0x0F)
		sd.Information = uint64(binary.BigEndian.Uint32(data[3:7]))
		sd.ASC = data[12]
		sd.ASCQ = data[13]

	case 0x72, 0x73: // descriptor format (current or deferred)
		if len(data) < 8 {
			return nil, fmt.Errorf("scsi: descriptor-format sense data too short (%d bytes, need 8)", len(data))
		}
		sd.Key = SenseKey(data[1] & 0x0F)
		sd.ASC = data[2]
		sd.ASCQ = data[3]
		parseDescriptors(sd, data)

	default:
		return nil, fmt.Errorf("scsi: unknown sense response code 0x%02X", responseCode)
	}

	return sd, nil
}

// parseDescriptors iterates SPC-4 sense descriptors (Section 4.5.2) to extract
// fields that fixed-format stores at fixed offsets but descriptor-format stores
// in typed descriptor entries. Only the Information descriptor (type 0x00) and
// Stream Commands descriptor (type 0x04) are handled; unknown types are skipped.
func parseDescriptors(sd *SenseData, data []byte) {
	if len(data) < 8 {
		return
	}
	addlLen := int(data[7])
	descStart := 8
	descEnd := descStart + addlLen
	if descEnd > len(data) {
		descEnd = len(data)
	}
	pos := descStart
	for pos+2 <= descEnd {
		descType := data[pos]
		descLen := int(data[pos+1])
		descDataEnd := pos + 2 + descLen
		if descDataEnd > descEnd {
			break // truncated descriptor — stop safely
		}
		switch descType {
		case 0x00: // Information descriptor (SPC-4 4.5.2.1)
			// Layout: [type(1), len(1), valid|reserved(1), reserved(1), info(8)]
			// descLen should be >= 10 (covers valid_bit byte, reserved, and 8-byte info).
			if descLen >= 10 {
				sd.Valid = data[pos+2]&0x80 != 0
				sd.Information = binary.BigEndian.Uint64(data[pos+4 : pos+12])
			}
		case 0x04: // Stream commands descriptor (SPC-4 4.5.2.5)
			// Layout: [type(1), len(1), reserved(1), flags(1)] — descLen=2 minimum
			if descLen >= 2 {
				sd.Filemark = data[pos+3]&0x80 != 0
				sd.EOM = data[pos+3]&0x40 != 0
				sd.ILI = data[pos+3]&0x20 != 0
			}
		}
		pos = descDataEnd
	}
}

// ascLookup returns a human-readable description for a given ASC/ASCQ pair.
// Returns empty string if not found.
func ascLookup(asc, ascq uint8) string {
	key := uint16(asc)<<8 | uint16(ascq)
	if desc, ok := ascTable[key]; ok {
		return desc
	}
	return ""
}

// ascTable maps ASC/ASCQ pairs to human-readable descriptions.
// Common codes per SPC-4 Annex D.
var ascTable = map[uint16]string{
	0x0000: "No additional sense information",
	0x0001: "Filemark detected",
	0x0002: "End-of-partition/medium detected",
	0x0004: "Beginning-of-partition/medium detected",
	0x0005: "End-of-data detected",
	0x0100: "No index/sector signal",
	0x0200: "No seek complete",
	0x0300: "Peripheral device write fault",
	0x0400: "Not ready, cause not reportable",
	0x0401: "Not ready, becoming ready",
	0x0402: "Not ready, initializing command required",
	0x0403: "Not ready, manual intervention required",
	0x0404: "Not ready, format in progress",
	0x0407: "Not ready, operation in progress",
	0x0408: "Not ready, long write in progress",
	0x0409: "Not ready, self-test in progress",
	0x0411: "Not ready, notify (enable spinup) required",
	0x0500: "Logical unit does not respond to selection",
	0x0600: "No reference position found",
	0x0700: "Multiple peripheral devices selected",
	0x0800: "Logical unit communication failure",
	0x0801: "Logical unit communication time-out",
	0x0802: "Logical unit communication parity error",
	0x0803: "Logical unit communication CRC error (ultra-DMA/32)",
	0x0900: "Track following error",
	0x0A00: "Error log overflow",
	0x0B00: "Warning",
	0x0B01: "Warning, specified temperature exceeded",
	0x0C00: "Write error",
	0x0C02: "Write error, auto reallocation failed",
	0x1000: "ID CRC or ECC error",
	0x1100: "Unrecovered read error",
	0x1101: "Read retries exhausted",
	0x1104: "Unrecovered read error, auto reallocate failed",
	0x1200: "Address mark not found for ID field",
	0x1400: "Recorded entity not found",
	0x1500: "Random positioning error",
	0x1600: "Data synchronization mark error",
	0x1700: "Recovered data with no error correction applied",
	0x1800: "Recovered data with retries",
	0x1900: "Defect list error",
	0x1A00: "Parameter list length error",
	0x2000: "Invalid command operation code",
	0x2100: "Logical block address out of range",
	0x2400: "Invalid field in CDB",
	0x2500: "Logical unit not supported",
	0x2600: "Invalid field in parameter list",
	0x2700: "Write protected",
	0x2800: "Not ready to ready change, medium may have changed",
	0x2900: "Power on, reset, or bus device reset occurred",
	0x2901: "Power on occurred",
	0x2902: "SCSI bus reset occurred",
	0x2903: "Bus device reset function occurred",
	0x2904: "Device internal reset",
	0x2A00: "Parameters changed",
	0x2A01: "Mode parameters changed",
	0x2A02: "Log parameters changed",
	0x2A03: "Reservations preempted",
	0x2A04: "Reservations released",
	0x2A05: "Registrations preempted",
	0x2C00: "Command sequence error",
	0x2F00: "Commands cleared by another initiator",
	0x3000: "Incompatible medium installed",
	0x3003: "Cleaning cartridge installed",
	0x3100: "Medium format corrupted",
	0x3200: "No defect spare location available",
	0x3500: "Enclosure failure",
	0x3700: "Rounded parameter",
	0x3900: "Saving parameters not supported",
	0x3A00: "Medium not present",
	0x3B00: "Sequential positioning error",
	0x3D00: "Invalid bits in identify message",
	0x3E00: "Logical unit has not self-configured yet",
	0x3F00: "Target operating conditions have changed",
	0x3F01: "Microcode has been changed",
	0x3F02: "Changed operating definition",
	0x3F03: "Inquiry data has changed",
	0x4000: "RAM failure",
	0x4100: "Data path failure",
	0x4200: "Power-on or self-test failure",
	0x4300: "Message error",
	0x4400: "Internal target failure",
	0x4500: "Select or reselect failure",
	0x4700: "SCSI parity error",
	0x4800: "Initiator detected error message received",
	0x4900: "Invalid message error",
	0x4E00: "Overlapped commands attempted",
	0x5300: "Media load or eject failed",
	0x5500: "System resource failure",
	0x5D00: "Failure prediction threshold exceeded",
	0x5E00: "Low power condition on",
}
