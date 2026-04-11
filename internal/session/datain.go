package session

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/uiscsi/uiscsi/internal/pdu"
	"github.com/uiscsi/uiscsi/internal/transport"
)

// task represents an in-flight SCSI command. It correlates request ITT
// with response PDUs and, for read commands, reassembles Data-In PDUs
// into a buffered reader delivered via Result.
type task struct {
	itt        uint32
	lun        uint64        // stored for TMF LUN-based cleanup (AbortTaskSet, LUNReset, ClearTaskSet)
	cmd        Command       // stored for retry during ERL 0 recovery
	cmdSN      uint32        // stored for same-connection retry at ERL >= 1 (RFC 7143 Section 6.2.1)
	buf        *bytes.Buffer // accumulates Data-In payload for read commands
	resultCh   chan Result
	done       chan struct{} // closed by cancel() to unblock taskLoop goroutine on session Close
	cancelOnce sync.Once    // ensures done is closed exactly once
	nextDataSN uint32
	nextOffset uint32
	isRead     bool
	isWrite    bool
	reader     io.Reader    // holds cmd.Data for write tasks; exclusively owned by task goroutine after Submit reads immediate data
	bytesSent  uint32       // cumulative bytes sent: immediate + unsolicited, used for offset tracking
	startTime  time.Time    // when the task was created, for latency metrics
	streaming  bool         // true = chanReader mode (bounded memory), false = bytes.Buffer mode
	dataReader *chanReader  // non-nil when streaming=true; delivers Data-In chunks to caller

	// ERL 1 SNACK recovery fields.
	erl           uint32                              // ErrorRecoveryLevel from negotiated params
	getWriteCh    func() chan<- *transport.RawPDU      // returns current write channel (survives reconnect)
	expStatSNFunc func() uint32                       // returns current ExpStatSN
	snackTimeout  time.Duration                       // per-task SNACK timeout for tail loss detection
	snack         *snackState                         // SNACK recovery state (nil until gap detected or timer started)
}

// newTask creates a task for the given ITT. If isRead is true, a data
// accumulation mechanism is allocated: bytes.Buffer for normal mode, or
// chanReader for streaming mode (bounded memory). If isWrite is true,
// no read buffer is allocated.
func newTask(itt uint32, isRead bool, isWrite bool, streamBufDepth int) *task {
	t := &task{
		itt:       itt,
		resultCh:  make(chan Result, 1),
		done:      make(chan struct{}),
		isRead:    isRead,
		isWrite:   isWrite,
		startTime: time.Now(),
	}
	if isRead {
		if streamBufDepth != 0 {
			t.streaming = true
			t.dataReader = newChanReader(streamBufDepth)
		} else {
			t.buf = &bytes.Buffer{}
		}
	}
	return t
}

// handleDataIn processes a Data-In PDU for this task. It validates DataSN
// and BufferOffset for in-order delivery, appends data to the buffer, and
// delivers a Result if the S-bit indicates status is present.
func (t *task) handleDataIn(din *pdu.DataIn) {
	// Reset per-task SNACK timeout on every received Data-In (D-06 tail loss safety net).
	if t.erl >= 1 && t.snackTimeout > 0 {
		t.resetSnackTimer(t.snackTimeout, t.getWriteCh, t.expStatSNFunc)
	}

	if din.DataSN != t.nextDataSN {
		if t.erl >= 1 {
			// ERL 1: SNACK recovery (D-05, D-07).
			gap := din.DataSN - t.nextDataSN
			if t.snack == nil {
				t.snack = newSnackState()
			}
			t.snack.gapDetected = true
			t.snack.pendingDataIn[din.DataSN] = din

			// Send SNACK for the gap.
			expStatSN := t.expStatSNFunc()
			if err := t.sendSNACK(t.getWriteCh, SNACKTypeDataR2T, t.nextDataSN, gap, expStatSN); err != nil {
				snackErr := fmt.Errorf("session: SNACK send failed: %w", err)
				if t.streaming && t.dataReader != nil {
					t.dataReader.closeWithError(snackErr)
				}
				t.resultCh <- Result{Err: snackErr}
			}
			return
		}

		// ERL 0: fatal gap (existing behavior).
		err := fmt.Errorf("session: DataSN gap (itt=0x%08x got=%d want=%d)", t.itt, din.DataSN, t.nextDataSN)
		if t.streaming && t.dataReader != nil {
			t.dataReader.closeWithError(err)
		}
		t.resultCh <- Result{Err: err}
		return
	}

	if din.BufferOffset != t.nextOffset {
		err := fmt.Errorf("session: BufferOffset mismatch (itt=0x%08x got=%d want=%d)", t.itt, din.BufferOffset, t.nextOffset)
		if t.streaming && t.dataReader != nil {
			t.dataReader.closeWithError(err)
		}
		t.resultCh <- Result{Err: err}
		return
	}

	if len(din.Data) > 0 {
		if t.streaming {
			t.dataReader.ch <- din.Data
		} else {
			t.buf.Write(din.Data)
		}
	}

	t.nextDataSN++
	t.nextOffset += uint32(len(din.Data))

	// A-bit: target requests DataACK acknowledgement (RFC 7143 Section 11.7.2).
	// Send a SNACK with Type=DataACK (2) at ERL >= 1, BegRun = next expected DataSN.
	if din.Acknowledge && t.erl >= 1 && t.getWriteCh != nil {
		expStatSN := t.expStatSNFunc()
		snack := &pdu.SNACKReq{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: t.itt,
			},
			Type:              SNACKTypeDataACK,
			TargetTransferTag: din.TargetTransferTag,
			BegRun:            t.nextDataSN, // acknowledge up to (but not including) this DataSN
			ExpStatSN:         expStatSN,
		}
		bhs, err := snack.MarshalBHS()
		if err == nil {
			raw := &transport.RawPDU{BHS: bhs}
			writeCh := t.getWriteCh()
			select {
			case writeCh <- raw:
			default:
			}
		}
	}

	// After processing in-order PDU, drain any buffered out-of-order PDUs.
	if t.snack != nil && len(t.snack.pendingDataIn) > 0 {
		t.drainPendingDataIn()
		// If drainPendingDataIn sent a result (HasStatus), we are done.
		if len(t.resultCh) > 0 {
			return
		}
	}

	if din.HasStatus {
		// S-bit: this Data-In carries status. Deliver result.
		t.stopSnackTimer()
		if t.streaming {
			t.dataReader.close()
			t.resultCh <- Result{
				Status:        din.Status,
				Overflow:      din.ResidualOverflow,
				Underflow:     din.ResidualUnderflow,
				ResidualCount: din.ResidualCount,
				// Data is nil — caller already has the chanReader.
			}
		} else {
			t.resultCh <- Result{
				Status:        din.Status,
				Data:          bytes.NewReader(t.buf.Bytes()),
				Overflow:      din.ResidualOverflow,
				Underflow:     din.ResidualUnderflow,
				ResidualCount: din.ResidualCount,
			}
		}
	}
}

// handleSCSIResponse processes a SCSIResponse PDU for this task.
// It delivers the final Result with buffered data (for reads) or nil data.
func (t *task) handleSCSIResponse(resp *pdu.SCSIResponse) {
	// Extract sense data from SCSI Response data segment.
	// Per RFC 7143 Section 11.4.7.2, the data segment starts with
	// a 2-byte SenseLength field followed by the actual sense data.
	var senseData []byte
	if len(resp.Data) >= 2 {
		senseLen := binary.BigEndian.Uint16(resp.Data[0:2])
		if int(senseLen) <= len(resp.Data)-2 {
			senseData = resp.Data[2 : 2+int(senseLen)]
		}
	}

	r := Result{
		Status:        resp.Status,
		SenseData:     senseData,
		Overflow:      resp.Overflow,
		Underflow:     resp.Underflow,
		ResidualCount: resp.ResidualCount,
	}
	if t.streaming && t.dataReader != nil {
		t.dataReader.close()
		// Data already delivered via chanReader; r.Data stays nil.
	} else if t.isRead && t.buf != nil && t.buf.Len() > 0 {
		r.Data = bytes.NewReader(t.buf.Bytes())
	}
	t.resultCh <- r
}

// cancel aborts this task with an error. For streaming tasks, the
// chanReader is closed with the error so that the caller's Read
// returns it immediately. The done channel is closed to unblock any
// taskLoop goroutine waiting on pduCh, ensuring no goroutine leaks.
func (t *task) cancel(err error) {
	if t.streaming && t.dataReader != nil {
		t.dataReader.closeWithError(err)
	}
	select {
	case t.resultCh <- Result{Err: err}:
	default:
	}
	t.cancelOnce.Do(func() { close(t.done) })
}
