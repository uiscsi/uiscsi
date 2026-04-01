package scsi

import (
	"encoding/binary"
	"fmt"
	"io"

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

// Verify10 returns a VERIFY(10) command (opcode 0x2F).
// It requests the target to verify the specified LBA range.
// WithBytchk(n) sets the byte check mode; when non-zero the caller must
// also set cmd.Data and cmd.Write for the comparison data.
func Verify10(lun uint64, lba uint32, blocks uint16, opts ...Option) session.Command {
	cfg := applyOptions(opts)
	var cmd session.Command
	cmd.CDB[0] = OpVerify10
	cmd.CDB[1] = (cfg.bytchk & 0x03) << 1
	binary.BigEndian.PutUint32(cmd.CDB[2:6], lba)
	binary.BigEndian.PutUint16(cmd.CDB[7:9], blocks)
	cmd.LUN = lun
	return cmd
}

// Verify16 returns a VERIFY(16) command (opcode 0x8F).
// It supports 64-bit LBA and 32-bit block count.
func Verify16(lun uint64, lba uint64, blocks uint32, opts ...Option) session.Command {
	cfg := applyOptions(opts)
	var cmd session.Command
	cmd.CDB[0] = OpVerify16
	cmd.CDB[1] = (cfg.bytchk & 0x03) << 1
	binary.BigEndian.PutUint64(cmd.CDB[2:10], lba)
	binary.BigEndian.PutUint32(cmd.CDB[10:14], blocks)
	cmd.LUN = lun
	return cmd
}

// CompareAndWrite returns a COMPARE AND WRITE command (opcode 0x89).
// The data reader must contain 2*blocks*blockSize bytes: the first half is
// the expected current data, the second half is the replacement data.
// This is an atomic read-compare-write operation (Pitfall 8).
func CompareAndWrite(lun uint64, lba uint64, blocks uint8, blockSize uint32, data io.Reader) session.Command {
	var cmd session.Command
	cmd.CDB[0] = OpCompareAndWrite
	binary.BigEndian.PutUint64(cmd.CDB[2:10], lba)
	cmd.CDB[13] = blocks
	cmd.Write = true
	cmd.Data = data
	cmd.ExpectedDataTransferLen = 2 * uint32(blocks) * blockSize
	cmd.LUN = lun
	return cmd
}

// StartStopUnit returns a START STOP UNIT command (opcode 0x1B).
// powerCondition sets the power condition field (byte 4 bits 7-4).
// start sets the START bit; loadEject sets the LOEJ bit.
// WithImmed() makes the command return before the operation completes.
func StartStopUnit(lun uint64, powerCondition uint8, start, loadEject bool, opts ...Option) session.Command {
	cfg := applyOptions(opts)
	var cmd session.Command
	cmd.CDB[0] = OpStartStopUnit
	if cfg.immed {
		cmd.CDB[1] |= 0x01
	}
	cmd.CDB[4] = (powerCondition & 0x0F) << 4
	if loadEject {
		cmd.CDB[4] |= 0x02
	}
	if start {
		cmd.CDB[4] |= 0x01
	}
	cmd.LUN = lun
	return cmd
}
