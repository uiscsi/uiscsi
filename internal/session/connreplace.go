package session

import (
	"context"
	"fmt"
	"time"

	"github.com/uiscsi/uiscsi/internal/login"
	"github.com/uiscsi/uiscsi/internal/transport"
)

// replaceConnection implements ERL 2 connection replacement per RFC 7143
// Section 7.4. It replaces a failed connection within the same session,
// then reassigns outstanding tasks to the new connection.
//
// This is single-connection replacement (MaxConnections=1 per D-08).
// Full MC/S support is deferred to v2.
func (s *Session) replaceConnection(cause error) error {
	s.cfg.logger.Info("session: starting ERL 2 connection replacement", "cause", cause)

	// Step 1: Stop old pumps. Cancel context first, then close connection
	// to unblock any blocked reads/writes, then wait for goroutines to exit.
	s.cancel()
	_ = s.conn.Close()

	// Wait for dispatchLoop to exit (it closes s.done on return).
	// This ensures all old goroutines are done accessing s.writeCh/s.unsolCh
	// before we replace them.
	select {
	case <-s.done:
	case <-time.After(5 * time.Second):
		// Timeout safety -- should not happen but prevents deadlock.
	}

	// Step 2: Snapshot in-flight tasks (keep them alive, unlike ERL 0 which
	// clears and retries -- ERL 2 reassigns).
	s.mu.Lock()
	taskSnapshot := make(map[uint32]*task, len(s.tasks))
	for itt, tk := range s.tasks {
		taskSnapshot[itt] = tk
	}
	s.mu.Unlock()

	// Step 3: Dial new TCP connection.
	ctx, ctxCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer ctxCancel()

	newTc, err := transport.Dial(ctx, s.targetAddr)
	if err != nil {
		return fmt.Errorf("session: ERL 2 dial failed: %w", err)
	}

	// Step 4: Login with same ISID + TSIH for session reinstatement (per D-08).
	// The initiator uses WithISID to maintain the same session identity and
	// WithTSIH with the non-zero TSIH assigned by the target during the
	// original login. This tells the target this is a session reinstatement,
	// not a new session.
	loginOpts := append([]login.LoginOption{
		login.WithISID(s.isid),
		login.WithTSIH(s.tsih),
	}, s.loginOpts...)

	newParams, err := login.Login(ctx, newTc, loginOpts...)
	if err != nil {
		_ = newTc.Close()
		return fmt.Errorf("session: ERL 2 login failed: %w", err)
	}

	// Step 5: Replace session internals.
	newCtx, newCancel := context.WithCancel(context.Background())
	newWriteCh := make(chan *transport.RawPDU, 64)
	newUnsolCh := make(chan *transport.RawPDU, 16)

	s.mu.Lock()
	s.conn = newTc
	s.params = *newParams
	s.tsih = newParams.TSIH
	s.writeCh = newWriteCh
	s.unsolCh = newUnsolCh
	s.cancel = newCancel
	s.window = newCmdWindow(newParams.CmdSN, newParams.CmdSN, newParams.CmdSN)
	s.expStatSN = newParams.ExpStatSN
	s.err = nil
	s.loggedIn = true
	s.done = make(chan struct{})
	s.mu.Unlock()

	// Start new pump goroutines.
	s.startPumps(newCtx)

	// Step 6: Issue Logout with reasonCode=2 on the NEW connection to signal
	// connection recovery for the old (failed) connection (per D-08).
	// Per RFC 7143 Section 7.4, the initiator uses the new connection to
	// issue a Logout for the old CID with reason "remove the connection for
	// recovery purposes" (reasonCode=2). Since we are in a single-connection
	// model, the old CID is implicitly the one that was just replaced. The
	// target uses this to clean up old connection state.
	//
	// NOTE: The old connection is dead, so we cannot send Logout on it.
	// This Logout goes over the NEW connection referencing the old CID.
	logoutCtx, logoutCancel := context.WithTimeout(newCtx, 5*time.Second)
	if logoutErr := s.logout(logoutCtx, 2); logoutErr != nil {
		// Logout failure is not fatal for ERL 2 -- the target may have
		// already cleaned up the old connection. Log and continue.
		s.cfg.logger.Warn("session: ERL 2 Logout(reasonCode=2) failed, continuing",
			"error", logoutErr)
	}
	logoutCancel()

	// Step 7: Reassign tasks via TMF TASK REASSIGN (Function=14).
	// Per RFC 7143 Section 7.2.2, task reassignment uses TMF with
	// Function=TASK REASSIGN and ReferencedTaskTag=task's ITT.
	for itt, tk := range taskSnapshot {
		// Allocate new ITT for reassigned task BEFORE unregistering old one.
		newITT := s.router.AllocateITT()
		newPduCh := s.router.RegisterPersistent(newITT)

		// Send TASK REASSIGN TMF referencing the OLD ITT.
		// The old ITT remains registered so any in-flight target responses
		// are still routed correctly during the reassignment window.
		tmfResult, tmfErr := s.sendTMF(newCtx, TMFTaskReassign, itt, tk.lun)
		if tmfErr != nil || (tmfResult != nil && tmfResult.Response != TMFRespComplete) {
			// Reassign failed -- fail this task.
			respStr := "send error"
			if tmfResult != nil {
				respStr = fmt.Sprintf("response=%d", tmfResult.Response)
			}
			// Clean up the new ITT we allocated but won't use.
			s.router.Unregister(newITT)
			// NOW unregister the old ITT since we're giving up.
			s.router.Unregister(itt)
			tk.cancel(fmt.Errorf("session: task reassign failed: %s: %w", respStr, tmfErr))
			continue
		}

		// TMF TASK REASSIGN confirmed -- NOW safe to unregister old ITT.
		s.router.Unregister(itt)

		// Update task with new ITT.
		s.mu.Lock()
		tk.itt = newITT
		s.tasks[newITT] = tk
		delete(s.tasks, itt) // remove old ITT entry
		s.mu.Unlock()

		// Restart task loop on new channel.
		go s.taskLoop(tk, newPduCh)
	}

	s.cfg.logger.Info("session: ERL 2 connection replacement complete",
		"tasks_reassigned", len(taskSnapshot))
	return nil
}
