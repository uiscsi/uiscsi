package session

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"github.com/rkujawa/uiscsi/internal/login"
	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
)

// triggerReconnect starts an ERL 0 reconnect in a background goroutine.
// It is safe to call from multiple goroutines; only the first call starts
// the reconnect, subsequent calls are no-ops while recovery is in progress.
func (s *Session) triggerReconnect(cause error) {
	s.mu.Lock()
	if s.recovering {
		s.mu.Unlock()
		return // Already recovering
	}
	s.recovering = true
	s.mu.Unlock()
	go s.reconnect(cause)
}

// reconnect implements ERL 0 (Error Recovery Level 0) session reinstatement.
// It closes the old connection, snapshots in-flight tasks, re-dials with
// exponential backoff, re-logins with same ISID+TSIH, replaces session
// internals, and retries snapshotted tasks.
func (s *Session) reconnect(cause error) {
	s.cfg.logger.Info("session: connection lost, starting ERL 0 recovery", "cause", cause)

	// Step 1: Stop old pumps by cancelling context and closing old connection.
	// Closing net.Conn unblocks pending Read/Write in pump goroutines.
	// Capture old done channel to wait for dispatchLoop to exit before
	// replacing session fields (prevents data race on s.done, s.unsolCh).
	oldDone := s.done
	s.cancel()
	s.conn.Close()

	// Wait for old dispatchLoop to exit so no goroutine reads replaced fields.
	<-oldDone

	// Step 2: Snapshot in-flight tasks and clear session state.
	s.mu.Lock()
	taskSnapshot := make(map[uint32]*task, len(s.tasks))
	for itt, tk := range s.tasks {
		taskSnapshot[itt] = tk
	}
	s.tasks = make(map[uint32]*task)
	s.mu.Unlock()

	// Unregister all old ITTs from router.
	for itt := range taskSnapshot {
		s.router.Unregister(itt)
	}

	// Step 3: Exponential backoff reconnect loop.
	var newParams *login.NegotiatedParams
	var newConn *transport.Conn
	var lastErr error

	for attempt := range s.cfg.maxReconnectAttempts {
		if attempt > 0 {
			delay := s.cfg.reconnectBackoff * time.Duration(1<<uint(attempt-1))
			time.Sleep(delay)
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

		// Login with same ISID + old TSIH for session reinstatement (RFC 7143 Section 6.3.5).
		loginOpts := make([]login.LoginOption, 0, len(s.cfg.loginOpts)+2)
		loginOpts = append(loginOpts, login.WithISID(s.isid), login.WithTSIH(s.tsih))
		loginOpts = append(loginOpts, s.cfg.loginOpts...)

		params, err := login.Login(ctx, tc, loginOpts...)
		if err != nil {
			cancel()
			tc.Close()
			lastErr = err
			s.cfg.logger.Warn("session: reconnect login failed",
				"attempt", attempt+1, "error", err)
			continue
		}

		cancel()
		newConn = tc
		newParams = params
		break
	}

	if newConn == nil {
		// All attempts exhausted.
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
	newUnsolCh := make(chan *transport.RawPDU, 16)

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

	// Step 5: Start new pump goroutines.
	go s.readPumpLoop(newCtx)
	go s.writePumpLoop(newCtx)
	go s.dispatchLoop(newCtx)
	go s.keepaliveLoop(newCtx)

	// Step 6: Retry snapshotted tasks.
	s.retryTasks(newCtx, taskSnapshot)

	// Step 7: Mark recovery complete.
	s.mu.Lock()
	s.recovering = false
	s.mu.Unlock()

	s.cfg.logger.Info("session: ERL 0 recovery complete")
}

// retryTasks resubmits in-flight tasks that were captured before reconnect.
// Write tasks with non-seekable readers fail with ErrRetryNotPossible.
func (s *Session) retryTasks(ctx context.Context, tasks map[uint32]*task) {
	for _, tk := range tasks {
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
		scsiCmd := &pdu.SCSICommand{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: newITT,
				DataSegmentLen:   uint32(len(immediateData)),
			},
			Read:                       cmd.Read,
			Write:                      cmd.Write,
			Attr:                       cmd.TaskAttributes,
			ExpectedDataTransferLength: cmd.ExpectedDataTransferLen,
			CmdSN:                      cmdSN,
			ExpStatSN:                  s.getExpStatSN(),
			CDB:                        cmd.CDB,
			ImmediateData:              immediateData,
		}
		binary.BigEndian.PutUint64(scsiCmd.Header.LUN[:], cmd.LUN)

		bhs, encErr := scsiCmd.MarshalBHS()
		if encErr != nil {
			tk.cancel(fmt.Errorf("session: retry encode SCSICommand: %w", encErr))
			continue
		}

		raw := &transport.RawPDU{BHS: bhs}
		if len(immediateData) > 0 {
			raw.DataSegment = immediateData
		}

		s.mu.Lock()
		s.tasks[newITT] = tk
		s.mu.Unlock()

		// Send to write pump.
		select {
		case s.writeCh <- raw:
		case <-ctx.Done():
			tk.cancel(ctx.Err())
			continue
		}

		// Send unsolicited Data-Out if InitialR2T=No for writes.
		if tk.isWrite && !s.params.InitialR2T && tk.reader != nil {
			if err := tk.sendUnsolicitedDataOut(s.writeCh, s.getExpStatSN, s.params); err != nil {
				tk.cancel(fmt.Errorf("session: retry unsolicited data: %w", err))
				continue
			}
		}

		go s.taskLoop(tk, pduCh)
	}
}

