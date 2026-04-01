package session

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/rkujawa/uiscsi/internal/digest"
	"github.com/rkujawa/uiscsi/internal/login"
	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/serial"
	"github.com/rkujawa/uiscsi/internal/transport"
)

// ErrRetryNotPossible indicates a write command cannot be retried after
// connection recovery because the io.Reader does not implement io.Seeker.
var ErrRetryNotPossible = errors.New("session: retry not possible (non-seekable write data)")

// ErrSessionRecovering indicates the session is currently performing
// ERL 0 reconnection and new commands cannot be submitted.
var ErrSessionRecovering = errors.New("session: recovery in progress")

// Session represents an iSCSI full-feature-phase session. It wraps a
// transport.Conn after login, provides CmdSN/MaxCmdSN flow control,
// dispatches SCSI commands with ITT correlation, and reassembles
// multi-PDU Data-In responses.
type Session struct {
	conn   *transport.Conn
	params login.NegotiatedParams
	router *transport.Router

	writeCh chan *transport.RawPDU
	unsolCh chan *transport.RawPDU

	window *cmdWindow

	mu         sync.Mutex
	expStatSN  uint32
	tasks      map[uint32]*task
	loggedIn   bool // true while session is in full-feature phase
	recovering bool // true during ERL 0 reconnect, blocks Submit

	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
	err       error

	cfg sessionConfig

	// Reconnect context for ERL 0/2 recovery.
	targetAddr string
	loginOpts  []login.LoginOption
	isid       [6]byte
	tsih       uint16
}

// NewSession creates a Session from a post-login transport connection
// and the negotiated parameters. It starts background goroutines for
// reading, writing, and dispatch. The caller must call Close when done.
func NewSession(conn *transport.Conn, params login.NegotiatedParams, opts ...SessionOption) *Session {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &Session{
		conn:       conn,
		params:     params,
		router:     transport.NewRouter(),
		writeCh:    make(chan *transport.RawPDU, 64),
		unsolCh:    make(chan *transport.RawPDU, 16),
		window:     newCmdWindow(params.CmdSN, params.CmdSN, params.CmdSN),
		expStatSN:  params.ExpStatSN,
		tasks:      make(map[uint32]*task),
		loggedIn:   true,
		cancel:     cancel,
		done:       make(chan struct{}),
		cfg:        cfg,
		targetAddr: cfg.targetAddr,
		loginOpts:  cfg.loginOpts,
		isid:       params.ISID,
		tsih:       params.TSIH,
	}

	s.cfg.logger.Info("session: opened",
		"isid", fmt.Sprintf("%x", params.ISID),
		"tsih", params.TSIH,
		"max_cmd_sn", params.CmdSN)

	// Start background goroutines. Capture conn/channels locally so
	// replaceConnection can safely replace s.conn/s.writeCh/s.unsolCh
	// without racing with goroutines that use the old values.
	s.startPumps(ctx)

	return s
}

// Params returns a copy of the negotiated parameters. The returned value
// is a snapshot; modifying it has no effect on the session.
func (s *Session) Params() login.NegotiatedParams {
	return s.params
}

// Submit sends a SCSI command via the session and returns a channel that
// will receive exactly one Result when the command completes. Submit
// blocks if the CmdSN window is full, respecting the context deadline.
func (s *Session) Submit(ctx context.Context, cmd Command) (<-chan Result, error) {
	// Reject new commands during ERL 0 recovery.
	s.mu.Lock()
	if s.recovering {
		s.mu.Unlock()
		return nil, ErrSessionRecovering
	}
	s.mu.Unlock()

	// Acquire a CmdSN slot (blocks if window is full).
	cmdSN, err := s.window.acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("session: acquire CmdSN (window full): %w", err)
	}

	// Allocate ITT and register persistent channel.
	itt := s.router.AllocateITT()
	pduCh := s.router.RegisterPersistent(itt)

	// Auto-detect write commands from Data io.Reader.
	isWrite := cmd.Data != nil
	if isWrite {
		cmd.Write = true // Auto-set W-bit per RFC 7143
	}

	// Create task for tracking this command.
	tk := newTask(itt, cmd.Read, isWrite)
	tk.lun = cmd.LUN // Store LUN for TMF LUN-based cleanup
	tk.cmd = cmd      // Store for retry during ERL 0 recovery

	s.mu.Lock()
	s.tasks[itt] = tk
	expStatSN := s.expStatSN
	s.mu.Unlock()

	// Read immediate data from io.Reader when ImmediateData=Yes.
	var immediateData []byte
	if isWrite && s.params.ImmediateData {
		immLen := min(s.params.FirstBurstLength, s.params.MaxRecvDataSegmentLength)
		immBuf := make([]byte, immLen)
		n, readErr := io.ReadFull(cmd.Data, immBuf)
		if readErr != nil && readErr != io.ErrUnexpectedEOF {
			// Reader failed before we could read immediate data.
			s.router.Unregister(itt)
			s.mu.Lock()
			delete(s.tasks, itt)
			s.mu.Unlock()
			return nil, fmt.Errorf("session: read immediate data: %w", readErr)
		}
		immediateData = immBuf[:n]
		tk.bytesSent = uint32(n) // Track for unsolicited/R2T offset
	}

	// Build SCSICommand PDU.
	scsiCmd := &pdu.SCSICommand{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: itt,
			DataSegmentLen:   uint32(len(immediateData)),
		},
		Read:                       cmd.Read,
		Write:                      cmd.Write,
		Attr:                       cmd.TaskAttributes,
		ExpectedDataTransferLength: cmd.ExpectedDataTransferLen,
		CmdSN:                      cmdSN,
		ExpStatSN:                  expStatSN,
		CDB:                        cmd.CDB,
		ImmediateData:              immediateData,
	}

	// Set LUN in header.
	binary.BigEndian.PutUint64(scsiCmd.Header.LUN[:], cmd.LUN)

	// Encode to wire format.
	bhs, encErr := scsiCmd.MarshalBHS()
	if encErr != nil {
		s.router.Unregister(itt)
		s.mu.Lock()
		delete(s.tasks, itt)
		s.mu.Unlock()
		return nil, fmt.Errorf("session: encode SCSICommand (itt=0x%08x cmd_sn=%d): %w", itt, cmdSN, encErr)
	}

	raw := &transport.RawPDU{BHS: bhs}
	if len(immediateData) > 0 {
		raw.DataSegment = immediateData
	}

	// Send to write pump.
	select {
	case s.writeCh <- raw:
	case <-ctx.Done():
		s.router.Unregister(itt)
		s.mu.Lock()
		delete(s.tasks, itt)
		s.mu.Unlock()
		return nil, ctx.Err()
	}

	// Transfer io.Reader ownership to task for R2T handling.
	tk.reader = cmd.Data

	// Send unsolicited Data-Out when InitialR2T=No (per D-04, WRITE-04).
	// This runs synchronously in Submit so the io.Reader is not yet shared
	// with the task goroutine (Pitfall 6: no concurrent reads).
	if isWrite && !s.params.InitialR2T {
		if err := tk.sendUnsolicitedDataOut(s.writeCh, s.getExpStatSN, s.params); err != nil {
			s.router.Unregister(itt)
			s.mu.Lock()
			delete(s.tasks, itt)
			s.mu.Unlock()
			return nil, fmt.Errorf("session: unsolicited data: %w", err)
		}
	}

	// Start per-task goroutine to drain PDU channel.
	go s.taskLoop(tk, pduCh)

	return tk.resultCh, nil
}

// Close shuts down the session. If the session is still logged in, it
// attempts a graceful Logout PDU exchange with a short timeout before
// force-closing. Close is idempotent via sync.Once.
func (s *Session) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		// Attempt graceful logout if still logged in.
		s.mu.Lock()
		wasLoggedIn := s.loggedIn
		s.loggedIn = false
		hasErr := s.err != nil
		s.mu.Unlock()

		s.cfg.logger.Info("session: closing",
			"was_logged_in", wasLoggedIn)

		if wasLoggedIn && !hasErr {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = s.logout(ctx, 0)
			cancel()
		}

		s.window.close()
		s.cancel()

		// Cancel all in-flight tasks and capture conn under lock
		// (conn may be replaced by reconnect goroutine).
		s.mu.Lock()
		for itt, tk := range s.tasks {
			tk.cancel(errors.New("session: closed"))
			s.router.Unregister(itt)
			delete(s.tasks, itt)
		}
		conn := s.conn
		s.mu.Unlock()

		closeErr = conn.Close()
	})
	return closeErr
}

// Err returns the session-fatal error, if any.
func (s *Session) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// updateStatSN updates ExpStatSN from a target response PDU.
// Per RFC 7143 Section 3.2.2.3, ExpStatSN = StatSN + 1.
func (s *Session) updateStatSN(statSN uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := serial.Incr(statSN)
	if serial.GreaterThan(next, s.expStatSN) || next == s.expStatSN {
		s.expStatSN = next
	}
}

// getExpStatSN returns the current ExpStatSN.
func (s *Session) getExpStatSN() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.expStatSN
}

// startPumps starts the background goroutines for the session. It captures
// current conn/channel values locally so that replaceConnection can safely
// replace them on the Session struct without racing with old goroutines.
func (s *Session) startPumps(ctx context.Context) {
	conn := s.conn
	writeCh := s.writeCh
	unsolCh := s.unsolCh
	done := s.done

	go s.readPumpLoop(ctx, conn, unsolCh)
	go s.writePumpLoop(ctx, conn, writeCh)
	go s.dispatchLoop(ctx, unsolCh, done)
	go s.keepaliveLoop(ctx)
}

// pduHookBridge returns a transport-compatible PDU hook function that bridges
// to the session-layer typed PDUDirection hook and metrics hook. Returns nil
// if neither hook is configured, allowing the pump to skip the call entirely.
func (s *Session) pduHookBridge() func(uint8, *transport.RawPDU) {
	if s.cfg.pduHook == nil && s.cfg.metricsHook == nil {
		return nil
	}
	return func(dir uint8, raw *transport.RawPDU) {
		if s.cfg.pduHook != nil {
			var d PDUDirection
			if dir == transport.HookReceive {
				d = PDUReceive
			}
			s.cfg.pduHook(d, raw)
		}
		if s.cfg.metricsHook != nil {
			opcode := pdu.OpCode(raw.BHS[0] & 0x3f)
			dsLen := uint32(raw.BHS[5])<<16 | uint32(raw.BHS[6])<<8 | uint32(raw.BHS[7])
			if dir == transport.HookSend {
				s.cfg.metricsHook(MetricEvent{Type: MetricPDUSent, OpCode: opcode})
				if dsLen > 0 {
					s.cfg.metricsHook(MetricEvent{Type: MetricBytesOut, Bytes: uint64(dsLen)})
				}
			} else {
				s.cfg.metricsHook(MetricEvent{Type: MetricPDUReceived, OpCode: opcode})
				if dsLen > 0 {
					s.cfg.metricsHook(MetricEvent{Type: MetricBytesIn, Bytes: uint64(dsLen)})
				}
			}
		}
	}
}

// readPumpLoop runs the transport read pump.
func (s *Session) readPumpLoop(ctx context.Context, conn *transport.Conn, unsolCh chan *transport.RawPDU) {
	err := transport.ReadPump(ctx, conn.NetConn(), s.router, unsolCh,
		conn.DigestHeader(), conn.DigestData(),
		s.cfg.logger, s.pduHookBridge())
	if err != nil && ctx.Err() == nil {
		// DigestError is connection-fatal at ERL 0 (per RFC 7143 Section 7.3
		// and D-03). Do NOT attempt reconnect -- the connection data is corrupt.
		var de *digest.DigestError
		if errors.As(err, &de) {
			s.cfg.logger.Warn("session: digest mismatch, dropping connection (per D-03)",
				"digest_type", de.Type.String(),
				"expected", fmt.Sprintf("0x%08X", de.Expected),
				"actual", fmt.Sprintf("0x%08X", de.Actual))
			s.mu.Lock()
			if s.err == nil {
				s.err = fmt.Errorf("session: digest mismatch (connection fatal): %w", err)
			}
			s.mu.Unlock()
			return
		}

		// Non-digest errors: attempt ERL 0 recovery if configured.
		if s.targetAddr != "" {
			s.triggerReconnect(fmt.Errorf("session: read pump: %w", err))
		} else {
			s.mu.Lock()
			if s.err == nil {
				s.err = fmt.Errorf("session: read pump: %w", err)
			}
			s.mu.Unlock()
		}
	}
}

// writePumpLoop runs the transport write pump.
func (s *Session) writePumpLoop(ctx context.Context, conn *transport.Conn, writeCh chan *transport.RawPDU) {
	err := transport.WritePump(ctx, conn.NetConn(), writeCh,
		s.cfg.logger, s.pduHookBridge())
	if err != nil && ctx.Err() == nil {
		s.cfg.logger.Error("session: fatal error",
			"source", "write_pump",
			"error", err.Error())
		s.mu.Lock()
		if s.err == nil {
			s.err = fmt.Errorf("session: write pump: %w", err)
		}
		s.mu.Unlock()
	}
}

// dispatchLoop handles unsolicited PDUs (ITT=0xFFFFFFFF) from the target.
func (s *Session) dispatchLoop(ctx context.Context, unsolCh chan *transport.RawPDU, done chan struct{}) {
	defer close(done)
	for {
		select {
		case <-ctx.Done():
			return
		case raw, ok := <-unsolCh:
			if !ok {
				return
			}
			s.handleUnsolicited(raw)
		}
	}
}

// handleUnsolicited processes unsolicited target PDUs (ITT=0xFFFFFFFF).
// It dispatches to dedicated handlers based on opcode.
func (s *Session) handleUnsolicited(raw *transport.RawPDU) {
	opcode := raw.BHS[0] & 0x3f

	switch pdu.OpCode(opcode) {
	case pdu.OpNOPIn:
		s.handleUnsolicitedNOPIn(raw)
	case pdu.OpAsyncMsg:
		s.handleAsyncMsg(raw)
	default:
		s.cfg.logger.Warn("session: unhandled unsolicited PDU",
			"opcode", fmt.Sprintf("0x%02X", opcode))
	}
}

// taskLoop drains PDUs from the router channel for a single task.
// It runs in its own goroutine so one slow reader doesn't block others.
func (s *Session) taskLoop(tk *task, pduCh <-chan *transport.RawPDU) {
	for raw := range pduCh {
		decoded, err := pdu.DecodeBHS(raw.BHS)
		if err != nil {
			tk.cancel(fmt.Errorf("session: decode response PDU (itt=0x%08x): %w", tk.itt, err))
			s.cleanupTask(tk.itt)
			return
		}

		switch p := decoded.(type) {
		case *pdu.DataIn:
			p.Data = raw.DataSegment
			oldMax := s.window.maxCmdSNValue()
			s.window.update(p.ExpCmdSN, p.MaxCmdSN)
			if s.window.maxCmdSNValue() != oldMax {
				s.cfg.logger.Info("session: command window updated",
					"exp_cmd_sn", p.ExpCmdSN,
					"max_cmd_sn", p.MaxCmdSN)
			}
			if p.HasStatus {
				s.updateStatSN(p.StatSN)
			}
			tk.handleDataIn(p)
			if p.HasStatus {
				if s.cfg.metricsHook != nil {
					s.cfg.metricsHook(MetricEvent{
						Type:    MetricCommandComplete,
						Latency: time.Since(tk.startTime),
					})
				}
				s.cleanupTask(tk.itt)
				return
			}

		case *pdu.R2T:
			oldMax := s.window.maxCmdSNValue()
			s.window.update(p.ExpCmdSN, p.MaxCmdSN)
			if s.window.maxCmdSNValue() != oldMax {
				s.cfg.logger.Info("session: command window updated",
					"exp_cmd_sn", p.ExpCmdSN,
					"max_cmd_sn", p.MaxCmdSN)
			}
			s.updateStatSN(p.StatSN)
			if err := tk.handleR2T(p, s.writeCh, s.getExpStatSN, s.params); err != nil {
				tk.cancel(fmt.Errorf("session: write data (itt=0x%08x): %w", tk.itt, err))
				s.cleanupTask(tk.itt)
				return
			}

		case *pdu.SCSIResponse:
			p.Data = raw.DataSegment
			oldMax := s.window.maxCmdSNValue()
			s.window.update(p.ExpCmdSN, p.MaxCmdSN)
			if s.window.maxCmdSNValue() != oldMax {
				s.cfg.logger.Info("session: command window updated",
					"exp_cmd_sn", p.ExpCmdSN,
					"max_cmd_sn", p.MaxCmdSN)
			}
			s.updateStatSN(p.StatSN)
			tk.handleSCSIResponse(p)
			if s.cfg.metricsHook != nil {
				s.cfg.metricsHook(MetricEvent{
					Type:    MetricCommandComplete,
					Latency: time.Since(tk.startTime),
				})
			}
			s.cleanupTask(tk.itt)
			return

		default:
			s.cfg.logger.Warn("session: unexpected PDU for task",
				"itt", fmt.Sprintf("0x%08x", tk.itt),
				"opcode", fmt.Sprintf("0x%02X", raw.BHS[0]&0x3f))
		}
	}
}

// cleanupTask removes a completed task from tracking.
func (s *Session) cleanupTask(itt uint32) {
	s.router.Unregister(itt)
	s.mu.Lock()
	delete(s.tasks, itt)
	s.mu.Unlock()
}
