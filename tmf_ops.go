package uiscsi

import "context"

// TMFOps provides task management function methods. Obtain via [Session.TMF].
type TMFOps struct {
	s *Session
}

// AbortTask aborts a single task identified by its initiator task tag.
func (o *TMFOps) AbortTask(ctx context.Context, taskTag uint32) (*TMFResult, error) {
	r, err := o.s.s.AbortTask(ctx, taskTag)
	if err != nil {
		return nil, wrapTransportError("tmf", err)
	}
	return convertTMFResult(r), nil
}

// AbortTaskSet aborts all tasks on the specified LUN.
func (o *TMFOps) AbortTaskSet(ctx context.Context, lun uint64) (*TMFResult, error) {
	r, err := o.s.s.AbortTaskSet(ctx, lun)
	if err != nil {
		return nil, wrapTransportError("tmf", err)
	}
	return convertTMFResult(r), nil
}

// ClearTaskSet clears all tasks on the specified LUN.
func (o *TMFOps) ClearTaskSet(ctx context.Context, lun uint64) (*TMFResult, error) {
	r, err := o.s.s.ClearTaskSet(ctx, lun)
	if err != nil {
		return nil, wrapTransportError("tmf", err)
	}
	return convertTMFResult(r), nil
}

// LUNReset resets the specified LUN.
func (o *TMFOps) LUNReset(ctx context.Context, lun uint64) (*TMFResult, error) {
	r, err := o.s.s.LUNReset(ctx, lun)
	if err != nil {
		return nil, wrapTransportError("tmf", err)
	}
	return convertTMFResult(r), nil
}

// TargetWarmReset performs a target warm reset.
func (o *TMFOps) TargetWarmReset(ctx context.Context) (*TMFResult, error) {
	r, err := o.s.s.TargetWarmReset(ctx)
	if err != nil {
		return nil, wrapTransportError("tmf", err)
	}
	return convertTMFResult(r), nil
}

// TargetColdReset performs a target cold reset.
func (o *TMFOps) TargetColdReset(ctx context.Context) (*TMFResult, error) {
	r, err := o.s.s.TargetColdReset(ctx)
	if err != nil {
		return nil, wrapTransportError("tmf", err)
	}
	return convertTMFResult(r), nil
}
