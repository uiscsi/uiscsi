// session.go implements the Session type and all SCSI command methods.
package uiscsi

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/rkujawa/uiscsi/internal/scsi"
	"github.com/rkujawa/uiscsi/internal/session"
)

// Session represents an active iSCSI session. It wraps the internal session
// and provides typed SCSI command methods, task management, and raw CDB
// pass-through.
type Session struct {
	s *session.Session
}

// Close shuts down the session, performing a graceful logout if possible.
func (s *Session) Close() error {
	return s.s.Close()
}

// submitAndWait submits a command and waits for the result.
func (s *Session) submitAndWait(ctx context.Context, cmd session.Command) (session.Result, error) {
	resultCh, err := s.s.Submit(ctx, cmd)
	if err != nil {
		return session.Result{}, wrapTransportError("submit", err)
	}

	select {
	case result := <-resultCh:
		return result, nil
	case <-ctx.Done():
		return session.Result{}, ctx.Err()
	}
}

// submitAndCheck submits a command, waits, and checks the result for errors.
// Returns the data bytes on success.
func (s *Session) submitAndCheck(ctx context.Context, cmd session.Command) ([]byte, error) {
	result, err := s.submitAndWait(ctx, cmd)
	if err != nil {
		return nil, err
	}
	if result.Err != nil {
		return nil, wrapTransportError("command", result.Err)
	}
	if result.Status != 0 {
		// Build SCSIError from status + sense.
		se := &SCSIError{Status: result.Status}
		if len(result.SenseData) > 0 {
			sd, parseErr := scsi.ParseSense(result.SenseData)
			if parseErr == nil {
				se.SenseKey = uint8(sd.Key)
				se.ASC = sd.ASC
				se.ASCQ = sd.ASCQ
				se.Message = sd.String()
			} else {
				se.Message = fmt.Sprintf("sense data present but unparseable: %v", parseErr)
			}
		}
		return nil, se
	}
	// Check for residual overflow (target received more data than expected).
	if result.Overflow {
		return nil, &SCSIError{
			Status:  result.Status,
			Message: fmt.Sprintf("residual overflow: %d bytes", result.ResidualCount),
		}
	}
	// Check for residual underflow (less data than expected).
	// Underflow with Status==0 is acceptable (short read); the data reader
	// already contains only the received bytes.
	if result.Data != nil {
		return io.ReadAll(result.Data)
	}
	return nil, nil
}

// ReadBlocks reads blocks from the target. Uses READ(16) for full 64-bit LBA support.
func (s *Session) ReadBlocks(ctx context.Context, lun uint64, lba uint64, blocks uint32, blockSize uint32) ([]byte, error) {
	cmd := scsi.Read16(lun, lba, blocks, blockSize)
	return s.submitAndCheck(ctx, cmd)
}

// WriteBlocks writes blocks to the target. Uses WRITE(16) for full 64-bit LBA support.
func (s *Session) WriteBlocks(ctx context.Context, lun uint64, lba uint64, blocks uint32, blockSize uint32, data []byte) error {
	cmd := scsi.Write16(lun, lba, blocks, blockSize, bytes.NewReader(data))
	_, err := s.submitAndCheck(ctx, cmd)
	return err
}

// Inquiry sends a standard INQUIRY command and returns the parsed response.
func (s *Session) Inquiry(ctx context.Context, lun uint64) (*InquiryData, error) {
	cmd := scsi.Inquiry(lun, 255)
	result, err := s.submitAndWait(ctx, cmd)
	if err != nil {
		return nil, err
	}
	resp, err := scsi.ParseInquiry(result)
	if err != nil {
		return nil, wrapSCSIError(err)
	}
	return convertInquiry(resp), nil
}

// ReadCapacity returns the capacity of the specified LUN.
// It uses READ CAPACITY(16) for full 64-bit LBA support.
func (s *Session) ReadCapacity(ctx context.Context, lun uint64) (*Capacity, error) {
	// Try ReadCapacity16 first (supports >2TB devices).
	cmd := scsi.ReadCapacity16(lun, 32)
	result, err := s.submitAndWait(ctx, cmd)
	if err != nil {
		return nil, err
	}
	resp16, err := scsi.ParseReadCapacity16(result)
	if err == nil {
		return convertCapacity16(resp16), nil
	}
	// Fallback to ReadCapacity10 — some targets (LIO with auto-ACLs)
	// return CHECK CONDITION for SERVICE ACTION IN on non-zero LUNs.
	cmd10 := scsi.ReadCapacity10(lun)
	result10, err := s.submitAndWait(ctx, cmd10)
	if err != nil {
		return nil, err
	}
	resp10, err := scsi.ParseReadCapacity10(result10)
	if err != nil {
		return nil, wrapSCSIError(err)
	}
	return convertCapacity10(resp10), nil
}

// TestUnitReady checks whether the specified LUN is ready.
func (s *Session) TestUnitReady(ctx context.Context, lun uint64) error {
	cmd := scsi.TestUnitReady(lun)
	_, err := s.submitAndCheck(ctx, cmd)
	return err
}

// RequestSense sends a REQUEST SENSE command and returns the parsed sense data.
func (s *Session) RequestSense(ctx context.Context, lun uint64) (*SenseInfo, error) {
	cmd := scsi.RequestSense(lun, 252)
	data, err := s.submitAndCheck(ctx, cmd)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	sd, err := scsi.ParseSense(data)
	if err != nil {
		return nil, err
	}
	return convertSense(sd), nil
}

// ReportLuns returns all LUNs reported by the target.
func (s *Session) ReportLuns(ctx context.Context) ([]uint64, error) {
	cmd := scsi.ReportLuns(1024)
	result, err := s.submitAndWait(ctx, cmd)
	if err != nil {
		return nil, err
	}
	luns, err := scsi.ParseReportLuns(result)
	if err != nil {
		return nil, wrapSCSIError(err)
	}
	return luns, nil
}

// ModeSense6 sends a MODE SENSE(6) command and returns the raw mode page bytes.
func (s *Session) ModeSense6(ctx context.Context, lun uint64, pageCode, subPageCode uint8) ([]byte, error) {
	cmd := scsi.ModeSense6(lun, pageCode, subPageCode, 255)
	return s.submitAndCheck(ctx, cmd)
}

// ModeSense10 sends a MODE SENSE(10) command and returns the raw mode page bytes.
func (s *Session) ModeSense10(ctx context.Context, lun uint64, pageCode, subPageCode uint8) ([]byte, error) {
	cmd := scsi.ModeSense10(lun, pageCode, subPageCode, 1024)
	return s.submitAndCheck(ctx, cmd)
}

// SynchronizeCache flushes the target's volatile cache for the entire LUN.
func (s *Session) SynchronizeCache(ctx context.Context, lun uint64) error {
	cmd := scsi.SynchronizeCache16(lun, 0, 0)
	_, err := s.submitAndCheck(ctx, cmd)
	return err
}

// Verify requests the target to verify the specified LBA range.
func (s *Session) Verify(ctx context.Context, lun uint64, lba uint64, blocks uint32) error {
	cmd := scsi.Verify16(lun, lba, blocks)
	_, err := s.submitAndCheck(ctx, cmd)
	return err
}

// WriteSame writes the same block of data to the specified LBA range.
func (s *Session) WriteSame(ctx context.Context, lun uint64, lba uint64, blocks uint32, blockSize uint32, data []byte) error {
	cmd := scsi.WriteSame16(lun, lba, blocks, blockSize, bytes.NewReader(data))
	_, err := s.submitAndCheck(ctx, cmd)
	return err
}

// Unmap deallocates the specified LBA ranges (thin provisioning).
func (s *Session) Unmap(ctx context.Context, lun uint64, descriptors []UnmapBlockDescriptor) error {
	internal := make([]scsi.UnmapBlockDescriptor, len(descriptors))
	for i, d := range descriptors {
		internal[i] = scsi.UnmapBlockDescriptor{LBA: d.LBA, BlockCount: d.Blocks}
	}
	cmd := scsi.Unmap(lun, internal)
	_, err := s.submitAndCheck(ctx, cmd)
	return err
}

// CompareAndWrite performs an atomic read-compare-write operation.
// data must contain 2*blocks*blockSize bytes: expected data followed by new data.
func (s *Session) CompareAndWrite(ctx context.Context, lun uint64, lba uint64, blocks uint8, blockSize uint32, data []byte) error {
	cmd := scsi.CompareAndWrite(lun, lba, blocks, blockSize, bytes.NewReader(data))
	_, err := s.submitAndCheck(ctx, cmd)
	return err
}

// StartStopUnit sends a START STOP UNIT command.
func (s *Session) StartStopUnit(ctx context.Context, lun uint64, powerCondition uint8, start, loadEject bool) error {
	cmd := scsi.StartStopUnit(lun, powerCondition, start, loadEject)
	_, err := s.submitAndCheck(ctx, cmd)
	return err
}

// PersistReserveIn sends a PERSISTENT RESERVE IN command and returns the raw response.
func (s *Session) PersistReserveIn(ctx context.Context, lun uint64, serviceAction uint8) ([]byte, error) {
	cmd := scsi.PersistReserveIn(lun, serviceAction, 1024)
	return s.submitAndCheck(ctx, cmd)
}

// PersistReserveOut sends a PERSISTENT RESERVE OUT command.
func (s *Session) PersistReserveOut(ctx context.Context, lun uint64, serviceAction, scopeType uint8, key, saKey uint64) error {
	cmd := scsi.PersistReserveOut(lun, serviceAction, scopeType, key, saKey)
	_, err := s.submitAndCheck(ctx, cmd)
	return err
}

// Task Management Functions

// AbortTask aborts a single task identified by its initiator task tag.
func (s *Session) AbortTask(ctx context.Context, taskTag uint32) (*TMFResult, error) {
	r, err := s.s.AbortTask(ctx, taskTag)
	if err != nil {
		return nil, wrapTransportError("tmf", err)
	}
	return convertTMFResult(r), nil
}

// AbortTaskSet aborts all tasks on the specified LUN.
func (s *Session) AbortTaskSet(ctx context.Context, lun uint64) (*TMFResult, error) {
	r, err := s.s.AbortTaskSet(ctx, lun)
	if err != nil {
		return nil, wrapTransportError("tmf", err)
	}
	return convertTMFResult(r), nil
}

// ClearTaskSet clears all tasks on the specified LUN.
func (s *Session) ClearTaskSet(ctx context.Context, lun uint64) (*TMFResult, error) {
	r, err := s.s.ClearTaskSet(ctx, lun)
	if err != nil {
		return nil, wrapTransportError("tmf", err)
	}
	return convertTMFResult(r), nil
}

// LUNReset resets the specified LUN.
func (s *Session) LUNReset(ctx context.Context, lun uint64) (*TMFResult, error) {
	r, err := s.s.LUNReset(ctx, lun)
	if err != nil {
		return nil, wrapTransportError("tmf", err)
	}
	return convertTMFResult(r), nil
}

// TargetWarmReset performs a target warm reset.
func (s *Session) TargetWarmReset(ctx context.Context) (*TMFResult, error) {
	r, err := s.s.TargetWarmReset(ctx)
	if err != nil {
		return nil, wrapTransportError("tmf", err)
	}
	return convertTMFResult(r), nil
}

// TargetColdReset performs a target cold reset.
func (s *Session) TargetColdReset(ctx context.Context) (*TMFResult, error) {
	r, err := s.s.TargetColdReset(ctx)
	if err != nil {
		return nil, wrapTransportError("tmf", err)
	}
	return convertTMFResult(r), nil
}

// ExecuteOption configures raw CDB execution via Execute.
type ExecuteOption func(*executeConfig)

type executeConfig struct {
	dataInLen uint32
	dataOut   io.Reader
	dataOutLen uint32
}

// WithDataIn configures Execute to expect a read response of up to allocLen bytes.
func WithDataIn(allocLen uint32) ExecuteOption {
	return func(c *executeConfig) {
		c.dataInLen = allocLen
	}
}

// WithDataOut configures Execute to send data with the command.
func WithDataOut(r io.Reader, length uint32) ExecuteOption {
	return func(c *executeConfig) {
		c.dataOut = r
		c.dataOutLen = length
	}
}

// Execute sends a raw CDB to the target and returns the raw result.
// This is the low-level pass-through for arbitrary SCSI commands.
func (s *Session) Execute(ctx context.Context, lun uint64, cdb []byte, opts ...ExecuteOption) (*RawResult, error) {
	cfg := &executeConfig{}
	for _, o := range opts {
		o(cfg)
	}

	if len(cdb) == 0 {
		return nil, fmt.Errorf("iscsi execute: empty CDB")
	}
	if len(cdb) > 16 {
		return nil, fmt.Errorf("iscsi execute: CDB length %d exceeds maximum 16 bytes (AHS Extended CDB not yet supported)", len(cdb))
	}

	var cmd session.Command
	copy(cmd.CDB[:len(cdb)], cdb)
	cmd.LUN = lun

	if cfg.dataInLen > 0 {
		cmd.Read = true
		cmd.ExpectedDataTransferLen = cfg.dataInLen
	}
	if cfg.dataOut != nil {
		cmd.Write = true
		cmd.Data = cfg.dataOut
		cmd.ExpectedDataTransferLen = cfg.dataOutLen
	}

	result, err := s.submitAndWait(ctx, cmd)
	if err != nil {
		return nil, err
	}
	if result.Err != nil {
		return nil, wrapTransportError("execute", result.Err)
	}

	rr := &RawResult{
		Status:    result.Status,
		SenseData: result.SenseData,
	}
	if result.Data != nil {
		rr.Data, err = io.ReadAll(result.Data)
		if err != nil {
			return nil, wrapTransportError("read", err)
		}
	}
	return rr, nil
}
