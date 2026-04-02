package session

import (
	"context"
	"fmt"
	"time"

	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
)

// keepaliveLoop sends periodic NOP-Out pings to detect dead connections.
// It runs as a background goroutine started by NewSession. If a NOP-In
// response is not received within the keepalive timeout, the session is
// terminated with a fatal error.
//
// Per RFC 7143 Section 11.18, initiator-originated NOP-Out uses
// ITT != 0xFFFFFFFF and TTT = 0xFFFFFFFF. The target responds with
// NOP-In carrying the same ITT.
func (s *Session) keepaliveLoop(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.keepaliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.sendKeepalivePing(ctx); err != nil {
				if ctx.Err() != nil {
					return
				}
				s.mu.Lock()
				if s.err == nil {
					s.err = fmt.Errorf("session: keepalive: %w", err)
				}
				s.mu.Unlock()
				s.cancel()
				return
			}
		}
	}
}

// sendKeepalivePing sends a single NOP-Out ping and waits for the NOP-In
// response. Returns nil on success or an error if the response times out.
func (s *Session) sendKeepalivePing(ctx context.Context) error {
	// Register ITT for single response.
	itt, respCh := s.router.Register()

	// Build NOP-Out: Immediate=true, Final=true, ITT=allocated,
	// TTT=0xFFFFFFFF (initiator-originated ping).
	// CmdSN is carried but NOT advanced per RFC 7143 Section 3.2.2.
	nopOut := &pdu.NOPOut{
		Header: pdu.Header{
			Immediate:        true,
			Final:            true,
			InitiatorTaskTag: itt,
		},
		TargetTransferTag: 0xFFFFFFFF,
		CmdSN:             s.window.current(),
		ExpStatSN:         s.getExpStatSN(),
	}

	bhs, err := nopOut.MarshalBHS()
	if err != nil {
		s.router.Unregister(itt)
		return fmt.Errorf("encode NOP-Out: %w", err)
	}

	raw := &transport.RawPDU{BHS: bhs}
	s.stampDigests(raw)
	select {
	case s.writeCh <- raw:
	case <-ctx.Done():
		s.router.Unregister(itt)
		return ctx.Err()
	}

	// Wait for NOP-In response with timeout.
	timer := time.NewTimer(s.cfg.keepaliveTimeout)
	defer timer.Stop()

	select {
	case resp := <-respCh:
		// Decode NOP-In and update sequence numbers.
		nopin := &pdu.NOPIn{}
		nopin.UnmarshalBHS(resp.BHS)
		s.window.update(nopin.ExpCmdSN, nopin.MaxCmdSN)
		s.updateStatSN(nopin.StatSN)
		return nil

	case <-timer.C:
		s.router.Unregister(itt)
		return fmt.Errorf("NOP-In response timeout after %v", s.cfg.keepaliveTimeout)

	case <-ctx.Done():
		s.router.Unregister(itt)
		return ctx.Err()
	}
}

// handleUnsolicitedNOPIn processes an unsolicited NOP-In PDU
// (ITT=0xFFFFFFFF) received from the target via the unsolicited channel.
//
// Two sub-cases per RFC 7143 Section 11.19:
//   - TTT != 0xFFFFFFFF: target-initiated ping. Respond with NOP-Out
//     echoing the TTT and setting ITT=0xFFFFFFFF.
//   - TTT == 0xFFFFFFFF: informational. Just update sequence numbers.
func (s *Session) handleUnsolicitedNOPIn(raw *transport.RawPDU) {
	nopin := &pdu.NOPIn{}
	nopin.UnmarshalBHS(raw.BHS)

	// Update sequence numbers from NOP-In.
	s.window.update(nopin.ExpCmdSN, nopin.MaxCmdSN)
	s.updateStatSN(nopin.StatSN)

	if nopin.TargetTransferTag != 0xFFFFFFFF {
		// Target-initiated ping: respond with NOP-Out echoing TTT.
		nopOut := &pdu.NOPOut{
			Header: pdu.Header{
				Immediate:        true,
				Final:            true,
				InitiatorTaskTag: 0xFFFFFFFF, // response, not new task
			},
			TargetTransferTag: nopin.TargetTransferTag,
			CmdSN:             s.window.current(),
			ExpStatSN:         s.getExpStatSN(),
		}

		// Echo ping data if present.
		if len(raw.DataSegment) > 0 {
			nopOut.Data = raw.DataSegment
			nopOut.Header.DataSegmentLen = uint32(len(raw.DataSegment))
		}

		bhs, err := nopOut.MarshalBHS()
		if err != nil {
			s.cfg.logger.Warn("session: encode NOP-Out response", "error", err)
			return
		}

		resp := &transport.RawPDU{BHS: bhs}
		if len(nopOut.Data) > 0 {
			resp.DataSegment = nopOut.Data
		}
		s.stampDigests(resp)

		select {
		case s.writeCh <- resp:
		default:
			s.cfg.logger.Warn("session: write channel full, dropping NOP-Out response")
		}
	}
}
