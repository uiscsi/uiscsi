package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/uiscsi/uiscsi/internal/login"
	"github.com/uiscsi/uiscsi/internal/transport"
)

// triggerReconnect starts a reconnect in a background goroutine, dispatching
// to replaceConnection (ERL >= 2) or reconnect (ERL 0) based on the
// negotiated ErrorRecoveryLevel. It is safe to call from multiple goroutines;
// only the first call starts recovery, subsequent calls are no-ops.
func (s *Session) triggerReconnect(cause error) {
	// Do not reconnect if the session is being closed.
	select {
	case <-s.closed:
		return
	default:
	}

	s.mu.Lock()
	if s.recovering {
		s.mu.Unlock()
		return // Already recovering
	}
	s.recovering = true
	erl := s.params.ErrorRecoveryLevel
	s.mu.Unlock()
	s.cfg.logger.Info("session: reconnect started",
		"target", s.targetAddr,
		"cause", cause.Error(),
		"erl", erl)
	if erl >= 2 {
		go func() {
			if err := s.replaceConnection(cause); err != nil {
				s.cfg.logger.Error("session: ERL 2 connection replacement failed, falling back to ERL 0", "err", err)
				s.reconnect(cause)
			}
		}()
	} else {
		go s.reconnect(cause)
	}
}

// reconnect implements ERL 0 (Error Recovery Level 0) session reinstatement.
// It closes the old connection, snapshots in-flight tasks, re-dials with
// exponential backoff, re-logins with same ISID+TSIH, replaces session
// internals, and retries snapshotted tasks.
func (s *Session) reconnect(cause error) {
	// Check if session was closed before reconnect even started.
	select {
	case <-s.closed:
		s.mu.Lock()
		s.recovering = false
		s.mu.Unlock()
		return
	default:
	}

	s.cfg.logger.Info("session: connection lost, starting ERL 0 recovery", "cause", cause)

	// Step 1: Stop old pumps by cancelling context and closing old connection.
	// Closing net.Conn unblocks pending Read/Write in pump goroutines.
	// Snapshot old pumpWg and wait for all 4 pump goroutines to exit before
	// replacing session fields (prevents data races on s.conn, s.writeCh,
	// s.unsolCh, s.done). conn.Close() MUST precede oldWg.Wait() so that
	// ReadPump's io.ReadFull unblocks (context cancel alone does NOT unblock it).
	s.mu.Lock()
	oldWg := s.pumpWg
	s.mu.Unlock()
	s.cancel()
	_ = s.conn.Close()

	// Wait for all old pump goroutines to fully exit before replacing session fields.
	if oldWg != nil {
		oldWg.Wait()
	}

	// Check if Close() was called while we were stopping old pumps.
	select {
	case <-s.closed:
		s.mu.Lock()
		s.recovering = false
		s.mu.Unlock()
		return
	default:
	}

	// Step 2: Snapshot in-flight tasks and clear session state.
	s.mu.Lock()
	taskSnapshot := make(map[uint32]*task, len(s.tasks))
	for itt, tk := range s.tasks {
		taskSnapshot[itt] = tk
	}
	s.tasks = make(map[uint32]*task)
	s.mu.Unlock()

	// Unregister and close all old ITT channels. It is safe to close the
	// channels here because oldWg.Wait() above guarantees the read pump
	// goroutine has exited — no more Dispatch calls can occur for these ITTs.
	// Closing the channels unblocks any taskLoop goroutines waiting on pduCh.
	for itt := range taskSnapshot {
		s.router.UnregisterAndClose(itt)
	}

	// Step 3: Exponential backoff reconnect loop.
	var newParams *login.NegotiatedParams
	var newConn *transport.Conn
	var lastErr error

	for attempt := range s.cfg.maxReconnectAttempts {
		if attempt > 0 {
			delay := s.cfg.reconnectBackoff * time.Duration(1<<uint(max(0, attempt-1)))
			// Use a timer-based select so the reconnect goroutine can be
			// interrupted by Close() without blocking for the full backoff period.
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-s.closed:
				timer.Stop()
				s.mu.Lock()
				s.recovering = false
				s.mu.Unlock()
				return // session closed, abort reconnect
			}
		}

		// Check again after sleep in case Close() was called.
		select {
		case <-s.closed:
			s.mu.Lock()
			s.recovering = false
			s.mu.Unlock()
			return
		default:
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

		// Dial new TCP connection.
		tc, err := transport.Dial(ctx, s.targetAddr)
		if err != nil {
			cancel()
			lastErr = err
			s.cfg.logger.Warn("session: reconnect dial failed",
				"attempt", attempt+1, "error", err)
			continue
		}

		// Login with same ISID. First try old TSIH for session
		// reinstatement (RFC 7143 Section 6.3.5). If that fails with
		// "session does not exist" (class=2 detail=10), retry with
		// TSIH=0 for a fresh session on the same attempt.
		loginOpts := make([]login.LoginOption, 0, len(s.cfg.loginOpts)+3)
		loginOpts = append(loginOpts, login.WithISID(s.isid), login.WithTSIH(s.tsih))
		loginOpts = append(loginOpts, login.WithLoginLogger(s.cfg.logger))
		loginOpts = append(loginOpts, s.cfg.loginOpts...)

		params, err := login.Login(ctx, tc, loginOpts...)
		if err != nil {
			// If session reinstatement fails, try fresh session (TSIH=0).
			var le *login.LoginError
			if errors.As(err, &le) && le.StatusClass == 2 {
				cancel()
				_ = tc.Close()

				// Re-dial for fresh session login.
				ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
				tc2, dialErr := transport.Dial(ctx2, s.targetAddr)
				if dialErr != nil {
					cancel2()
					lastErr = dialErr
					s.cfg.logger.Warn("session: reconnect re-dial failed",
						"attempt", attempt+1, "error", dialErr)
					continue
				}

				freshOpts := make([]login.LoginOption, 0, len(s.cfg.loginOpts)+2)
				freshOpts = append(freshOpts, login.WithISID(s.isid))
				freshOpts = append(freshOpts, login.WithLoginLogger(s.cfg.logger))
				freshOpts = append(freshOpts, s.cfg.loginOpts...)

				params, err = login.Login(ctx2, tc2, freshOpts...)
				cancel2()
				if err != nil {
					_ = tc2.Close()
					lastErr = err
					s.cfg.logger.Warn("session: reconnect fresh login failed",
						"attempt", attempt+1, "error", err)
					continue
				}
				tc = tc2
			} else {
				cancel()
				_ = tc.Close()
				lastErr = err
				s.cfg.logger.Warn("session: reconnect login failed",
					"attempt", attempt+1, "error", err)
				continue
			}
		} else {
			cancel()
		}

		newConn = tc
		newParams = params
		break
	}

	if newConn == nil {
		// All attempts exhausted (or maxReconnectAttempts == 0).
		if lastErr == nil {
			lastErr = cause
		}
		s.cfg.logger.Warn("session: reconnect failed",
			"attempts", s.cfg.maxReconnectAttempts,
			"error", lastErr.Error())
		s.mu.Lock()
		s.err = fmt.Errorf("session: reconnect failed after %d attempts: %w",
			s.cfg.maxReconnectAttempts, lastErr)
		s.recovering = false
		s.mu.Unlock()

		// Fail all snapshotted tasks.
		for _, tk := range taskSnapshot {
			tk.cancel(fmt.Errorf("session: reconnect failed: %w", lastErr))
		}
		return
	}

	// Step 4: Replace session internals with new connection.
	newCtx, newCancel := context.WithCancel(context.Background())
	newWriteCh := make(chan *transport.RawPDU, 64)
	newUnsolCh := make(chan *transport.RawPDU, 64)

	s.mu.Lock()
	s.conn = newConn
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

	// Step 5: Start new pump goroutines with fresh per-invocation WaitGroup.
	s.startPumps(newCtx)

	// Step 6: Retry snapshotted tasks.
	s.retryTasks(newCtx, taskSnapshot)

	// Step 7: Mark recovery complete.
	s.mu.Lock()
	s.recovering = false
	s.mu.Unlock()

	s.cfg.logger.Info("session: reconnect complete",
		"new_tsih", newParams.TSIH)
}

// retryTasks resubmits in-flight tasks that were captured before reconnect.
// Write tasks with non-seekable readers fail with ErrRetryNotPossible.
func (s *Session) retryTasks(ctx context.Context, tasks map[uint32]*task) {
	for _, tk := range tasks {
		// Streaming tasks cannot be retried: the caller already holds the
		// chanReader from the original submission. The reader cannot be
		// replaced after reconnection. This is correct for sequential
		// devices (tape) — you cannot resume a tape read mid-stream.
		if tk.streaming {
			tk.cancel(fmt.Errorf("session: streaming task not retriable after reconnect"))
			continue
		}

		// For write tasks, check if Reader is seekable.
		if tk.isWrite && tk.reader != nil {
			if seeker, ok := tk.reader.(io.Seeker); ok {
				if _, err := seeker.Seek(0, io.SeekStart); err != nil {
					tk.cancel(fmt.Errorf("session: retry seek failed: %w", err))
					continue
				}
				tk.bytesSent = 0
			} else {
				tk.cancel(ErrRetryNotPossible)
				continue
			}
		}

		// Acquire new CmdSN.
		cmdSN, err := s.window.acquire(ctx)
		if err != nil {
			tk.cancel(fmt.Errorf("session: retry acquire CmdSN: %w", err))
			continue
		}

		// Allocate new ITT and register with router.
		newITT := s.router.AllocateITT()
		pduCh := s.router.RegisterPersistent(newITT)

		// Reset task state for fresh attempt.
		tk.itt = newITT
		tk.nextDataSN = 0
		tk.nextOffset = 0
		if tk.isRead && tk.buf != nil {
			tk.buf.Reset()
		}

		// Re-read immediate data for write commands.
		var immediateData []byte
		if tk.isWrite && s.params.ImmediateData && tk.reader != nil {
			immLen := min(s.params.FirstBurstLength, s.params.MaxRecvDataSegmentLength)
			immBuf := make([]byte, immLen)
			n, readErr := io.ReadFull(tk.reader, immBuf)
			if readErr != nil && readErr != io.ErrUnexpectedEOF {
				tk.cancel(fmt.Errorf("session: retry read immediate data: %w", readErr))
				continue
			}
			immediateData = immBuf[:n]
			tk.bytesSent = uint32(n)
		}

		// Build fresh SCSICommand PDU with new sequence numbers.
		cmd := tk.cmd
		raw, encErr := buildSCSICommandPDU(cmd, newITT, cmdSN, s.getExpStatSN(), immediateData)
		if encErr != nil {
			tk.cancel(fmt.Errorf("session: retry encode SCSICommand: %w", encErr))
			continue
		}

		s.mu.Lock()
		s.tasks[newITT] = tk
		s.mu.Unlock()

		s.stampDigests(raw)

		// Send to write pump.
		select {
		case s.writeCh <- raw:
		case <-ctx.Done():
			tk.cancel(ctx.Err())
			continue
		}

		// Send unsolicited Data-Out if InitialR2T=No for writes.
		if tk.isWrite && !s.params.InitialR2T && tk.reader != nil {
			if err := tk.sendUnsolicitedDataOut(s.writeCh, s.getExpStatSN, s.params, s.stampDigests); err != nil {
				tk.cancel(fmt.Errorf("session: retry unsolicited data: %w", err))
				continue
			}
		}

		go s.taskLoop(tk, pduCh)
	}
}

