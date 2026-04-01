package session

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/rkujawa/uiscsi/internal/pdu"
	"github.com/rkujawa/uiscsi/internal/transport"
)

// task represents an in-flight SCSI command. It correlates request ITT
// with response PDUs and, for read commands, reassembles Data-In PDUs
// into a buffered reader delivered via Result.
type task struct {
	itt        uint32
	lun        uint64        // stored for TMF LUN-based cleanup (AbortTaskSet, LUNReset, ClearTaskSet)
	cmd        Command       // stored for retry during ERL 0 recovery
	buf        *bytes.Buffer // accumulates Data-In payload for read commands
	resultCh   chan Result
	nextDataSN uint32
	nextOffset uint32
	isRead     bool
	isWrite    bool
	reader     io.Reader  // holds cmd.Data for write tasks; exclusively owned by task goroutine after Submit reads immediate data
	bytesSent  uint32     // cumulative bytes sent: immediate + unsolicited, used for offset tracking
	startTime  time.Time  // when the task was created, for latency metrics

	// ERL 1 SNACK recovery fields.
	erl           uint32                   // ErrorRecoveryLevel from negotiated params
	writeCh       chan<- *transport.RawPDU  // session write channel for sending SNACKs
	expStatSNFunc func() uint32            // returns current ExpStatSN
	snackTimeout  time.Duration            // per-task SNACK timeout for tail loss detection
	snack         *snackState              // SNACK recovery state (nil until gap detected or timer started)
}

// newTask creates a task for the given ITT. If isRead is true, a buffer
// is allocated for Data-In reassembly. If isWrite is true, no buffer is
// allocated (writes don't accumulate Data-In).
func newTask(itt uint32, isRead bool, isWrite bool) *task {
	t := &task{
		itt:       itt,
		resultCh:  make(chan Result, 1),
		isRead:    isRead,
		isWrite:   isWrite,
		startTime: time.Now(),
	}
	if isRead {
		t.buf = &bytes.Buffer{}
	}
	return t
}

// handleDataIn processes a Data-In PDU for this task. It validates DataSN
// and BufferOffset for in-order delivery, appends data to the buffer, and
// delivers a Result if the S-bit indicates status is present.
func (t *task) handleDataIn(din *pdu.DataIn) {
	// Reset per-task SNACK timeout on every received Data-In (D-06 tail loss safety net).
	if t.erl >= 1 && t.snackTimeout > 0 {
		t.resetSnackTimer(t.snackTimeout, t.writeCh, t.expStatSNFunc)
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
			if err := t.sendSNACK(t.writeCh, SNACKTypeDataR2T, t.nextDataSN, gap, expStatSN); err != nil {
				t.resultCh <- Result{Err: fmt.Errorf("session: SNACK send failed: %w", err)}
			}
			return
		}

		// ERL 0: fatal gap (existing behavior).
		err := fmt.Errorf("session: DataSN gap (itt=0x%08x got=%d want=%d)", t.itt, din.DataSN, t.nextDataSN)
		t.resultCh <- Result{Err: err}
		return
	}

	if din.BufferOffset != t.nextOffset {
		err := fmt.Errorf("session: BufferOffset mismatch (itt=0x%08x got=%d want=%d)", t.itt, din.BufferOffset, t.nextOffset)
		t.resultCh <- Result{Err: err}
		return
	}

	if len(din.Data) > 0 {
		t.buf.Write(din.Data)
	}

	t.nextDataSN++
	t.nextOffset += uint32(len(din.Data))

	// After processing in-order PDU, drain any buffered out-of-order PDUs.
	if t.snack != nil && len(t.snack.pendingDataIn) > 0 {
		t.drainPendingDataIn()
		// If drainPendingDataIn sent a result (HasStatus), we are done.
		if len(t.resultCh) > 0 {
			return
		}
	}

	if din.HasStatus {
		// S-bit: this Data-In carries status. Deliver result with buffered data.
		t.stopSnackTimer()
		t.resultCh <- Result{
			Status:        din.Status,
			Data:          bytes.NewReader(t.buf.Bytes()),
			Overflow:      din.ResidualOverflow,
			Underflow:     din.ResidualUnderflow,
			ResidualCount: din.ResidualCount,
		}
	}
}

// handleSCSIResponse processes a SCSIResponse PDU for this task.
// It delivers the final Result with buffered data (for reads) or nil data.
func (t *task) handleSCSIResponse(resp *pdu.SCSIResponse) {
	r := Result{
		Status:        resp.Status,
		SenseData:     resp.Data,
		Overflow:      resp.Overflow,
		Underflow:     resp.Underflow,
		ResidualCount: resp.ResidualCount,
	}
	if t.isRead && t.buf.Len() > 0 {
		r.Data = bytes.NewReader(t.buf.Bytes())
	}
	t.resultCh <- r
}

// cancel aborts this task with an error.
func (t *task) cancel(err error) {
	select {
	case t.resultCh <- Result{Err: err}:
	default:
	}
}
