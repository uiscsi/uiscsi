package session

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/uiscsi/uiscsi/internal/login"
	"github.com/uiscsi/uiscsi/internal/pdu"
	"github.com/uiscsi/uiscsi/internal/transport"
)

// handleAsyncMsg processes an AsyncMsg PDU from the target.
// Per RFC 7143 Section 11.9, async messages carry event codes that
// signal various target state changes.
func (s *Session) handleAsyncMsg(raw *transport.RawPDU) {
	async := &pdu.AsyncMsg{}
	async.UnmarshalBHS(raw.BHS)
	async.Data = raw.DataSegment

	// Update sequence numbers.
	s.window.update(async.ExpCmdSN, async.MaxCmdSN)
	s.updateStatSN(async.StatSN)

	evt := AsyncEvent{
		EventCode:  async.AsyncEvent,
		VendorCode: async.AsyncVCode,
		Parameter1: async.Parameter1,
		Parameter2: async.Parameter2,
		Parameter3: async.Parameter3,
		Data:       async.Data,
	}

	switch async.AsyncEvent {
	case 0:
		// SCSI async event: dispatch with sense data.
		s.dispatchAsyncEvent(evt)

	case 1:
		// Target requests logout.
		go s.handleTargetRequestedLogout(evt)
		s.dispatchAsyncEvent(evt)

	case 2:
		// Connection drop notification.
		s.dispatchAsyncEvent(evt)
		if s.targetAddr != "" {
			s.triggerReconnect(fmt.Errorf("session: target reported connection drop (AsyncEvent 2)"))
		} else {
			s.mu.Lock()
			if s.err == nil {
				s.err = fmt.Errorf("session: target reported connection drop (AsyncEvent 2)")
			}
			s.mu.Unlock()
		}

	case 3:
		// Session drop notification.
		s.dispatchAsyncEvent(evt)
		s.mu.Lock()
		if s.err == nil {
			s.err = fmt.Errorf("session: target reported session drop (AsyncEvent 3)")
		}
		s.mu.Unlock()
		s.cancel()

	case 4:
		// Negotiation request: renegotiate within Parameter3 seconds.
		s.dispatchAsyncEvent(evt)
		go func() {
			deadline := time.Duration(async.Parameter3) * time.Second
			if deadline <= 0 {
				deadline = 30 * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), deadline)
			defer cancel()
			if err := s.renegotiate(ctx); err != nil {
				s.cfg.logger.Warn("session: renegotiation failed", "error", err)
			}
		}()

	default:
		// Vendor-specific (255) and all others.
		s.dispatchAsyncEvent(evt)
	}
}

// dispatchAsyncEvent invokes the user-provided async handler callback.
// If no handler is registered, the event is logged and discarded.
func (s *Session) dispatchAsyncEvent(evt AsyncEvent) {
	if s.cfg.asyncHandler != nil {
		s.cfg.asyncHandler(context.Background(), evt)
		return
	}
	s.cfg.logger.Warn("session: async event received but no handler registered",
		"event_code", evt.EventCode)
}

// handleTargetRequestedLogout handles an AsyncMsg with EventCode 1 (target
// requests logout). Per RFC 7143 S11.9.1 and FFP #14.1, the initiator MUST
// logout within Parameter3 seconds. DefaultTime2Wait is the delay before
// logout CAN start, used only if it fits within the deadline.
func (s *Session) handleTargetRequestedLogout(evt AsyncEvent) {
	// Parameter3 = seconds by which logout MUST complete (RFC 7143 S11.9.1).
	deadline := time.Duration(evt.Parameter3) * time.Second
	if deadline <= 0 {
		deadline = 30 * time.Second // fallback
	}

	// DefaultTime2Wait is the delay before logout CAN start.
	// Only wait if it fits within the deadline.
	// Use a context-aware timer so Close() can interrupt the wait and
	// so testing/synctest fake time virtualizes the delay (SESS-05).
	waitDuration := time.Duration(s.params.DefaultTime2Wait) * time.Second
	if waitDuration > 0 && waitDuration < deadline {
		timer := time.NewTimer(waitDuration)
		select {
		case <-timer.C:
		case <-s.closed:
			timer.Stop()
			return // session closed during wait
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	if err := s.logout(ctx, 0); err != nil {
		s.cfg.logger.Warn("session: target-requested logout failed", "error", err)
	}
	_ = s.Close()
}

// renegotiate initiates a Text Request exchange to renegotiate operational
// parameters after receiving AsyncEvent code 4. Per RFC 7143 Section 11.9,
// the initiator MUST send a Text Request within Parameter3 seconds.
func (s *Session) renegotiate(ctx context.Context) error {
	// Read params under lock to avoid races with concurrent renegotiations.
	s.mu.Lock()
	maxRecvDSL := s.params.MaxRecvDataSegmentLength
	maxBurst := s.params.MaxBurstLength
	firstBurst := s.params.FirstBurstLength
	s.mu.Unlock()

	data := login.EncodeTextKV([]login.KeyValue{
		{Key: "MaxRecvDataSegmentLength", Value: strconv.Itoa(int(maxRecvDSL))},
		{Key: "MaxBurstLength", Value: strconv.Itoa(int(maxBurst))},
		{Key: "FirstBurstLength", Value: strconv.Itoa(int(firstBurst))},
	})

	cmdSN, err := s.window.acquire(ctx)
	if err != nil {
		return fmt.Errorf("renegotiate: acquire CmdSN: %w", err)
	}

	itt, respCh := s.router.Register()
	textReq := &pdu.TextReq{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: itt,
			DataSegmentLen:   uint32(len(data)),
		},
		TargetTransferTag: 0xFFFFFFFF,
		CmdSN:             cmdSN,
		ExpStatSN:         s.getExpStatSN(),
		Data:              data,
	}

	bhs, err := textReq.MarshalBHS()
	if err != nil {
		s.router.Unregister(itt)
		return fmt.Errorf("renegotiate: encode TextReq: %w", err)
	}

	raw := &transport.RawPDU{BHS: bhs}
	if len(data) > 0 {
		raw.DataSegment = data
	}
	s.stampDigests(raw)

	select {
	case s.writeCh <- raw:
	case <-ctx.Done():
		s.router.Unregister(itt)
		return ctx.Err()
	}

	// Wait for TextResp.
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()

	select {
	case respRaw := <-respCh:
		decoded, decErr := pdu.DecodeBHS(respRaw.BHS)
		if decErr != nil {
			return fmt.Errorf("renegotiate: decode TextResp: %w", decErr)
		}
		textResp, ok := decoded.(*pdu.TextResp)
		if !ok {
			return fmt.Errorf("renegotiate: unexpected PDU type %T", decoded)
		}
		s.window.update(textResp.ExpCmdSN, textResp.MaxCmdSN)
		s.updateStatSN(textResp.StatSN)
		// Parse accepted parameters and update session params if needed.
		if len(respRaw.DataSegment) > 0 {
			kvs := login.DecodeTextKV(respRaw.DataSegment)
			s.applyRenegotiatedParams(kvs)
		}
		return nil
	case <-timer.C:
		s.router.Unregister(itt)
		return fmt.Errorf("renegotiate: TextResp timeout")
	case <-ctx.Done():
		s.router.Unregister(itt)
		return ctx.Err()
	}
}

// applyRenegotiatedParams updates session parameters from TextResp key-values.
// Holds s.mu to avoid races with concurrent renegotiate() reads.
func (s *Session) applyRenegotiatedParams(kvs []login.KeyValue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, kv := range kvs {
		switch kv.Key {
		case "MaxRecvDataSegmentLength":
			if v, err := strconv.ParseUint(kv.Value, 10, 32); err == nil {
				s.params.MaxRecvDataSegmentLength = uint32(v)
			}
		case "MaxBurstLength":
			if v, err := strconv.ParseUint(kv.Value, 10, 32); err == nil {
				s.params.MaxBurstLength = uint32(v)
			}
		case "FirstBurstLength":
			if v, err := strconv.ParseUint(kv.Value, 10, 32); err == nil {
				s.params.FirstBurstLength = uint32(v)
			}
		}
	}
}
