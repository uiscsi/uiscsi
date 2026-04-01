package scsi

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/rkujawa/uiscsi/internal/session"
)

// InquiryResponse represents the standard INQUIRY data returned by a device.
type InquiryResponse struct {
	PeripheralDeviceType uint8
	PeripheralQualifier  uint8
	Vendor               string
	Product              string
	Revision             string
	AdditionalLength     uint8
	Raw                  []byte
}

// Inquiry returns a standard INQUIRY command (opcode 0x12, EVPD=0).
// allocLen specifies the maximum response length.
func Inquiry(lun uint64, allocLen uint16) session.Command {
	var cmd session.Command
	cmd.CDB[0] = OpInquiry
	// CDB[1] = 0 (EVPD=0)
	// CDB[2] = 0 (page code)
	binary.BigEndian.PutUint16(cmd.CDB[3:5], allocLen)
	cmd.Read = true
	cmd.ExpectedDataTransferLen = uint32(allocLen)
	cmd.LUN = lun
	return cmd
}

// InquiryVPD returns an INQUIRY command with EVPD=1 for Vital Product Data
// pages (opcode 0x12, byte 1 bit 0 set). pageCode selects which VPD page
// to request.
func InquiryVPD(lun uint64, pageCode uint8, allocLen uint16) session.Command {
	var cmd session.Command
	cmd.CDB[0] = OpInquiry
	cmd.CDB[1] = 0x01 // EVPD bit
	cmd.CDB[2] = pageCode
	binary.BigEndian.PutUint16(cmd.CDB[3:5], allocLen)
	cmd.Read = true
	cmd.ExpectedDataTransferLen = uint32(allocLen)
	cmd.LUN = lun
	return cmd
}

// ParseInquiry parses a standard INQUIRY response from a session.Result.
// Per Pitfall 9: vendor, product, and revision strings are trimmed of
// trailing spaces.
func ParseInquiry(result session.Result) (*InquiryResponse, error) {
	data, err := checkResult(result)
	if err != nil {
		return nil, err
	}
	if len(data) < 36 {
		return nil, fmt.Errorf("scsi: INQUIRY response too short (%d bytes, need 36)", len(data))
	}

	resp := &InquiryResponse{
		PeripheralDeviceType: data[0] & 0x1F,
		PeripheralQualifier:  (data[0] >> 5) & 0x07,
		AdditionalLength:     data[4],
		Vendor:               strings.TrimRight(string(data[8:16]), " "),
		Product:              strings.TrimRight(string(data[16:32]), " "),
		Revision:             strings.TrimRight(string(data[32:36]), " "),
		Raw:                  make([]byte, len(data)),
	}
	copy(resp.Raw, data)

	return resp, nil
}
