package session

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/uiscsi/uiscsi/internal/digest"
	"github.com/uiscsi/uiscsi/internal/login"
	"github.com/uiscsi/uiscsi/internal/pdu"
	"github.com/uiscsi/uiscsi/internal/serial"
	"github.com/uiscsi/uiscsi/internal/transport"
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
	ctx    context.Context
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
	pumpWg    *sync.WaitGroup // tracks all 4 pump goroutines for deterministic Close
	closeOnce sync.Once
	closed    chan struct{} // closed by Close() to signal reconnect goroutine to abort
	err       error

	// dropCounter counts optional async PDUs dropped by ReadPump when
	// unsolicitedCh is full. Exposed for observability and test assertions.
	dropCounter atomic.Uint64

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
		ctx:        ctx,
		conn:       conn,
		params:     params,
		router:     transport.NewRouter(cfg.routerBufDepth),
		writeCh:    make(chan *transport.RawPDU, 64),
		unsolCh:    make(chan *transport.RawPDU, 64),
		window:     newCmdWindow(params.CmdSN, params.CmdSN, params.CmdSN),
		expStatSN:  params.ExpStatSN,
		tasks:      make(map[uint32]*task),
		loggedIn:   true,
		cancel:     cancel,
		done:       make(chan struct{}),
		closed:     make(chan struct{}),
		cfg:        cfg,
		targetAddr: cfg.targetAddr,
		loginOpts:  cfg.loginOpts,
		isid:       params.ISID,
		tsih:       params.TSIH,
	}

	// Apply digest byte order if configured.
	if cfg.digestByteOrder != nil {
		conn.SetDigestByteOrder(cfg.digestByteOrder)
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
	tk := newTask(itt, cmd.Read, isWrite, 0) // non-streaming
	tk.lun = cmd.LUN  // Store LUN for TMF LUN-based cleanup
	tk.cmd = cmd       // Store for retry during ERL 0 recovery
	tk.cmdSN = cmdSN   // Store for same-connection retry at ERL >= 1

	// Populate ERL fields for SNACK handling (DataSN gap detection, A-bit DataACK).
	tk.erl = uint32(s.params.ErrorRecoveryLevel)
	tk.getWriteCh = s.getWriteCh
	tk.expStatSNFunc = s.getExpStatSN
	tk.snackTimeout = s.cfg.snackTimeout

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
	raw, encErr := buildSCSICommandPDU(cmd, itt, cmdSN, expStatSN, immediateData)
	if encErr != nil {
		s.router.Unregister(itt)
		s.mu.Lock()
		delete(s.tasks, itt)
		s.mu.Unlock()
		return nil, fmt.Errorf("session: encode SCSICommand (itt=0x%08x cmd_sn=%d): %w", itt, cmdSN, encErr)
	}

	// Compute digests before sending.
	s.stampDigests(raw)

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
		if err := tk.sendUnsolicitedDataOut(s.writeCh, s.getExpStatSN, s.params, s.stampDigests); err != nil {
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

// SubmitStreaming is like Submit but creates a streaming task for read
// commands. Data flows through a bounded-memory chanReader as Data-In
// PDUs arrive, keeping memory usage constant regardless of transfer size.
//
// Returns both the result channel (for final status/sense) and an
// io.Reader (for streaming data). The caller must read from the
// io.Reader concurrently with (or before) receiving from the result
// channel. The result channel delivers the final status when the command
// completes; Result.Data is nil for streaming submissions since data is
// delivered via the returned io.Reader.
//
// Streaming tasks are not retriable after ERL 0 reconnection because the
// caller already holds the io.Reader from the original submission.
func (s *Session) SubmitStreaming(ctx context.Context, cmd Command) (<-chan Result, io.Reader, error) {
	s.mu.Lock()
	if s.recovering {
		s.mu.Unlock()
		return nil, nil, ErrSessionRecovering
	}
	s.mu.Unlock()

	cmdSN, err := s.window.acquire(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("session: acquire CmdSN (window full): %w", err)
	}

	itt := s.router.AllocateITT()
	pduCh := s.router.RegisterPersistent(itt)

	isWrite := cmd.Data != nil
	if isWrite {
		cmd.Write = true
	}

	// Streaming with configured depth. If config is 0, use default.
	depth := s.cfg.streamBufDepth
	if depth <= 0 {
		depth = defaultChanBufSize
	}
	tk := newTask(itt, cmd.Read, isWrite, depth)
	tk.lun = cmd.LUN
	tk.cmd = cmd
	tk.cmdSN = cmdSN

	tk.erl = uint32(s.params.ErrorRecoveryLevel)
	tk.getWriteCh = s.getWriteCh
	tk.expStatSNFunc = s.getExpStatSN
	tk.snackTimeout = s.cfg.snackTimeout

	s.mu.Lock()
	s.tasks[itt] = tk
	expStatSN := s.expStatSN
	s.mu.Unlock()

	var immediateData []byte
	if isWrite && s.params.ImmediateData {
		immLen := min(s.params.FirstBurstLength, s.params.MaxRecvDataSegmentLength)
		immBuf := make([]byte, immLen)
		n, readErr := io.ReadFull(cmd.Data, immBuf)
		if readErr != nil && readErr != io.ErrUnexpectedEOF {
			s.router.Unregister(itt)
			s.mu.Lock()
			delete(s.tasks, itt)
			s.mu.Unlock()
			return nil, nil, fmt.Errorf("session: read immediate data: %w", readErr)
		}
		immediateData = immBuf[:n]
		tk.bytesSent = uint32(n)
	}

	raw, encErr := buildSCSICommandPDU(cmd, itt, cmdSN, expStatSN, immediateData)
	if encErr != nil {
		s.router.Unregister(itt)
		s.mu.Lock()
		delete(s.tasks, itt)
		s.mu.Unlock()
		return nil, nil, fmt.Errorf("session: encode SCSICommand (itt=0x%08x cmd_sn=%d): %w", itt, cmdSN, encErr)
	}
	s.stampDigests(raw)

	select {
	case s.writeCh <- raw:
	case <-ctx.Done():
		s.router.Unregister(itt)
		s.mu.Lock()
		delete(s.tasks, itt)
		s.mu.Unlock()
		return nil, nil, ctx.Err()
	}

	tk.reader = cmd.Data

	if isWrite && !s.params.InitialR2T {
		if err := tk.sendUnsolicitedDataOut(s.writeCh, s.getExpStatSN, s.params, s.stampDigests); err != nil {
			s.router.Unregister(itt)
			s.mu.Lock()
			delete(s.tasks, itt)
			s.mu.Unlock()
			return nil, nil, fmt.Errorf("session: unsolicited data: %w", err)
		}
	}

	go s.taskLoop(tk, pduCh)

	// Return chanReader as io.Reader. May be nil if this is not a read command.
	var dataReader io.Reader
	if tk.dataReader != nil {
		dataReader = tk.dataReader
	}
	return tk.resultCh, dataReader, nil
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

		// Signal reconnect goroutine to abort before attempting logout.
		// This prevents a reconnect from starting if the logout causes the
		// remote side to close the TCP connection while the read pump is
		// still running (which would otherwise look like a connection error).
		close(s.closed)

		if wasLoggedIn && !hasErr {
			const logoutTimeout = 5 * time.Second // enough for a single PDU round-trip
			ctx, cancel := context.WithTimeout(context.Background(), logoutTimeout)
			if err := s.logout(ctx, 0); err != nil {
				s.cfg.logger.Warn("session: logout failed during close", "err", err)
			}
			cancel()
		}

		// Snapshot window under lock — reconnect() may replace s.window
		// concurrently. We close the snapshotted window to unblock any
		// goroutines waiting on acquire.
		s.mu.Lock()
		win := s.window
		s.mu.Unlock()
		win.close()
		s.cancel()

		// Cancel all in-flight tasks and capture conn and pumpWg under lock
		// (conn may be replaced by reconnect goroutine).
		s.mu.Lock()
		for itt, tk := range s.tasks {
			tk.cancel(errors.New("session: closed"))
			s.router.Unregister(itt)
			delete(s.tasks, itt)
		}
		conn := s.conn
		wg := s.pumpWg // snapshot WG under lock
		s.mu.Unlock()

		// conn.Close() MUST precede wg.Wait(): ReadPump blocks in io.ReadFull
		// on the TCP socket; closing the conn returns io.ErrClosedPipe which
		// exits the read pump loop. Context cancel alone does NOT unblock it.
		closeErr = conn.Close()
		if wg != nil {
			wg.Wait() // deterministic: all 4 pump goroutines exited before Close returns
		}
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

// getWriteCh returns the current write channel under lock.
// This ensures goroutines always send on the current (not stale) channel
// after a reconnect replaces s.writeCh.
func (s *Session) getWriteCh() chan<- *transport.RawPDU {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeCh
}

// stampDigests computes and attaches header and/or data digests to an
// outgoing RawPDU based on the current connection's negotiated digest flags.
// This must be called before sending any PDU to writeCh during full-feature
// phase. Per RFC 7143 Section 12.1, digests cover BHS+AHS (header) and
// data segment + padding (data).
func (s *Session) stampDigests(raw *transport.RawPDU) {
	if s.conn.DigestHeader() {
		var input []byte
		if len(raw.AHS) > 0 {
			input = make([]byte, pdu.BHSLength+len(raw.AHS))
			copy(input, raw.BHS[:])
			copy(input[pdu.BHSLength:], raw.AHS)
		} else {
			input = raw.BHS[:]
		}
		raw.HeaderDigest = digest.HeaderDigest(input)
		raw.HasHDigest = true
	}
	if s.conn.DigestData() && len(raw.DataSegment) > 0 {
		raw.DataDigest = digest.DataDigest(raw.DataSegment)
		raw.HasDDigest = true
	}
}

// startPumps starts the background goroutines for the session. It captures
// current conn/channel values locally so that replaceConnection can safely
// replace them on the Session struct without racing with old goroutines.
// A per-invocation WaitGroup is created so Close and reconnect can wait for
// exactly these 4 goroutines to exit without risking an Add-after-Wait panic.
func (s *Session) startPumps(ctx context.Context) {
	conn := s.conn
	writeCh := s.writeCh
	unsolCh := s.unsolCh
	done := s.done

	// wg.Add(4) MUST be called before launching goroutines to prevent
	// a race where Wait() returns before all goroutines have started.
	wg := &sync.WaitGroup{}
	wg.Add(4)

	// Store pumpWg before launching goroutines so that a concurrent Close()
	// call cannot snapshot s.pumpWg as nil and skip wg.Wait(), which would
	// allow session teardown to race with the pump goroutines accessing s.conn.
	s.mu.Lock()
	s.pumpWg = wg
	s.mu.Unlock()

	go func() { defer wg.Done(); s.readPumpLoop(ctx, conn, unsolCh) }()
	go func() { defer wg.Done(); s.writePumpLoop(ctx, conn, writeCh) }()
	go func() { defer wg.Done(); s.dispatchLoop(ctx, unsolCh, done) }()
	go func() { defer wg.Done(); s.keepaliveLoop(ctx) }()
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
			s.cfg.pduHook(s.ctx, d, raw)
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
		s.cfg.logger, s.pduHookBridge(), conn.MaxRecvDSL(), conn.DigestByteOrder(),
		&s.dropCounter)
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
		s.cfg.logger, s.pduHookBridge(), conn.DigestByteOrder())
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
	case pdu.OpReject:
		decoded, err := pdu.DecodeBHS(raw.BHS)
		if err != nil {
			s.cfg.logger.Warn("session: failed to decode Reject PDU",
				"error", err)
			return
		}
		reject := decoded.(*pdu.Reject)
		reject.Data = raw.DataSegment

		s.cfg.logger.Warn("session: received unsolicited Reject PDU",
			"reason", fmt.Sprintf("0x%02X", reject.Reason))

		s.updateStatSN(reject.StatSN)
		s.window.update(reject.ExpCmdSN, reject.MaxCmdSN)

		// The Reject data segment contains the complete BHS of the rejected
		// PDU. Extract the ITT (bytes 16-19) to cancel the corresponding task.
		if len(reject.Data) >= 20 {
			rejectedOpcode := reject.Data[0] & 0x3f
			rejectedITT := binary.BigEndian.Uint32(reject.Data[16:20])
			s.cfg.logger.Warn("session: rejected PDU",
				"opcode", fmt.Sprintf("0x%02X", rejectedOpcode),
				"itt", fmt.Sprintf("0x%08X", rejectedITT))

			if rejectedITT != 0xFFFFFFFF {
				s.mu.Lock()
				tk, ok := s.tasks[rejectedITT]
				s.mu.Unlock()
				if ok {
					if tk.erl >= 1 {
						s.cfg.logger.Info("session: unsolicited Reject at ERL>=1, retrying same connection",
							"itt", fmt.Sprintf("0x%08x", rejectedITT),
							"reason", fmt.Sprintf("0x%02X", reject.Reason))
						s.retrySameConnection(tk)
					} else {
						tk.cancel(fmt.Errorf("session: target rejected PDU (reason=0x%02X, itt=0x%08x)", reject.Reason, rejectedITT))
						s.cleanupTask(rejectedITT)
					}
				}
			}
		} else if len(reject.Data) >= 1 {
			s.cfg.logger.Warn("session: rejected PDU opcode",
				"opcode", fmt.Sprintf("0x%02X", reject.Data[0]&0x3f))
		}
	default:
		s.cfg.logger.Warn("session: unhandled unsolicited PDU",
			"opcode", fmt.Sprintf("0x%02X", opcode))
	}
}

// taskLoop drains PDUs from the router channel for a single task.
// It runs in its own goroutine so one slow reader doesn't block others.
// The select on tk.done ensures this goroutine exits when the task is
// cancelled (e.g. during session Close) even if no more PDUs arrive on
// pduCh, preventing goroutine leaks.
func (s *Session) taskLoop(tk *task, pduCh <-chan *transport.RawPDU) {
	for {
		var raw *transport.RawPDU
		select {
		case <-tk.done:
			return
		case r, ok := <-pduCh:
			if !ok {
				return
			}
			raw = r
		}
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
			if err := tk.handleR2T(p, s.writeCh, s.getExpStatSN, s.params, s.stampDigests); err != nil {
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

		case *pdu.Reject:
			p.Data = raw.DataSegment
			s.window.update(p.ExpCmdSN, p.MaxCmdSN)
			s.updateStatSN(p.StatSN)
			if tk.erl >= 1 {
				// ERL >= 1: same-connection retry with original ITT, CDB, CmdSN
				// per RFC 7143 Section 6.2.1.
				s.cfg.logger.Info("session: Reject at ERL>=1, retrying same connection",
					"itt", fmt.Sprintf("0x%08x", tk.itt),
					"reason", fmt.Sprintf("0x%02X", p.Reason))
				s.retrySameConnection(tk)
				// Do NOT return — continue draining pduCh for the retry response.
			} else {
				tk.cancel(fmt.Errorf("session: target rejected PDU (reason=0x%02X, itt=0x%08x)", p.Reason, tk.itt))
				s.cleanupTask(tk.itt)
				return
			}

		default:
			s.cfg.logger.Warn("session: unexpected PDU for task",
				"itt", fmt.Sprintf("0x%08x", tk.itt),
				"opcode", fmt.Sprintf("0x%02X", raw.BHS[0]&0x3f))
		}
	}
}

// buildSCSICommandPDU constructs a SCSICommand PDU ready for wire encoding.
// immediateData may be nil for commands with no immediate data payload.
// The caller is responsible for digest stamping (stampDigests).
//
// This helper centralises the four PDU construction sites (Submit,
// SubmitStreaming, retrySameConnection, retryTasks) to eliminate duplication
// and ensure they all produce structurally identical PDUs.
func buildSCSICommandPDU(cmd Command, itt, cmdSN, expStatSN uint32, immediateData []byte) (*transport.RawPDU, error) {
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
	scsiCmd.LUN = pdu.EncodeSAMLUN(cmd.LUN)

	bhs, err := scsiCmd.MarshalBHS()
	if err != nil {
		return nil, err
	}

	raw := &transport.RawPDU{BHS: bhs}
	if len(immediateData) > 0 {
		raw.DataSegment = immediateData
	}
	return raw, nil
}

// cleanupTask removes a completed task from tracking.
func (s *Session) cleanupTask(itt uint32) {
	// Unregister (not UnregisterAndClose) is intentional: taskLoop exits via
	// tk.done, not via channel close. Closing the channel here would risk a
	// send-on-closed-channel panic if a late Dispatch call races with the close.
	// Any PDU arriving after Unregister removes the map entry is silently
	// discarded by the dispatcher — the bounded channel (persistentDepth)
	// prevents blocking.
	s.router.Unregister(itt)
	s.mu.Lock()
	delete(s.tasks, itt)
	s.mu.Unlock()
}

// retrySameConnection re-sends a SCSI command on the same connection using
// the original ITT, CDB, and CmdSN per RFC 7143 Section 6.2.1. This is the
// ERL >= 1 same-connection retry path, distinct from the ERL 0 reconnect
// retry in retryTasks which allocates new ITT and CmdSN.
func (s *Session) retrySameConnection(tk *task) {
	// Streaming tasks cannot be retried — caller holds the chanReader.
	if tk.streaming {
		tk.cancel(fmt.Errorf("session: streaming task not retriable"))
		return
	}

	cmd := tk.cmd

	// Reset read buffer for reads.
	tk.nextDataSN = 0
	tk.nextOffset = 0
	if tk.isRead && tk.buf != nil {
		tk.buf.Reset()
	}

	// Re-read immediate data for write commands.
	var immediateData []byte
	if tk.isWrite && s.params.ImmediateData && tk.reader != nil {
		if seeker, ok := tk.reader.(io.Seeker); ok {
			if _, err := seeker.Seek(0, io.SeekStart); err != nil {
				tk.cancel(fmt.Errorf("session: same-connection retry seek failed: %w", err))
				return
			}
			tk.bytesSent = 0
		} else {
			tk.cancel(ErrRetryNotPossible)
			return
		}
		immLen := min(s.params.FirstBurstLength, s.params.MaxRecvDataSegmentLength)
		immBuf := make([]byte, immLen)
		n, readErr := io.ReadFull(tk.reader, immBuf)
		if readErr != nil && readErr != io.ErrUnexpectedEOF {
			tk.cancel(fmt.Errorf("session: same-connection retry read immediate data: %w", readErr))
			return
		}
		immediateData = immBuf[:n]
		tk.bytesSent = uint32(n)
	}

	raw, encErr := buildSCSICommandPDU(cmd, tk.itt, tk.cmdSN, s.getExpStatSN(), immediateData)
	if encErr != nil {
		tk.cancel(fmt.Errorf("session: same-connection retry encode: %w", encErr))
		return
	}
	s.stampDigests(raw)

	select {
	case s.writeCh <- raw:
		s.cfg.logger.Info("session: same-connection retry sent",
			"itt", fmt.Sprintf("0x%08x", tk.itt),
			"cmd_sn", tk.cmdSN)
	default:
		tk.cancel(fmt.Errorf("session: same-connection retry write channel full"))
	}
}
