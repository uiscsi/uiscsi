package session

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/rkujawa/uiscsi/internal/login"
	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/serial"
	"github.com/rkujawa/uiscsi/internal/transport"
)

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

	mu        sync.Mutex
	expStatSN uint32
	tasks     map[uint32]*task
	loggedIn  bool // true while session is in full-feature phase

	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
	err       error

	cfg sessionConfig
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
		conn:      conn,
		params:    params,
		router:    transport.NewRouter(),
		writeCh:   make(chan *transport.RawPDU, 64),
		unsolCh:   make(chan *transport.RawPDU, 16),
		window:    newCmdWindow(params.CmdSN, params.CmdSN, params.CmdSN),
		expStatSN: params.ExpStatSN,
		tasks:     make(map[uint32]*task),
		loggedIn:  true,
		cancel:    cancel,
		done:      make(chan struct{}),
		cfg:       cfg,
	}

	// Start background goroutines.
	go s.readPumpLoop(ctx)
	go s.writePumpLoop(ctx)
	go s.dispatchLoop(ctx)
	go s.keepaliveLoop(ctx)

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
	// Acquire a CmdSN slot (blocks if window is full).
	cmdSN, err := s.window.acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("session: acquire CmdSN: %w", err)
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
		return nil, fmt.Errorf("session: encode SCSICommand: %w", encErr)
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
		s.mu.Unlock()

		if wasLoggedIn && s.err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = s.logout(ctx, 0)
			cancel()
		}

		s.window.close()
		s.cancel()

		// Cancel all in-flight tasks.
		s.mu.Lock()
		for itt, tk := range s.tasks {
			tk.cancel(errors.New("session: closed"))
			s.router.Unregister(itt)
			delete(s.tasks, itt)
		}
		s.mu.Unlock()

		closeErr = s.conn.Close()
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

// readPumpLoop runs the transport read pump.
func (s *Session) readPumpLoop(ctx context.Context) {
	err := transport.ReadPump(ctx, s.conn.NetConn(), s.router, s.unsolCh,
		s.conn.DigestHeader(), s.conn.DigestData())
	if err != nil && ctx.Err() == nil {
		s.mu.Lock()
		if s.err == nil {
			s.err = fmt.Errorf("session: read pump: %w", err)
		}
		s.mu.Unlock()
	}
}

// writePumpLoop runs the transport write pump.
func (s *Session) writePumpLoop(ctx context.Context) {
	err := transport.WritePump(ctx, s.conn.NetConn(), s.writeCh)
	if err != nil && ctx.Err() == nil {
		s.mu.Lock()
		if s.err == nil {
			s.err = fmt.Errorf("session: write pump: %w", err)
		}
		s.mu.Unlock()
	}
}

// dispatchLoop handles unsolicited PDUs (ITT=0xFFFFFFFF) from the target.
func (s *Session) dispatchLoop(ctx context.Context) {
	defer close(s.done)
	for {
		select {
		case <-ctx.Done():
			return
		case raw, ok := <-s.unsolCh:
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
			tk.cancel(fmt.Errorf("session: decode response PDU: %w", err))
			s.cleanupTask(tk.itt)
			return
		}

		switch p := decoded.(type) {
		case *pdu.DataIn:
			p.Data = raw.DataSegment
			s.window.update(p.ExpCmdSN, p.MaxCmdSN)
			if p.HasStatus {
				s.updateStatSN(p.StatSN)
			}
			tk.handleDataIn(p)
			if p.HasStatus {
				s.cleanupTask(tk.itt)
				return
			}

		case *pdu.R2T:
			s.window.update(p.ExpCmdSN, p.MaxCmdSN)
			s.updateStatSN(p.StatSN)
			if err := tk.handleR2T(p, s.writeCh, s.getExpStatSN, s.params); err != nil {
				tk.cancel(fmt.Errorf("session: write data: %w", err))
				s.cleanupTask(tk.itt)
				return
			}

		case *pdu.SCSIResponse:
			p.Data = raw.DataSegment
			s.window.update(p.ExpCmdSN, p.MaxCmdSN)
			s.updateStatSN(p.StatSN)
			tk.handleSCSIResponse(p)
			s.cleanupTask(tk.itt)
			return

		default:
			s.cfg.logger.Warn("session: unexpected PDU for task",
				"itt", tk.itt,
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
