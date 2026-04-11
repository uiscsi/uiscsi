package session

import (
	"context"
	"fmt"
	"time"

	"github.com/uiscsi/uiscsi/internal/pdu"
	"github.com/uiscsi/uiscsi/internal/transport"
)

// sendTMF sends a Task Management Function request and waits for the response.
// TMF requests are always immediate (do not consume CmdSN window slots) per
// RFC 7143 Section 11.5.
func (s *Session) sendTMF(ctx context.Context, fn uint8, refTaskTag uint32, lun uint64) (*TMFResult, error) {
	// Register ITT for single-shot TMF response.
	itt, respCh := s.router.Register()

	// Build TaskMgmtReq PDU.
	// TMF is always immediate -- use s.window.current() not acquire() (Pitfall 7).
	tmfReq := &pdu.TaskMgmtReq{
		Header: pdu.Header{
			Immediate:        true,
			Final:            true,
			InitiatorTaskTag: itt,
		},
		Function:          fn,
		ReferencedTaskTag: refTaskTag,
		CmdSN:             s.window.current(),
		ExpStatSN:         s.getExpStatSN(),
	}

	// Set LUN in header for LUN-scoped TMFs (SAM-5 encoding).
	tmfReq.LUN = pdu.EncodeSAMLUN(lun)

	bhs, err := tmfReq.MarshalBHS()
	if err != nil {
		s.router.Unregister(itt)
		return nil, fmt.Errorf("session: encode TaskMgmtReq: %w", err)
	}

	raw := &transport.RawPDU{BHS: bhs}
	s.stampDigests(raw)

	// Send to write pump.
	select {
	case s.writeCh <- raw:
	case <-ctx.Done():
		s.router.Unregister(itt)
		return nil, ctx.Err()
	}

	// Wait for TaskMgmtResp with timeout.
	timeout := 30 * time.Second
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case resp := <-respCh:
		tmfResp := &pdu.TaskMgmtResp{}
		tmfResp.UnmarshalBHS(resp.BHS)
		s.window.update(tmfResp.ExpCmdSN, tmfResp.MaxCmdSN)
		s.updateStatSN(tmfResp.StatSN)
		return &TMFResult{Response: tmfResp.Response}, nil

	case <-timer.C:
		s.router.Unregister(itt)
		return nil, fmt.Errorf("session: TMF response timeout after %v", timeout)

	case <-ctx.Done():
		s.router.Unregister(itt)
		return nil, ctx.Err()
	}
}

// AbortTask sends a TMF ABORT TASK request for the specified initiator task tag.
// On success (TMFRespComplete), the aborted task's result channel receives
// Result{Err: ErrTaskAborted}. RFC 7143 Section 11.5.1, Function=1.
func (s *Session) AbortTask(ctx context.Context, targetITT uint32) (*TMFResult, error) {
	result, err := s.sendTMF(ctx, TMFAbortTask, targetITT, 0)
	if err != nil {
		return nil, err
	}
	if result.Response == TMFRespComplete {
		s.cleanupAbortedTask(targetITT)
	}
	return result, nil
}

// AbortTaskSet sends a TMF ABORT TASK SET request for the specified LUN.
// On success, all tasks on that LUN are cancelled with ErrTaskAborted.
// RFC 7143 Section 11.5.1, Function=2.
func (s *Session) AbortTaskSet(ctx context.Context, lun uint64) (*TMFResult, error) {
	result, err := s.sendTMF(ctx, TMFAbortTaskSet, 0, lun)
	if err != nil {
		return nil, err
	}
	if result.Response == TMFRespComplete {
		s.cleanupTasksByLUN(lun)
	}
	return result, nil
}

// ClearTaskSet sends a TMF CLEAR TASK SET request for the specified LUN.
// On success, all tasks on that LUN are cancelled with ErrTaskAborted.
// RFC 7143 Section 11.5.1, Function=3.
func (s *Session) ClearTaskSet(ctx context.Context, lun uint64) (*TMFResult, error) {
	result, err := s.sendTMF(ctx, TMFClearTaskSet, 0, lun)
	if err != nil {
		return nil, err
	}
	if result.Response == TMFRespComplete {
		s.cleanupTasksByLUN(lun)
	}
	return result, nil
}

// LUNReset sends a TMF LOGICAL UNIT RESET request for the specified LUN.
// On success, all tasks on that LUN are cancelled with ErrTaskAborted.
// RFC 7143 Section 11.5.1, Function=5.
func (s *Session) LUNReset(ctx context.Context, lun uint64) (*TMFResult, error) {
	result, err := s.sendTMF(ctx, TMFLogicalUnitReset, 0, lun)
	if err != nil {
		return nil, err
	}
	if result.Response == TMFRespComplete {
		s.cleanupTasksByLUN(lun)
	}
	return result, nil
}

// TargetWarmReset sends a TMF TARGET WARM RESET request.
// RFC 7143 Section 11.5.1, Function=6.
func (s *Session) TargetWarmReset(ctx context.Context) (*TMFResult, error) {
	return s.sendTMF(ctx, TMFTargetWarmReset, 0, 0)
}

// TargetColdReset sends a TMF TARGET COLD RESET request.
// RFC 7143 Section 11.5.1, Function=7.
func (s *Session) TargetColdReset(ctx context.Context) (*TMFResult, error) {
	return s.sendTMF(ctx, TMFTargetColdReset, 0, 0)
}

// cleanupAbortedTask removes a single task that was aborted via TMF.
// Per Pitfall 2: Unregister ITT from Router FIRST, then cancel, then delete.
func (s *Session) cleanupAbortedTask(itt uint32) {
	s.router.Unregister(itt)
	s.mu.Lock()
	tk, ok := s.tasks[itt]
	if ok {
		delete(s.tasks, itt)
	}
	s.mu.Unlock()
	if ok {
		tk.cancel(ErrTaskAborted)
	}
}

// cleanupTasksByLUN cancels all in-flight tasks matching the given LUN.
// Per Pitfall 8: snapshot matching ITTs first, then clean each one to
// avoid holding the lock during potentially blocking cancel operations.
func (s *Session) cleanupTasksByLUN(lun uint64) {
	s.mu.Lock()
	var toClean []uint32
	for itt, tk := range s.tasks {
		if tk.lun == lun {
			toClean = append(toClean, itt)
		}
	}
	s.mu.Unlock()

	for _, itt := range toClean {
		s.cleanupAbortedTask(itt)
	}
}
