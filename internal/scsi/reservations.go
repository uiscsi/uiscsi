package scsi

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/rkujawa/uiscsi/internal/session"
)

// PERSISTENT RESERVE IN service action codes per SPC-4.
const (
	PRInReadKeys           = 0x00
	PRInReadReservation    = 0x01
	PRInReportCapabilities = 0x02
	PRInReadFullStatus     = 0x03
)

// PERSISTENT RESERVE OUT service action codes per SPC-4.
const (
	PROutRegister          = 0x00
	PROutReserve           = 0x01
	PROutRelease           = 0x02
	PROutClear             = 0x03
	PROutPreempt           = 0x04
	PROutPreemptAndAbort   = 0x05
	PROutRegisterAndIgnore = 0x06
	PROutRegisterAndMove   = 0x07
)

// PersistReserveIn returns a PERSISTENT RESERVE IN command (opcode 0x5E).
// serviceAction selects the sub-command (READ KEYS, READ RESERVATION, etc.).
// allocLen specifies the maximum response size in bytes.
func PersistReserveIn(lun uint64, serviceAction uint8, allocLen uint16) session.Command {
	var cmd session.Command
	cmd.CDB[0] = OpPersistReserveIn
	cmd.CDB[1] = serviceAction & 0x1F
	binary.BigEndian.PutUint16(cmd.CDB[7:9], allocLen)
	cmd.Read = true
	cmd.ExpectedDataTransferLen = uint32(allocLen)
	cmd.LUN = lun
	return cmd
}

// PRKeysResponse holds the result of a PERSISTENT RESERVE IN READ KEYS.
type PRKeysResponse struct {
	Generation uint32
	Keys       []uint64
}

// ParsePersistReserveInKeys parses a PERSISTENT RESERVE IN READ KEYS response.
func ParsePersistReserveInKeys(result session.Result) (*PRKeysResponse, error) {
	data, err := checkResult(result)
	if err != nil {
		return nil, err
	}
	if len(data) < 8 {
		return nil, fmt.Errorf("scsi: PR IN READ KEYS response too short (%d bytes, need 8)", len(data))
	}

	resp := &PRKeysResponse{
		Generation: binary.BigEndian.Uint32(data[0:4]),
	}
	additionalLen := binary.BigEndian.Uint32(data[4:8])
	numKeys := additionalLen / 8

	resp.Keys = make([]uint64, 0, numKeys)
	for i := uint32(0); i < numKeys; i++ {
		off := 8 + i*8
		if off+8 > uint32(len(data)) {
			break
		}
		resp.Keys = append(resp.Keys, binary.BigEndian.Uint64(data[off:off+8]))
	}
	return resp, nil
}

// PRReservation holds the result of a PERSISTENT RESERVE IN READ RESERVATION.
type PRReservation struct {
	Key       uint64
	ScopeType uint8 // scope in bits 7-4, type in bits 3-0
}

// ParsePersistReserveInReservation parses a PERSISTENT RESERVE IN READ
// RESERVATION response. Returns nil (no error) if no reservation is held.
func ParsePersistReserveInReservation(result session.Result) (*PRReservation, error) {
	data, err := checkResult(result)
	if err != nil {
		return nil, err
	}
	if len(data) < 8 {
		return nil, fmt.Errorf("scsi: PR IN READ RESERVATION response too short (%d bytes, need 8)", len(data))
	}

	additionalLen := binary.BigEndian.Uint32(data[4:8])
	if additionalLen == 0 {
		// No reservation held
		return nil, nil
	}
	if len(data) < 24 {
		return nil, fmt.Errorf("scsi: PR IN READ RESERVATION response too short for descriptor (%d bytes, need 24)", len(data))
	}

	return &PRReservation{
		Key:       binary.BigEndian.Uint64(data[8:16]),
		ScopeType: data[21],
	}, nil
}

// PersistReserveOut returns a PERSISTENT RESERVE OUT command (opcode 0x5F).
// serviceAction selects the sub-command (REGISTER, RESERVE, RELEASE, etc.).
// scopeType encodes the scope (bits 7-4) and type (bits 3-0).
// key is the reservation key; saKey is the service action reservation key.
// The 24-byte parameter data is automatically constructed per SPC-4.
func PersistReserveOut(lun uint64, serviceAction uint8, scopeType uint8, key, saKey uint64) session.Command {
	// Build 24-byte parameter data
	paramData := make([]byte, 24)
	binary.BigEndian.PutUint64(paramData[0:8], key)
	binary.BigEndian.PutUint64(paramData[8:16], saKey)
	// bytes 16-23 remain zero (scope-specific address, aptpl, etc.)

	var cmd session.Command
	cmd.CDB[0] = OpPersistReserveOut
	cmd.CDB[1] = serviceAction & 0x1F
	cmd.CDB[2] = scopeType
	binary.BigEndian.PutUint32(cmd.CDB[5:9], 24) // parameter list length
	cmd.Write = true
	cmd.Data = bytes.NewReader(paramData)
	cmd.ExpectedDataTransferLen = 24
	cmd.LUN = lun
	return cmd
}
