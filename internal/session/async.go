package session

import (
	"context"
	"fmt"
	"time"

	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
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
		// Negotiation request (renegotiation is Phase 6+).
		s.dispatchAsyncEvent(evt)

	default:
		// Vendor-specific (255) and all others.
		s.dispatchAsyncEvent(evt)
	}
}

// dispatchAsyncEvent invokes the user-provided async handler callback.
// If no handler is registered, the event is logged and discarded.
func (s *Session) dispatchAsyncEvent(evt AsyncEvent) {
	if s.cfg.asyncHandler != nil {
		s.cfg.asyncHandler(evt)
		return
	}
	s.cfg.logger.Warn("session: async event received but no handler registered",
		"event_code", evt.EventCode)
}

// handleTargetRequestedLogout handles an AsyncMsg with EventCode 1 (target
// requests logout). Per RFC 7143, the initiator should wait DefaultTime2Wait
// seconds before initiating logout.
func (s *Session) handleTargetRequestedLogout(evt AsyncEvent) {
	waitDuration := time.Duration(s.params.DefaultTime2Wait) * time.Second
	if waitDuration > 0 {
		time.Sleep(waitDuration)
	}

	// Initiate logout with reason 0 (close session).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.logout(ctx, 0); err != nil {
		s.cfg.logger.Warn("session: target-requested logout failed", "error", err)
	}

	// Close session after logout.
	s.Close()
}
