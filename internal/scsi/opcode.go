// Package scsi constructs SCSI Command Descriptor Blocks (CDBs) and parses
// responses for standard SCSI commands per SPC-4 and SBC-3. This is a pure
// data-transformation layer: functions take parameters and produce
// session.Command structs with correctly packed CDB bytes, or parse
// session.Result into typed Go structs.
package scsi

import (
	"errors"
	"fmt"
	"io"

	"github.com/rkujawa/uiscsi/internal/session"
)

// SCSI operation codes per SPC-4 and SBC-3.
const (
	OpTestUnitReady       = 0x00
	OpRequestSense        = 0x03
	OpInquiry             = 0x12
	OpModeSense6          = 0x1A
	OpStartStopUnit       = 0x1B
	OpReadCapacity10      = 0x25
	OpRead10              = 0x28
	OpWrite10             = 0x2A
	OpVerify10            = 0x2F
	OpSynchronizeCache10  = 0x35
	OpWriteSame10         = 0x41
	OpUnmap               = 0x42
	OpModeSense10         = 0x5A
	OpPersistReserveIn    = 0x5E
	OpPersistReserveOut   = 0x5F
	OpRead16              = 0x88
	OpCompareAndWrite     = 0x89
	OpWrite16             = 0x8A
	OpVerify16            = 0x8F
	OpSynchronizeCache16  = 0x91
	OpWriteSame16         = 0x93
	OpServiceActionIn16   = 0x9E
	OpReportLuns          = 0xA0
)

// SCSI status codes per SAM-5.
const (
	StatusGood                = 0x00
	StatusCheckCondition      = 0x02
	StatusConditionMet        = 0x04
	StatusBusy                = 0x08
	StatusReservationConflict = 0x18
	StatusTaskSetFull         = 0x28
	StatusACAActive           = 0x30
	StatusTaskAborted         = 0x40
)

// SenseKey represents a SCSI sense key per SPC-4 Table 28.
type SenseKey uint8

// All 14 defined sense keys.
const (
	SenseNoSense         SenseKey = 0x00
	SenseRecoveredError  SenseKey = 0x01
	SenseNotReady        SenseKey = 0x02
	SenseMediumError     SenseKey = 0x03
	SenseHardwareError   SenseKey = 0x04
	SenseIllegalRequest  SenseKey = 0x05
	SenseUnitAttention   SenseKey = 0x06
	SenseDataProtect     SenseKey = 0x07
	SenseBlankCheck      SenseKey = 0x08
	SenseVendorSpecific  SenseKey = 0x09
	SenseCopyAborted     SenseKey = 0x0A
	SenseAbortedCommand  SenseKey = 0x0B
	SenseVolumeOverflow  SenseKey = 0x0D
	SenseMiscompare      SenseKey = 0x0E
)

var senseKeyNames = [16]string{
	0x00: "NO SENSE",
	0x01: "RECOVERED ERROR",
	0x02: "NOT READY",
	0x03: "MEDIUM ERROR",
	0x04: "HARDWARE ERROR",
	0x05: "ILLEGAL REQUEST",
	0x06: "UNIT ATTENTION",
	0x07: "DATA PROTECT",
	0x08: "BLANK CHECK",
	0x09: "VENDOR SPECIFIC",
	0x0A: "COPY ABORTED",
	0x0B: "ABORTED COMMAND",
	0x0C: "", // reserved
	0x0D: "VOLUME OVERFLOW",
	0x0E: "MISCOMPARE",
	0x0F: "", // reserved
}

// String returns the human-readable name of the sense key.
func (sk SenseKey) String() string {
	if int(sk) < len(senseKeyNames) && senseKeyNames[sk] != "" {
		return senseKeyNames[sk]
	}
	return fmt.Sprintf("UNKNOWN SENSE KEY (0x%02X)", uint8(sk))
}

// Option configures optional flags on SCSI CDB builders.
type Option func(*options)

type options struct {
	fua, dpo, immed, anchor, unmap, ndob, dbd bool
	bytchk                                     uint8
	pageControl                                uint8
}

// WithFUA sets the Force Unit Access flag.
func WithFUA() Option { return func(o *options) { o.fua = true } }

// WithDPO sets the Disable Page Out flag.
func WithDPO() Option { return func(o *options) { o.dpo = true } }

// WithImmed sets the Immediate flag.
func WithImmed() Option { return func(o *options) { o.immed = true } }

// WithAnchor sets the Anchor flag.
func WithAnchor() Option { return func(o *options) { o.anchor = true } }

// WithUnmap sets the Unmap flag.
func WithUnmap() Option { return func(o *options) { o.unmap = true } }

// WithNDOB sets the No Data-Out Buffer flag.
func WithNDOB() Option { return func(o *options) { o.ndob = true } }

// WithBytchk sets the byte check mode for VERIFY commands.
func WithBytchk(v uint8) Option { return func(o *options) { o.bytchk = v } }

// WithDBD sets the Disable Block Descriptors flag for MODE SENSE.
func WithDBD() Option { return func(o *options) { o.dbd = true } }

// WithPageControl sets the page control field (bits 7-6) for MODE SENSE.
func WithPageControl(pc uint8) Option { return func(o *options) { o.pageControl = pc & 0x03 } }

func applyOptions(opts []Option) options {
	var o options
	for _, fn := range opts {
		fn(&o)
	}
	return o
}

// CommandError represents a SCSI command failure with status and optional
// sense data.
type CommandError struct {
	Status uint8
	Sense  *SenseData
}

// Error returns a human-readable description of the SCSI error.
func (e *CommandError) Error() string {
	if e.Sense != nil {
		return fmt.Sprintf("scsi: status 0x%02X: %s", e.Status, e.Sense.String())
	}
	return fmt.Sprintf("scsi: status 0x%02X", e.Status)
}

// IsSenseKey reports whether err is a CommandError with the given sense key.
// It uses errors.As to unwrap wrapped errors.
func IsSenseKey(err error, key SenseKey) bool {
	var ce *CommandError
	if errors.As(err, &ce) && ce.Sense != nil {
		return ce.Sense.Key == key
	}
	return false
}

// checkResult validates a session.Result and returns the response data bytes.
// If the command failed at the transport level, it returns result.Err.
// If the SCSI status is not GOOD, it returns a CommandError with parsed sense.
func checkResult(result session.Result) ([]byte, error) {
	if result.Err != nil {
		return nil, result.Err
	}
	if result.Status != StatusGood {
		ce := &CommandError{Status: result.Status}
		if len(result.SenseData) > 0 {
			sd, err := ParseSense(result.SenseData)
			if err == nil {
				ce.Sense = sd
			}
		}
		return nil, ce
	}
	if result.Data == nil {
		return nil, nil
	}
	return io.ReadAll(result.Data)
}
