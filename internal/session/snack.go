package session

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/uiscsi/uiscsi/internal/pdu"
	"github.com/uiscsi/uiscsi/internal/transport"
)

// snackState tracks SNACK recovery state for a task.
type snackState struct {
	pendingDataIn map[uint32]*pdu.DataIn // buffered out-of-order PDUs by DataSN
	gapDetected   bool
	timer         *time.Timer // per-task SNACK timeout for tail loss (D-06)
}

func newSnackState() *snackState {
	return &snackState{
		pendingDataIn: make(map[uint32]*pdu.DataIn),
	}
}

// sendSNACK builds and sends a SNACK Request for missing Data-In PDUs.
// Per RFC 7143 Section 11.16, the SNACK carries the task's ITT (Pitfall 4).
// Per D-05, D-07.
func (t *task) sendSNACK(getWriteCh func() chan<- *transport.RawPDU, snackType uint8, begRun, runLength uint32, expStatSN uint32) error {
	snack := &pdu.SNACKReq{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: t.itt, // Task's own ITT, NOT a new one (Pitfall 4)
		},
		Type:      snackType,
		BegRun:    begRun,
		RunLength: runLength,
		ExpStatSN: expStatSN,
	}

	bhs, err := snack.MarshalBHS()
	if err != nil {
		return fmt.Errorf("session: encode SNACK: %w", err)
	}

	raw := &transport.RawPDU{BHS: bhs}

	// Use blocking send with 5-second timeout per RFC 7143 Section 11.16
	// which requires reliable SNACK transmission.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	writeCh := getWriteCh()
	select {
	case writeCh <- raw:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("session: SNACK send timed out after 5s (writeCh blocked)")
	}
}

// drainPendingDataIn processes buffered out-of-order PDUs after a gap fill.
// It iterates from nextDataSN forward, processing any PDUs that were buffered
// during gap detection. If a buffered PDU has HasStatus=true, the Result is
// delivered and the task completes.
func (t *task) drainPendingDataIn() {
	if t.snack == nil {
		return
	}
	for {
		din, ok := t.snack.pendingDataIn[t.nextDataSN]
		if !ok {
			break
		}
		delete(t.snack.pendingDataIn, din.DataSN)
		// Reassemble normally.
		if len(din.Data) > 0 {
			if t.streaming {
				t.dataReader.ch <- din.Data
			} else {
				t.buf.Write(din.Data)
			}
		}
		t.nextDataSN++
		t.nextOffset += uint32(len(din.Data))
		if din.HasStatus {
			t.stopSnackTimer()
			if t.streaming {
				t.dataReader.close()
				t.resultCh <- Result{
					Status:        din.Status,
					Overflow:      din.ResidualOverflow,
					Underflow:     din.ResidualUnderflow,
					ResidualCount: din.ResidualCount,
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
			return
		}
	}
}

// startSnackTimer starts or resets the per-task SNACK timeout.
// When the timer fires, it sends a Status SNACK (SNACKTypeStatus) to
// request the missing Status/final Data-In PDU -- this handles the
// tail loss case where the last Data-In PDUs are dropped and no
// further PDUs arrive to trigger gap detection.
func (t *task) startSnackTimer(timeout time.Duration, getWriteCh func() chan<- *transport.RawPDU, expStatSNFunc func() uint32) {
	if t.snack == nil {
		t.snack = newSnackState()
	}
	// Stop existing timer if running.
	if t.snack.timer != nil {
		t.snack.timer.Stop()
	}
	t.snack.timer = time.AfterFunc(timeout, func() {
		// Tail loss detected: no Data-In arrived within snackTimeout.
		// Send Status SNACK to request the missing final status.
		expStatSN := expStatSNFunc()
		_ = t.sendSNACK(getWriteCh, SNACKTypeStatus, 0, 0, expStatSN)
	})
}

// resetSnackTimer resets the per-task SNACK timeout. Called on each
// received Data-In PDU to push the timeout window forward.
func (t *task) resetSnackTimer(timeout time.Duration, getWriteCh func() chan<- *transport.RawPDU, expStatSNFunc func() uint32) {
	if t.snack != nil && t.snack.timer != nil {
		t.snack.timer.Stop()
	}
	t.startSnackTimer(timeout, getWriteCh, expStatSNFunc)
}

// stopSnackTimer stops the per-task SNACK timeout. Called when the
// task completes (receives final Status or is cleaned up).
func (t *task) stopSnackTimer() {
	if t.snack != nil && t.snack.timer != nil {
		t.snack.timer.Stop()
		t.snack.timer = nil
	}
}
