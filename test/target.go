// Package test provides an in-process mock iSCSI target for testing.
// It implements a minimal target that accepts TCP connections, handles
// iSCSI login negotiation, and responds to SCSI commands with programmable
// data. Used by the conformance test suite (test/conformance/) and
// available for external test consumers.
package test

import (
	"encoding/binary"
	"fmt"
	"log"
	"log/slog"
	"net"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/rkujawa/uiscsi/internal/login"
	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
)

// PDUHandler processes a received PDU and returns an error if the
// response could not be sent. Handlers use tc to send response PDUs.
type PDUHandler func(tc *TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error

// TargetConn wraps a connection to a single initiator, providing
// helper methods for sending response PDUs.
type TargetConn struct {
	nc     net.Conn
	mu     sync.Mutex // serialize writes
	statSN uint32
}

// SendPDU marshals and sends a PDU to the initiator.
func (tc *TargetConn) SendPDU(p pdu.PDU) error {
	raw, err := BuildRawPDU(p)
	if err != nil {
		return err
	}
	return tc.SendRaw(raw)
}

// SendRaw sends a pre-built RawPDU.
func (tc *TargetConn) SendRaw(raw *transport.RawPDU) error {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return transport.WriteRawPDU(tc.nc, raw)
}

// NextStatSN returns and increments the StatSN counter.
func (tc *TargetConn) NextStatSN() uint32 {
	return atomic.AddUint32(&tc.statSN, 1) - 1
}

// StatSN returns the current StatSN without incrementing.
func (tc *TargetConn) StatSN() uint32 {
	return atomic.LoadUint32(&tc.statSN)
}

// Close closes the underlying connection.
func (tc *TargetConn) Close() error {
	return tc.nc.Close()
}

// ReadPDU reads the next PDU from the initiator. Used by HandleSCSIFunc handlers
// to receive Data-Out PDUs inline during write sequences.
func (tc *TargetConn) ReadPDU() (pdu.PDU, *transport.RawPDU, error) {
	raw, err := transport.ReadRawPDU(tc.nc, false, false, 0)
	if err != nil {
		return nil, nil, err
	}
	decoded, err := pdu.DecodeBHS(raw.BHS)
	if err != nil {
		return nil, raw, err
	}
	attachDataSegment(decoded, raw.DataSegment)
	return decoded, raw, nil
}

// SessionState tracks target-side command sequencing state.
// Handlers call Update() to get correct ExpCmdSN/MaxCmdSN for responses
// instead of hardcoding cmd.CmdSN+1/cmd.CmdSN+10.
type SessionState struct {
	mu            sync.Mutex
	expCmdSN      uint32
	maxCmdSNDelta int32 // MaxCmdSN = ExpCmdSN + delta; default 10
	initialized   bool
}

// NewSessionState returns a SessionState with a default MaxCmdSN delta of 10.
func NewSessionState() *SessionState {
	return &SessionState{maxCmdSNDelta: 10}
}

// Update advances ExpCmdSN based on the received command's CmdSN and returns
// the current ExpCmdSN and MaxCmdSN for use in response PDUs. Immediate
// commands do not advance ExpCmdSN per RFC 7143 Section 3.2.2.1.
func (ss *SessionState) Update(cmdSN uint32, immediate bool) (expCmdSN, maxCmdSN uint32) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	if !ss.initialized {
		ss.expCmdSN = cmdSN
		ss.initialized = true
	}
	if !immediate {
		ss.expCmdSN = cmdSN + 1
	}
	maxCmdSN = uint32(int32(ss.expCmdSN) + ss.maxCmdSNDelta)
	return ss.expCmdSN, maxCmdSN
}

// SetMaxCmdSNDelta configures the delta between ExpCmdSN and MaxCmdSN.
// Use negative values to create a closed command window for flow control tests.
func (ss *SessionState) SetMaxCmdSNDelta(delta int32) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	ss.maxCmdSNDelta = delta
}

// ExpCmdSN returns the current expected command sequence number.
func (ss *SessionState) ExpCmdSN() uint32 {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	return ss.expCmdSN
}

// NegotiationConfig controls what the target offers during login negotiation.
// Nil pointer fields mean "echo the initiator's proposal" (existing behavior).
type NegotiationConfig struct {
	ImmediateData            *bool
	InitialR2T               *bool
	FirstBurstLength         *uint32
	MaxBurstLength           *uint32
	MaxRecvDataSegmentLength *uint32
	ErrorRecoveryLevel       *uint32
}

// BoolPtr returns a pointer to the given bool value. Used in NegotiationConfig.
func BoolPtr(v bool) *bool { return &v }

// boolToYesNo converts a bool to iSCSI "Yes"/"No" string per RFC 7143.
func boolToYesNo(v bool) string {
	if v {
		return "Yes"
	}
	return "No"
}

// Uint32Ptr returns a pointer to the given uint32 value. Used in NegotiationConfig.
func Uint32Ptr(v uint32) *uint32 { return &v }

// MockTarget is an in-process iSCSI target for testing.
// It listens on a local TCP port, accepts connections, and dispatches
// received PDUs to registered handlers by opcode.
type MockTarget struct {
	listener net.Listener
	handlers map[pdu.OpCode]PDUHandler
	mu       sync.Mutex
	conns    []*TargetConn
	done     chan struct{}
	wg       sync.WaitGroup
	closed   atomic.Bool
	strict            bool // if true, unhandled opcodes close the connection
	session           *SessionState
	negotiationConfig NegotiationConfig
}

// SetStrictMode configures whether unhandled opcodes cause connection errors.
// In strict mode, receiving an unhandled opcode logs an error and closes the
// connection, which surfaces test setup bugs immediately.
func (mt *MockTarget) SetStrictMode(strict bool) {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	mt.strict = strict
}

// NewMockTarget starts a mock target listening on 127.0.0.1:0.
// It automatically accepts connections and dispatches PDUs to handlers.
func NewMockTarget() (*MockTarget, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	mt := &MockTarget{
		listener: ln,
		handlers: make(map[pdu.OpCode]PDUHandler),
		done:     make(chan struct{}),
		session:  NewSessionState(),
	}

	mt.wg.Add(1)
	go mt.acceptLoop()

	return mt, nil
}

// Addr returns the listener address (host:port) for use with uiscsi.Dial.
func (mt *MockTarget) Addr() string {
	return mt.listener.Addr().String()
}

// Handle registers a handler for the given opcode.
func (mt *MockTarget) Handle(opcode pdu.OpCode, h PDUHandler) {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	mt.handlers[opcode] = h
}

// Session returns the MockTarget's SessionState for configuring command
// window behavior in tests.
func (mt *MockTarget) Session() *SessionState {
	return mt.session
}

// SetNegotiationConfig configures the negotiation parameters the target
// will offer during login. Non-nil fields override the default echo behavior.
func (mt *MockTarget) SetNegotiationConfig(cfg NegotiationConfig) {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	mt.negotiationConfig = cfg
}

// HandleSCSIFunc registers a flexible SCSI command handler where the test
// function receives the decoded *pdu.SCSICommand and a 0-based call counter.
// This replaces any previously registered OpSCSICommand handler (HandleSCSIRead,
// HandleSCSIWrite, HandleSCSIError).
func (mt *MockTarget) HandleSCSIFunc(h func(tc *TargetConn, cmd *pdu.SCSICommand, callCount int) error) {
	var count atomic.Int32
	mt.Handle(pdu.OpSCSICommand, func(tc *TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		cmd := decoded.(*pdu.SCSICommand)
		n := int(count.Add(1) - 1)
		return h(tc, cmd, n)
	})
}

// Conns returns the currently tracked connections.
func (mt *MockTarget) Conns() []*TargetConn {
	mt.mu.Lock()
	defer mt.mu.Unlock()
	result := make([]*TargetConn, len(mt.conns))
	copy(result, mt.conns)
	return result
}

// Close shuts down the listener and waits for all goroutines to finish.
func (mt *MockTarget) Close() error {
	if mt.closed.Swap(true) {
		return nil // already closed
	}
	close(mt.done)
	err := mt.listener.Close()
	// Close all active connections to unblock serve loops.
	mt.mu.Lock()
	for _, tc := range mt.conns {
		tc.nc.Close()
	}
	mt.mu.Unlock()
	mt.wg.Wait()
	return err
}

func (mt *MockTarget) acceptLoop() {
	defer mt.wg.Done()
	for {
		nc, err := mt.listener.Accept()
		if err != nil {
			select {
			case <-mt.done:
				return
			default:
				log.Printf("mock target: accept error: %v", err)
				return
			}
		}

		tc := &TargetConn{nc: nc, statSN: 1}
		mt.mu.Lock()
		mt.conns = append(mt.conns, tc)
		mt.mu.Unlock()

		mt.wg.Add(1)
		go mt.serveConn(tc)
	}
}

func (mt *MockTarget) serveConn(tc *TargetConn) {
	defer mt.wg.Done()
	for {
		raw, err := transport.ReadRawPDU(tc.nc, false, false, 0)
		if err != nil {
			return // connection closed or error
		}

		decoded, err := pdu.DecodeBHS(raw.BHS)
		if err != nil {
			return // unrecognized opcode
		}

		// Attach data segment to decoded PDU types that carry data.
		attachDataSegment(decoded, raw.DataSegment)

		opcode := decoded.Opcode()
		mt.mu.Lock()
		handler, ok := mt.handlers[opcode]
		mt.mu.Unlock()

		if !ok {
			slog.Warn("mock target: unhandled opcode",
				"opcode", fmt.Sprintf("0x%02X", opcode),
				"remote", tc.nc.RemoteAddr())
			mt.mu.Lock()
			strict := mt.strict
			mt.mu.Unlock()
			if strict {
				return // close connection on unhandled opcode in strict mode
			}
			continue
		}

		if err := handler(tc, raw, decoded); err != nil {
			return // handler error, close connection
		}
	}
}

// attachDataSegment copies the raw data segment into the appropriate
// field of the decoded PDU. DecodeBHS only decodes the BHS header;
// data segments must be attached separately.
func attachDataSegment(p pdu.PDU, data []byte) {
	if len(data) == 0 {
		return
	}
	switch v := p.(type) {
	case *pdu.LoginReq:
		v.Data = data
	case *pdu.TextReq:
		v.Data = data
	case *pdu.SCSICommand:
		v.ImmediateData = data
	case *pdu.DataOut:
		v.Data = data
	case *pdu.NOPOut:
		v.Data = data
	}
}

// HandleLogin registers an OpLoginReq handler that:
//  1. Parses login request, reads CSG/NSG/transit
//  2. Handles security phase (AuthMethod=None) and operational negotiation
//  3. Accepts all proposed operational parameters (echoes target values)
//  4. Sets TSIH=1 in final response
func (mt *MockTarget) HandleLogin() {
	mt.Handle(pdu.OpLoginReq, func(tc *TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		req := decoded.(*pdu.LoginReq)
		kvs := login.DecodeTextKV(req.Data)

		// Seed SessionState with login CmdSN so ExpCmdSN is correct
		// for the first full-feature phase command.
		mt.session.Update(req.CmdSN, false)

		switch req.CSG {
		case 0: // Security Negotiation
			// Build response keys.
			var respKVs []login.KeyValue
			for _, kv := range kvs {
				switch kv.Key {
				case "AuthMethod":
					respKVs = append(respKVs, login.KeyValue{Key: "AuthMethod", Value: "None"})
				}
			}

			if req.Transit && req.NSG == 1 {
				// Transit to operational negotiation.
				resp := &pdu.LoginResp{
					Header: pdu.Header{
						Final:            true,
						InitiatorTaskTag: req.InitiatorTaskTag,
						DataSegmentLen:   uint32(len(login.EncodeTextKV(respKVs))),
					},
					Transit:       true,
					CSG:           0,
					NSG:           1,
					VersionMax:    0x00,
					VersionActive: 0x00,
					ISID:          req.ISID,
					StatSN:        tc.NextStatSN(),
					ExpCmdSN:      req.CmdSN,
					MaxCmdSN:      req.CmdSN + 10,
					StatusClass:   0,
					Data:          login.EncodeTextKV(respKVs),
				}
				return tc.SendPDU(resp)
			}

			// Transit directly to full-feature phase.
			if req.Transit && req.NSG == 3 {
				resp := &pdu.LoginResp{
					Header: pdu.Header{
						Final:            true,
						InitiatorTaskTag: req.InitiatorTaskTag,
					},
					Transit:       true,
					CSG:           0,
					NSG:           3,
					VersionMax:    0x00,
					VersionActive: 0x00,
					ISID:          req.ISID,
					TSIH:          1,
					StatSN:        tc.NextStatSN(),
					ExpCmdSN:      req.CmdSN,
					MaxCmdSN:      req.CmdSN + 10,
					StatusClass:   0,
					Data:          login.EncodeTextKV(respKVs),
				}
				return tc.SendPDU(resp)
			}

			// Non-transit response.
			resp := &pdu.LoginResp{
				Header: pdu.Header{
					Final:            true,
					InitiatorTaskTag: req.InitiatorTaskTag,
				},
				CSG:           0,
				NSG:           0,
				VersionMax:    0x00,
				VersionActive: 0x00,
				ISID:          req.ISID,
				StatSN:        tc.NextStatSN(),
				ExpCmdSN:      req.CmdSN,
				MaxCmdSN:      req.CmdSN + 10,
				StatusClass:   0,
				Data:          login.EncodeTextKV(respKVs),
			}
			return tc.SendPDU(resp)

		case 1: // Operational Negotiation
			// Echo back operational parameters with target values,
			// respecting NegotiationConfig overrides where set.
			mt.mu.Lock()
			negCfg := mt.negotiationConfig
			mt.mu.Unlock()

			var respKVs []login.KeyValue
			for _, kv := range kvs {
				switch kv.Key {
				case "HeaderDigest", "DataDigest":
					respKVs = append(respKVs, login.KeyValue{Key: kv.Key, Value: "None"})
				case "ImmediateData":
					if negCfg.ImmediateData != nil {
						respKVs = append(respKVs, login.KeyValue{Key: kv.Key, Value: boolToYesNo(*negCfg.ImmediateData)})
					} else {
						respKVs = append(respKVs, login.KeyValue{Key: kv.Key, Value: kv.Value})
					}
				case "InitialR2T":
					if negCfg.InitialR2T != nil {
						respKVs = append(respKVs, login.KeyValue{Key: kv.Key, Value: boolToYesNo(*negCfg.InitialR2T)})
					} else {
						respKVs = append(respKVs, login.KeyValue{Key: kv.Key, Value: kv.Value})
					}
				case "FirstBurstLength":
					if negCfg.FirstBurstLength != nil {
						respKVs = append(respKVs, login.KeyValue{Key: kv.Key, Value: strconv.FormatUint(uint64(*negCfg.FirstBurstLength), 10)})
					} else {
						respKVs = append(respKVs, login.KeyValue{Key: kv.Key, Value: kv.Value})
					}
				case "MaxBurstLength":
					if negCfg.MaxBurstLength != nil {
						respKVs = append(respKVs, login.KeyValue{Key: kv.Key, Value: strconv.FormatUint(uint64(*negCfg.MaxBurstLength), 10)})
					} else {
						respKVs = append(respKVs, login.KeyValue{Key: kv.Key, Value: kv.Value})
					}
				case "MaxRecvDataSegmentLength":
					if negCfg.MaxRecvDataSegmentLength != nil {
						respKVs = append(respKVs, login.KeyValue{Key: kv.Key, Value: strconv.FormatUint(uint64(*negCfg.MaxRecvDataSegmentLength), 10)})
					} else {
						// Declarative: target declares its own value.
						respKVs = append(respKVs, login.KeyValue{Key: kv.Key, Value: "8192"})
					}
				case "ErrorRecoveryLevel":
					if negCfg.ErrorRecoveryLevel != nil {
						respKVs = append(respKVs, login.KeyValue{Key: kv.Key, Value: strconv.FormatUint(uint64(*negCfg.ErrorRecoveryLevel), 10)})
					} else {
						respKVs = append(respKVs, login.KeyValue{Key: kv.Key, Value: kv.Value})
					}
				default:
					// Echo back the proposed value (accept everything).
					respKVs = append(respKVs, login.KeyValue{Key: kv.Key, Value: kv.Value})
				}
			}

			data := login.EncodeTextKV(respKVs)
			resp := &pdu.LoginResp{
				Header: pdu.Header{
					Final:            true,
					InitiatorTaskTag: req.InitiatorTaskTag,
					DataSegmentLen:   uint32(len(data)),
				},
				Transit:       true,
				CSG:           1,
				NSG:           3,
				VersionMax:    0x00,
				VersionActive: 0x00,
				ISID:          req.ISID,
				TSIH:          1,
				StatSN:        tc.NextStatSN(),
				ExpCmdSN:      req.CmdSN,
				MaxCmdSN:      req.CmdSN + 10,
				StatusClass:   0,
				Data:          data,
			}
			return tc.SendPDU(resp)
		}
		return nil
	})
}

// HandleSCSIRead registers a handler that responds to SCSI read commands
// on the specified LUN with the provided data. It sends a single DataIn
// PDU with status followed by (implicit) SCSIResponse.
func (mt *MockTarget) HandleSCSIRead(lun uint64, data []byte) {
	mt.Handle(pdu.OpSCSICommand, func(tc *TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		cmd := decoded.(*pdu.SCSICommand)

		if cmd.Read {
			// Determine how much data to send (min of expected and available).
			sendLen := int(cmd.ExpectedDataTransferLength)
			if sendLen > len(data) {
				sendLen = len(data)
			}

			statSN := tc.NextStatSN()

			// Send DataIn with status.
			din := &pdu.DataIn{
				Header: pdu.Header{
					Final:            true,
					InitiatorTaskTag: cmd.InitiatorTaskTag,
					DataSegmentLen:   uint32(sendLen),
				},
				HasStatus: true,
				Status:    0x00,
				StatSN:    statSN,
				ExpCmdSN:  cmd.CmdSN + 1,
				MaxCmdSN:  cmd.CmdSN + 10,
				DataSN:    0,
				Data:      data[:sendLen],
			}
			return tc.SendPDU(din)
		}

		if cmd.Write {
			// For write commands, accept the data and send success.
			statSN := tc.NextStatSN()
			resp := &pdu.SCSIResponse{
				Header: pdu.Header{
					Final:            true,
					InitiatorTaskTag: cmd.InitiatorTaskTag,
				},
				Status:   0x00,
				StatSN:   statSN,
				ExpCmdSN: cmd.CmdSN + 1,
				MaxCmdSN: cmd.CmdSN + 10,
			}
			return tc.SendPDU(resp)
		}

		// Non-read/write command (e.g., TEST UNIT READY, INQUIRY).
		statSN := tc.NextStatSN()

		// Check CDB opcode to determine if this is INQUIRY (0x12)
		// or other commands that return data.
		cdbOp := cmd.CDB[0]
		switch {
		case cdbOp == 0x12: // INQUIRY
			sendLen := int(cmd.ExpectedDataTransferLength)
			if sendLen > len(data) {
				sendLen = len(data)
			}
			din := &pdu.DataIn{
				Header: pdu.Header{
					Final:            true,
					InitiatorTaskTag: cmd.InitiatorTaskTag,
					DataSegmentLen:   uint32(sendLen),
				},
				HasStatus: true,
				Status:    0x00,
				StatSN:    statSN,
				ExpCmdSN:  cmd.CmdSN + 1,
				MaxCmdSN:  cmd.CmdSN + 10,
				DataSN:    0,
				Data:      data[:sendLen],
			}
			return tc.SendPDU(din)

		case cdbOp == 0x25 || cdbOp == 0x9e: // READ CAPACITY(10) or SERVICE ACTION IN(16) for READ CAPACITY(16)
			sendLen := int(cmd.ExpectedDataTransferLength)
			if sendLen > len(data) {
				sendLen = len(data)
			}
			din := &pdu.DataIn{
				Header: pdu.Header{
					Final:            true,
					InitiatorTaskTag: cmd.InitiatorTaskTag,
					DataSegmentLen:   uint32(sendLen),
				},
				HasStatus: true,
				Status:    0x00,
				StatSN:    statSN,
				ExpCmdSN:  cmd.CmdSN + 1,
				MaxCmdSN:  cmd.CmdSN + 10,
				DataSN:    0,
				Data:      data[:sendLen],
			}
			return tc.SendPDU(din)

		case cdbOp == 0xa0: // REPORT LUNS
			sendLen := int(cmd.ExpectedDataTransferLength)
			if sendLen > len(data) {
				sendLen = len(data)
			}
			din := &pdu.DataIn{
				Header: pdu.Header{
					Final:            true,
					InitiatorTaskTag: cmd.InitiatorTaskTag,
					DataSegmentLen:   uint32(sendLen),
				},
				HasStatus: true,
				Status:    0x00,
				StatSN:    statSN,
				ExpCmdSN:  cmd.CmdSN + 1,
				MaxCmdSN:  cmd.CmdSN + 10,
				DataSN:    0,
				Data:      data[:sendLen],
			}
			return tc.SendPDU(din)

		default:
			// No data response (TEST UNIT READY, etc.).
			resp := &pdu.SCSIResponse{
				Header: pdu.Header{
					Final:            true,
					InitiatorTaskTag: cmd.InitiatorTaskTag,
				},
				Status:   0x00,
				StatSN:   statSN,
				ExpCmdSN: cmd.CmdSN + 1,
				MaxCmdSN: cmd.CmdSN + 10,
			}
			return tc.SendPDU(resp)
		}
	})
}

// HandleSCSIWrite registers a handler for SCSI write commands.
// Accepts immediate data and sends SCSIResponse(status=0).
func (mt *MockTarget) HandleSCSIWrite(lun uint64) {
	mt.Handle(pdu.OpSCSICommand, func(tc *TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		cmd := decoded.(*pdu.SCSICommand)
		statSN := tc.NextStatSN()
		resp := &pdu.SCSIResponse{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
			},
			Status:   0x00,
			StatSN:   statSN,
			ExpCmdSN: cmd.CmdSN + 1,
			MaxCmdSN: cmd.CmdSN + 10,
		}
		return tc.SendPDU(resp)
	})
}

// HandleLogout registers a handler for Logout requests.
func (mt *MockTarget) HandleLogout() {
	mt.Handle(pdu.OpLogoutReq, func(tc *TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		req := decoded.(*pdu.LogoutReq)
		statSN := tc.NextStatSN()
		resp := &pdu.LogoutResp{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: req.InitiatorTaskTag,
			},
			Response: 0,
			StatSN:   statSN,
			ExpCmdSN: req.CmdSN + 1,
			MaxCmdSN: req.CmdSN + 10,
		}
		return tc.SendPDU(resp)
	})
}

// HandleNOPOut registers a handler for NOP-Out (keepalive) requests.
func (mt *MockTarget) HandleNOPOut() {
	mt.Handle(pdu.OpNOPOut, func(tc *TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		req := decoded.(*pdu.NOPOut)
		statSN := tc.NextStatSN()
		resp := &pdu.NOPIn{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: req.InitiatorTaskTag,
			},
			TargetTransferTag: 0xFFFFFFFF,
			StatSN:            statSN,
			ExpCmdSN:          req.CmdSN + 1,
			MaxCmdSN:          req.CmdSN + 10,
			Data:              req.Data,
		}
		return tc.SendPDU(resp)
	})
}

// HandleTMF registers a handler for Task Management Function requests.
// Simply responds with TMFResp(Response=0) for all TMF requests.
func (mt *MockTarget) HandleTMF() {
	mt.Handle(pdu.OpTaskMgmtReq, func(tc *TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		req := decoded.(*pdu.TaskMgmtReq)
		statSN := tc.NextStatSN()
		resp := &pdu.TaskMgmtResp{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: req.InitiatorTaskTag,
			},
			Response: 0, // Function complete
			StatSN:   statSN,
			// TMF is Immediate — does not advance CmdSN per RFC 7143 Section 3.2.2.1.
			// ExpCmdSN must NOT be incremented for immediate commands.
			ExpCmdSN: req.CmdSN,
			MaxCmdSN: req.CmdSN + 10,
		}
		return tc.SendPDU(resp)
	})
}

// HandleSCSIError registers a SCSI command handler that always returns
// the specified status with optional sense data. Useful for testing
// error recovery paths.
func (mt *MockTarget) HandleSCSIError(status uint8, senseData []byte) {
	mt.Handle(pdu.OpSCSICommand, func(tc *TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		cmd := decoded.(*pdu.SCSICommand)
		statSN := tc.NextStatSN()

		// Per RFC 7143 Section 11.4.7.2, the SCSI Response data segment
		// is [SenseLength (2 bytes, big-endian)] [Sense Data].
		dataSegment := make([]byte, 2+len(senseData))
		binary.BigEndian.PutUint16(dataSegment[0:2], uint16(len(senseData)))
		copy(dataSegment[2:], senseData)

		resp := &pdu.SCSIResponse{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
				DataSegmentLen:   uint32(len(dataSegment)),
			},
			Status:   status,
			StatSN:   statSN,
			ExpCmdSN: cmd.CmdSN + 1,
			MaxCmdSN: cmd.CmdSN + 10,
			Data:     dataSegment,
		}
		return tc.SendPDU(resp)
	})
}

// HandleDiscovery registers a TextReq handler that responds with
// SendTargets discovery data. Used by Discover() tests.
func (mt *MockTarget) HandleDiscovery(targets []login.KeyValue) {
	mt.Handle(pdu.OpTextReq, func(tc *TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		req := decoded.(*pdu.TextReq)
		data := login.EncodeTextKV(targets)
		statSN := tc.NextStatSN()
		resp := &pdu.TextResp{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: req.InitiatorTaskTag,
				DataSegmentLen:   uint32(len(data)),
			},
			TargetTransferTag: 0xFFFFFFFF,
			StatSN:            statSN,
			ExpCmdSN:          req.CmdSN + 1,
			MaxCmdSN:          req.CmdSN + 10,
			Data:              data,
		}
		return tc.SendPDU(resp)
	})
}

// HandleSCSIReadMultiPDU registers a handler that responds to SCSI read commands
// by splitting data into multiple Data-In PDUs of pduSize bytes each.
// The last PDU carries HasStatus=true (S-bit) and Final=true (F-bit).
// DataSN increments from 0. BufferOffset tracks cumulative offset.
// Uses SessionState for correct ExpCmdSN/MaxCmdSN.
func (mt *MockTarget) HandleSCSIReadMultiPDU(lun uint64, data []byte, pduSize int) {
	mt.Handle(pdu.OpSCSICommand, func(tc *TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		cmd := decoded.(*pdu.SCSICommand)
		expCmdSN, maxCmdSN := mt.session.Update(cmd.CmdSN, cmd.Header.Immediate)
		edtl := int(cmd.ExpectedDataTransferLength)
		sendLen := edtl
		if sendLen > len(data) {
			sendLen = len(data)
		}

		var offset, dataSN uint32
		for int(offset) < sendLen {
			chunk := pduSize
			if int(offset)+chunk > sendLen {
				chunk = sendLen - int(offset)
			}
			isFinal := int(offset)+chunk >= sendLen

			din := &pdu.DataIn{
				Header: pdu.Header{
					Final:            isFinal,
					InitiatorTaskTag: cmd.InitiatorTaskTag,
					DataSegmentLen:   uint32(chunk),
				},
				DataSN:       dataSN,
				BufferOffset: offset,
				ExpCmdSN:     expCmdSN,
				MaxCmdSN:     maxCmdSN,
				Data:         data[offset : offset+uint32(chunk)],
			}
			if isFinal {
				din.HasStatus = true
				din.Status = 0x00
				din.StatSN = tc.NextStatSN()
			}
			if err := tc.SendPDU(din); err != nil {
				return err
			}
			offset += uint32(chunk)
			dataSN++
		}
		return nil
	})
}

// SendR2TSequence sends R2T PDUs for a write command, splitting totalLen into
// bursts of burstLen bytes. Each R2T gets a unique TTT (starting from baseTTT),
// incrementing R2TSN, and correct BufferOffset. Returns the TTT values assigned
// to each R2T for Data-Out verification.
func SendR2TSequence(tc *TargetConn, itt uint32, startOffset uint32,
	totalLen uint32, burstLen uint32, baseTTT uint32, session *SessionState) ([]uint32, error) {
	var ttts []uint32
	offset := startOffset
	remaining := totalLen
	var r2tsn uint32
	expCmdSN := session.ExpCmdSN()
	maxCmdSN := uint32(int32(expCmdSN) + 10) // default window

	for remaining > 0 {
		desired := burstLen
		if desired > remaining {
			desired = remaining
		}
		ttt := baseTTT + r2tsn
		r2t := &pdu.R2T{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: itt,
			},
			TargetTransferTag:        ttt,
			StatSN:                   tc.StatSN(), // R2T does not advance StatSN
			ExpCmdSN:                 expCmdSN,
			MaxCmdSN:                 maxCmdSN,
			R2TSN:                    r2tsn,
			BufferOffset:             offset,
			DesiredDataTransferLength: desired,
		}
		if err := tc.SendPDU(r2t); err != nil {
			return ttts, err
		}
		ttts = append(ttts, ttt)
		offset += desired
		remaining -= desired
		r2tsn++
	}
	return ttts, nil
}

// ReadDataOutPDUs reads Data-Out PDUs from the initiator until one with F-bit
// (Final) is received. Returns all received Data-Out PDUs in order.
// Used by HandleSCSIFunc handlers to collect solicited write data.
func ReadDataOutPDUs(tc *TargetConn) ([]*pdu.DataOut, error) {
	var result []*pdu.DataOut
	for {
		decoded, _, err := tc.ReadPDU()
		if err != nil {
			return result, err
		}
		dout, ok := decoded.(*pdu.DataOut)
		if !ok {
			// Skip non-DataOut PDUs (e.g., NOP-Out keepalive)
			continue
		}
		result = append(result, dout)
		if dout.Header.Final {
			return result, nil
		}
	}
}

// BuildRawPDU marshals a PDU into a RawPDU for wire transmission.
func BuildRawPDU(p pdu.PDU) (*transport.RawPDU, error) {
	bhs, err := p.MarshalBHS()
	if err != nil {
		return nil, err
	}
	raw := &transport.RawPDU{BHS: bhs}
	if ds := p.DataSegment(); len(ds) > 0 {
		raw.DataSegment = ds
	}
	return raw, nil
}

// BuildLoginResp builds a LoginResp PDU for custom login test scenarios.
func BuildLoginResp(req *pdu.LoginReq, keys []login.KeyValue, transit bool, csg, nsg uint8) *pdu.LoginResp {
	data := login.EncodeTextKV(keys)
	return &pdu.LoginResp{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: req.InitiatorTaskTag,
			DataSegmentLen:   uint32(len(data)),
		},
		Transit:       transit,
		CSG:           csg,
		NSG:           nsg,
		VersionMax:    0x00,
		VersionActive: 0x00,
		ISID:          req.ISID,
		TSIH:          1,
		StatSN:        1,
		ExpCmdSN:      req.CmdSN,
		MaxCmdSN:      req.CmdSN + 10,
		StatusClass:   0,
		Data:          data,
	}
}

// BuildInquiryData builds a minimal INQUIRY response data suitable for
// testing. Returns bytes in standard INQUIRY format.
func BuildInquiryData(vendor, product, revision string) []byte {
	data := make([]byte, 96)
	data[0] = 0x00          // Peripheral device type: disk
	data[1] = 0x00          // Not removable
	data[2] = 0x06          // SPC-4 version
	data[3] = 0x02          // Response data format 2
	data[4] = 91            // Additional length (96-5)
	copy(data[8:16], pad(vendor, 8))
	copy(data[16:32], pad(product, 16))
	copy(data[32:36], pad(revision, 4))
	return data
}

// BuildReadCapacity16Data builds a READ CAPACITY(16) response.
func BuildReadCapacity16Data(lastLBA uint64, blockSize uint32) []byte {
	data := make([]byte, 32)
	binary.BigEndian.PutUint64(data[0:8], lastLBA)
	binary.BigEndian.PutUint32(data[8:12], blockSize)
	return data
}

// BuildReportLunsData builds a REPORT LUNS response for the given LUNs.
func BuildReportLunsData(luns []uint64) []byte {
	listLen := len(luns) * 8
	data := make([]byte, 8+listLen)
	binary.BigEndian.PutUint32(data[0:4], uint32(listLen))
	for i, lun := range luns {
		binary.BigEndian.PutUint64(data[8+i*8:16+i*8], lun)
	}
	return data
}

// pad right-pads s to the given length with spaces.
func pad(s string, length int) []byte {
	b := make([]byte, length)
	for i := range b {
		b[i] = ' '
	}
	copy(b, s)
	return b
}
