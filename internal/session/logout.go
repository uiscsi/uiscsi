package session

import (
	"context"
	"fmt"
	"time"

	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
)

// Logout performs a clean iSCSI logout by sending a Logout Request PDU
// and waiting for the Logout Response. Per RFC 7143 Section 11.14,
// ReasonCode 0 = close the session.
func (s *Session) Logout(ctx context.Context) error {
	// Acquire CmdSN (logout is non-immediate).
	cmdSN, err := s.window.acquire(ctx)
	if err != nil {
		return fmt.Errorf("session: Logout: acquire CmdSN: %w", err)
	}

	// Register for response.
	itt, pduCh := s.router.Register()

	// Build LogoutReq PDU.
	logoutReq := &pdu.LogoutReq{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: itt,
		},
		ReasonCode: 0, // Close the session
		CmdSN:      cmdSN,
		ExpStatSN:  s.getExpStatSN(),
	}

	bhs, err := logoutReq.MarshalBHS()
	if err != nil {
		s.router.Unregister(itt)
		return fmt.Errorf("session: Logout: encode: %w", err)
	}

	raw := &transport.RawPDU{BHS: bhs}
	select {
	case s.writeCh <- raw:
	case <-ctx.Done():
		s.router.Unregister(itt)
		return ctx.Err()
	}

	// Wait for LogoutResp with timeout.
	timeout := 30 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		timeout = time.Until(dl)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case respRaw := <-pduCh:
		decoded, decErr := pdu.DecodeBHS(respRaw.BHS)
		if decErr != nil {
			return fmt.Errorf("session: Logout: decode response: %w", decErr)
		}
		logoutResp, ok := decoded.(*pdu.LogoutResp)
		if !ok {
			return fmt.Errorf("session: Logout: unexpected PDU type %T", decoded)
		}
		s.window.update(logoutResp.ExpCmdSN, logoutResp.MaxCmdSN)
		s.updateStatSN(logoutResp.StatSN)
		if logoutResp.Response != 0 {
			return fmt.Errorf("session: Logout: target rejected with response %d", logoutResp.Response)
		}
		return nil
	case <-timer.C:
		return fmt.Errorf("session: Logout: timeout waiting for response")
	case <-ctx.Done():
		return ctx.Err()
	}
}
