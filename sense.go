// sense.go provides public sense data parsing and status checking helpers
// for callers of Execute and StreamExecute.
package uiscsi

import (
	"fmt"

	"github.com/uiscsi/uiscsi/internal/scsi"
)

// ParseSenseData parses raw SCSI sense bytes (as returned in
// [RawResult.SenseData] or [StreamResult.Wait]) into a [SenseInfo].
// Returns nil, nil if data is empty. Returns an error if the sense
// format is unrecognizable.
//
// This is the same parser used internally by the typed Session methods.
// It supports both fixed (0x70/0x71) and descriptor (0x72/0x73) formats
// per SPC-4 Section 4.5.
func ParseSenseData(raw []byte) (*SenseInfo, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	sd, err := scsi.ParseSense(raw)
	if err != nil {
		return nil, err
	}
	return convertSense(sd), nil
}

// CheckStatus inspects a SCSI status byte and optional raw sense data,
// returning nil for GOOD status (0x00) and a [*SCSIError] for anything
// else. This is the same interpretation that typed Session methods
// ([Session.ReadBlocks], [Session.Inquiry], etc.) apply internally.
//
// Use this with [Session.Execute] or [StreamResult.Wait] when you want
// the same error-wrapping behavior without reimplementing it:
//
//	result, err := sess.Execute(ctx, lun, cdb, uiscsi.WithDataIn(256))
//	if err != nil { return err }
//	if err := uiscsi.CheckStatus(result.Status, result.SenseData); err != nil {
//	    var se *uiscsi.SCSIError
//	    if errors.As(err, &se) { /* se.SenseKey, se.ASC, se.ASCQ */ }
//	    return err
//	}
func CheckStatus(status uint8, senseData []byte) error {
	if status == 0 {
		return nil
	}
	se := &SCSIError{Status: status}
	if len(senseData) > 0 {
		sd, parseErr := scsi.ParseSense(senseData)
		if parseErr == nil {
			se.SenseKey = uint8(sd.Key)
			se.ASC = sd.ASC
			se.ASCQ = sd.ASCQ
			se.Message = sd.String()
		} else {
			se.Message = fmt.Sprintf("sense data present but unparseable: %v", parseErr)
		}
	}
	return se
}
