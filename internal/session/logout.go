package session

import (
	"context"
	"fmt"
	"time"

	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
)

// logout performs the Logout PDU exchange per RFC 7143 Section 11.14/11.15.
// reasonCode: 0=close session, 1=close connection, 2=remove connection for recovery.
func (s *Session) logout(ctx context.Context, reasonCode uint8) error {
	// Acquire CmdSN (Logout is a non-immediate command that advances CmdSN).
	cmdSN, err := s.window.acquire(ctx)
	if err != nil {
		return fmt.Errorf("logout: acquire CmdSN: %w", err)
	}

	// Register ITT for single response.
	itt, respCh := s.router.Register()

	// Build LogoutReq PDU.
	logoutReq := &pdu.LogoutReq{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: itt,
		},
		ReasonCode: reasonCode,
		CmdSN:      cmdSN,
		ExpStatSN:  s.getExpStatSN(),
	}

	bhs, encErr := logoutReq.MarshalBHS()
	if encErr != nil {
		s.router.Unregister(itt)
		return fmt.Errorf("logout: encode LogoutReq: %w", encErr)
	}

	raw := &transport.RawPDU{BHS: bhs}
	s.stampDigests(raw)
	select {
	case s.writeCh <- raw:
	case <-ctx.Done():
		s.router.Unregister(itt)
		return ctx.Err()
	}

	// Wait for LogoutResp with timeout.
	// Use DefaultTime2Wait + DefaultTime2Retain as max, capped at 30s.
	// Read params under lock — reconnect() may replace s.params concurrently.
	s.mu.Lock()
	t2w, t2r := s.params.DefaultTime2Wait, s.params.DefaultTime2Retain
	s.mu.Unlock()
	timeout := time.Duration(t2w+t2r) * time.Second
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 30 * time.Second
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case resp := <-respCh:
		logoutResp := &pdu.LogoutResp{}
		logoutResp.UnmarshalBHS(resp.BHS)
		s.window.update(logoutResp.ExpCmdSN, logoutResp.MaxCmdSN)
		s.updateStatSN(logoutResp.StatSN)

		if logoutResp.Response != 0 {
			return fmt.Errorf("logout: target rejected with response %d", logoutResp.Response)
		}
		return nil

	case <-timer.C:
		s.router.Unregister(itt)
		return fmt.Errorf("logout: response timeout after %v", timeout)

	case <-ctx.Done():
		s.router.Unregister(itt)
		return ctx.Err()
	}
}

// Logout performs a graceful session logout. It waits for in-flight
// commands to complete, then exchanges Logout/LogoutResp PDUs with the
// target before shutting down. Per RFC 7143 Section 11.14.
func (s *Session) Logout(ctx context.Context) error {
	// Wait for all in-flight tasks to drain (or timeout).
	drainCtx, drainCancel := context.WithTimeout(ctx, 30*time.Second)
	defer drainCancel()

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		s.mu.Lock()
		pending := len(s.tasks)
		s.mu.Unlock()

		if pending == 0 {
			break
		}

		select {
		case <-drainCtx.Done():
			return fmt.Errorf("logout: timed out waiting for %d in-flight tasks", pending)
		case <-ticker.C:
		}
	}

	// Perform the Logout PDU exchange (needs CmdSN from window).
	if err := s.logout(ctx, 0); err != nil {
		return err
	}

	// Mark session as logged out, close window, and stop goroutines.
	s.mu.Lock()
	s.loggedIn = false
	s.mu.Unlock()
	s.window.close()
	s.cancel()
	return nil
}

// LogoutConnection initiates a logout for connection recovery.
// reasonCode 2 = remove connection for recovery per RFC 7143 Section 11.14.
func (s *Session) LogoutConnection(ctx context.Context, reasonCode uint8) error {
	return s.logout(ctx, reasonCode)
}
