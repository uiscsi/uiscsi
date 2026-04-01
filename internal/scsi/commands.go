package scsi

import (
	"encoding/binary"
	"fmt"

	"github.com/rkujawa/uiscsi/internal/session"
)

// TestUnitReady returns a TEST UNIT READY command (opcode 0x00).
// It checks whether the logical unit is ready to accept commands.
func TestUnitReady(lun uint64) session.Command {
	var cmd session.Command
	cmd.CDB[0] = OpTestUnitReady
	cmd.LUN = lun
	// Read=false, Write=false, ExpectedDataTransferLen=0
	return cmd
}

// RequestSense returns a REQUEST SENSE command (opcode 0x03).
// allocLen specifies the maximum number of sense data bytes to return.
func RequestSense(lun uint64, allocLen uint8) session.Command {
	var cmd session.Command
	cmd.CDB[0] = OpRequestSense
	cmd.CDB[4] = allocLen
	cmd.Read = true
	cmd.ExpectedDataTransferLen = uint32(allocLen)
	cmd.LUN = lun
	return cmd
}

// ReportLuns returns a REPORT LUNS command (opcode 0xA0).
// allocLen specifies the allocation length in bytes for the response.
// LUN is set to 0 as REPORT LUNS targets the target, not a specific LUN.
func ReportLuns(allocLen uint32) session.Command {
	var cmd session.Command
	cmd.CDB[0] = OpReportLuns
	binary.BigEndian.PutUint32(cmd.CDB[6:10], allocLen)
	cmd.Read = true
	cmd.ExpectedDataTransferLen = allocLen
	// LUN = 0: REPORT LUNS targets the target
	return cmd
}

// ParseReportLuns parses a REPORT LUNS response into a slice of LUN values.
func ParseReportLuns(result session.Result) ([]uint64, error) {
	data, err := checkResult(result)
	if err != nil {
		return nil, err
	}
	if len(data) < 8 {
		return nil, fmt.Errorf("scsi: REPORT LUNS response too short (%d bytes, need 8)", len(data))
	}

	listLen := binary.BigEndian.Uint32(data[0:4])
	numLUNs := listLen / 8

	luns := make([]uint64, 0, numLUNs)
	for i := uint32(0); i < numLUNs; i++ {
		offset := 8 + i*8
		if offset+8 > uint32(len(data)) {
			break
		}
		luns = append(luns, binary.BigEndian.Uint64(data[offset:offset+8]))
	}

	return luns, nil
}
