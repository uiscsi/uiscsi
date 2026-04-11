package uiscsi

import (
	"context"
	"fmt"
	"io"

	"github.com/uiscsi/uiscsi/internal/scsi"
	"github.com/uiscsi/uiscsi/internal/session"
)

// Session represents an active iSCSI session. It wraps the internal session
// and provides grouped APIs for SCSI commands, task management, raw CDB
// pass-through, and protocol operations.
//
// Use the accessor methods to access each API group:
//
//	sess.SCSI()     — typed SCSI commands (Inquiry, ReadBlocks, ModeSelect, etc.)
//	sess.TMF()      — task management (AbortTask, LUNReset, etc.)
//	sess.Raw()      — raw CDB pass-through (Execute, StreamExecute)
//	sess.Protocol() — low-level iSCSI protocol (Logout, SendExpStatSNConfirmation)
type Session struct {
	s    *session.Session
	scsi SCSIOps
	tmf  TMFOps
	raw  RawOps
	prot ProtocolOps
}

// initOps sets up the accessor structs after session creation.
func (s *Session) initOps() {
	s.scsi = SCSIOps{s: s.s}
	s.tmf = TMFOps{s: s}
	s.raw = RawOps{s: s}
	s.prot = ProtocolOps{s: s}
}

// Close shuts down the session, performing a graceful logout if possible.
func (s *Session) Close() error {
	return s.s.Close()
}

// SCSI returns the typed SCSI command interface.
func (s *Session) SCSI() *SCSIOps { return &s.scsi }

// TMF returns the task management function interface.
func (s *Session) TMF() *TMFOps { return &s.tmf }

// Raw returns the raw CDB pass-through interface.
func (s *Session) Raw() *RawOps { return &s.raw }

// Protocol returns the low-level iSCSI protocol interface.
func (s *Session) Protocol() *ProtocolOps { return &s.prot }

// ── Internal helpers used by all Ops types ────────────────────────────

// submitAndWait submits a command and waits for the result.
func submitAndWait(ss *session.Session, ctx context.Context, cmd session.Command) (session.Result, error) {
	resultCh, err := ss.Submit(ctx, cmd)
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
func submitAndCheck(ss *session.Session, ctx context.Context, cmd session.Command) ([]byte, error) {
	result, err := submitAndWait(ss, ctx, cmd)
	if err != nil {
		return nil, err
	}
	if result.Err != nil {
		return nil, wrapTransportError("command", result.Err)
	}
	if result.Status != 0 {
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
	if result.Overflow {
		return nil, &SCSIError{
			Status:  result.Status,
			Message: fmt.Sprintf("residual overflow: %d bytes", result.ResidualCount),
		}
	}
	if result.Data != nil {
		return io.ReadAll(result.Data)
	}
	return nil, nil
}

// ── Deprecated flat methods (forward to grouped APIs) ─────────────────
//
// These methods are kept for backward compatibility. Use the grouped
// accessor methods (SCSI(), TMF(), Raw(), Protocol()) instead.

// Deprecated: Use sess.SCSI().ReadBlocks.
func (s *Session) ReadBlocks(ctx context.Context, lun uint64, lba uint64, blocks uint32, blockSize uint32) ([]byte, error) {
	return s.scsi.ReadBlocks(ctx, lun, lba, blocks, blockSize)
}

// Deprecated: Use sess.SCSI().WriteBlocks.
func (s *Session) WriteBlocks(ctx context.Context, lun uint64, lba uint64, blocks uint32, blockSize uint32, data []byte) error {
	return s.scsi.WriteBlocks(ctx, lun, lba, blocks, blockSize, data)
}

// Deprecated: Use sess.SCSI().Inquiry.
func (s *Session) Inquiry(ctx context.Context, lun uint64) (*InquiryData, error) {
	return s.scsi.Inquiry(ctx, lun)
}

// Deprecated: Use sess.SCSI().ReadCapacity.
func (s *Session) ReadCapacity(ctx context.Context, lun uint64) (*Capacity, error) {
	return s.scsi.ReadCapacity(ctx, lun)
}

// Deprecated: Use sess.SCSI().TestUnitReady.
func (s *Session) TestUnitReady(ctx context.Context, lun uint64) error {
	return s.scsi.TestUnitReady(ctx, lun)
}

// Deprecated: Use sess.SCSI().RequestSense.
func (s *Session) RequestSense(ctx context.Context, lun uint64) (*SenseInfo, error) {
	return s.scsi.RequestSense(ctx, lun)
}

// Deprecated: Use sess.SCSI().ReportLuns.
func (s *Session) ReportLuns(ctx context.Context) ([]uint64, error) {
	return s.scsi.ReportLuns(ctx)
}

// Deprecated: Use sess.SCSI().ModeSense6.
func (s *Session) ModeSense6(ctx context.Context, lun uint64, pageCode, subPageCode uint8) ([]byte, error) {
	return s.scsi.ModeSense6(ctx, lun, pageCode, subPageCode)
}

// Deprecated: Use sess.SCSI().ModeSense10.
func (s *Session) ModeSense10(ctx context.Context, lun uint64, pageCode, subPageCode uint8) ([]byte, error) {
	return s.scsi.ModeSense10(ctx, lun, pageCode, subPageCode)
}

// Deprecated: Use sess.SCSI().SynchronizeCache.
func (s *Session) SynchronizeCache(ctx context.Context, lun uint64) error {
	return s.scsi.SynchronizeCache(ctx, lun)
}

// Deprecated: Use sess.SCSI().Verify.
func (s *Session) Verify(ctx context.Context, lun uint64, lba uint64, blocks uint32) error {
	return s.scsi.Verify(ctx, lun, lba, blocks)
}

// Deprecated: Use sess.SCSI().WriteSame.
func (s *Session) WriteSame(ctx context.Context, lun uint64, lba uint64, blocks uint32, blockSize uint32, data []byte) error {
	return s.scsi.WriteSame(ctx, lun, lba, blocks, blockSize, data)
}

// Deprecated: Use sess.SCSI().Unmap.
func (s *Session) Unmap(ctx context.Context, lun uint64, descriptors []UnmapBlockDescriptor) error {
	return s.scsi.Unmap(ctx, lun, descriptors)
}

// Deprecated: Use sess.SCSI().CompareAndWrite.
func (s *Session) CompareAndWrite(ctx context.Context, lun uint64, lba uint64, blocks uint8, blockSize uint32, data []byte) error {
	return s.scsi.CompareAndWrite(ctx, lun, lba, blocks, blockSize, data)
}

// Deprecated: Use sess.SCSI().StartStopUnit.
func (s *Session) StartStopUnit(ctx context.Context, lun uint64, powerCondition uint8, start, loadEject bool) error {
	return s.scsi.StartStopUnit(ctx, lun, powerCondition, start, loadEject)
}

// Deprecated: Use sess.SCSI().PersistReserveIn.
func (s *Session) PersistReserveIn(ctx context.Context, lun uint64, serviceAction uint8) ([]byte, error) {
	return s.scsi.PersistReserveIn(ctx, lun, serviceAction)
}

// Deprecated: Use sess.SCSI().PersistReserveOut.
func (s *Session) PersistReserveOut(ctx context.Context, lun uint64, serviceAction, scopeType uint8, key, saKey uint64) error {
	return s.scsi.PersistReserveOut(ctx, lun, serviceAction, scopeType, key, saKey)
}

// Deprecated: Use sess.Protocol().Logout.
func (s *Session) Logout(ctx context.Context) error {
	return s.prot.Logout(ctx)
}

// Deprecated: Use sess.Protocol().SendExpStatSNConfirmation.
func (s *Session) SendExpStatSNConfirmation() error {
	return s.prot.SendExpStatSNConfirmation()
}

// Deprecated: Use sess.TMF().AbortTask.
func (s *Session) AbortTask(ctx context.Context, taskTag uint32) (*TMFResult, error) {
	return s.tmf.AbortTask(ctx, taskTag)
}

// Deprecated: Use sess.TMF().AbortTaskSet.
func (s *Session) AbortTaskSet(ctx context.Context, lun uint64) (*TMFResult, error) {
	return s.tmf.AbortTaskSet(ctx, lun)
}

// Deprecated: Use sess.TMF().ClearTaskSet.
func (s *Session) ClearTaskSet(ctx context.Context, lun uint64) (*TMFResult, error) {
	return s.tmf.ClearTaskSet(ctx, lun)
}

// Deprecated: Use sess.TMF().LUNReset.
func (s *Session) LUNReset(ctx context.Context, lun uint64) (*TMFResult, error) {
	return s.tmf.LUNReset(ctx, lun)
}

// Deprecated: Use sess.TMF().TargetWarmReset.
func (s *Session) TargetWarmReset(ctx context.Context) (*TMFResult, error) {
	return s.tmf.TargetWarmReset(ctx)
}

// Deprecated: Use sess.TMF().TargetColdReset.
func (s *Session) TargetColdReset(ctx context.Context) (*TMFResult, error) {
	return s.tmf.TargetColdReset(ctx)
}

// Deprecated: Use sess.Raw().Execute.
func (s *Session) Execute(ctx context.Context, lun uint64, cdb []byte, opts ...ExecuteOption) (*RawResult, error) {
	return s.execute(ctx, lun, cdb, opts...)
}

// Deprecated: Use sess.Raw().StreamExecute.
func (s *Session) StreamExecute(ctx context.Context, lun uint64, cdb []byte, opts ...ExecuteOption) (*StreamResult, error) {
	return s.streamExecute(ctx, lun, cdb, opts...)
}
