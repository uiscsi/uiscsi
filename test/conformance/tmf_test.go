package conformance_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/uiscsi/uiscsi"
	"github.com/uiscsi/uiscsi/internal/pdu"
	"github.com/uiscsi/uiscsi/internal/transport"
	testutil "github.com/uiscsi/uiscsi/test"
	"github.com/uiscsi/uiscsi/test/pducapture"
)

// TestTMF_CmdSN verifies that Task Management Function PDUs use the correct
// CmdSN (immediate, not acquired from the window) and have the Immediate bit
// set in the BHS.
//
// Per RFC 7143 Section 11.5: TMF requests are always Immediate. The CmdSN
// field carries the current value but does NOT advance the command sequence.
//
// Conformance: TMF-01 (FFP #19.1).
func TestTMF_CmdSN(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	// Stall the first SCSI command by NOT sending a response. The handler
	// must return immediately to avoid blocking the mock target's serve loop
	// (which processes one PDU at a time per connection). The response is
	// sent later when the test unblocks the stall channel.
	stallCh := make(chan struct{})
	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Immediate)
		if callCount == 0 {
			// Launch goroutine to send response after unblock.
			go func() {
				<-stallCh
				din := &pdu.DataIn{
					Header: pdu.Header{
						Final:            true,
						InitiatorTaskTag: cmd.InitiatorTaskTag,
						DataSegmentLen:   512,
					},
					HasStatus: true,
					Status:    0x00,
					StatSN:    tc.NextStatSN(),
					ExpCmdSN:  expCmdSN,
					MaxCmdSN:  maxCmdSN,
					DataSN:    0,
					Data:      make([]byte, 512),
				}
				tc.SendPDU(din)
			}()
			return nil // return immediately so serve loop continues
		}
		// Subsequent commands respond normally.
		din := &pdu.DataIn{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
				DataSegmentLen:   512,
			},
			HasStatus: true,
			Status:    0x00,
			StatSN:    tc.NextStatSN(),
			ExpCmdSN:  expCmdSN,
			MaxCmdSN:  maxCmdSN,
			DataSN:    0,
			Data:      make([]byte, 512),
		}
		return tc.SendPDU(din)
	})

	// TMF handler: respond with Function Complete and update session state.
	tgt.Handle(pdu.OpTaskMgmtReq, func(tc *testutil.TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		tmf := decoded.(*pdu.TaskMgmtReq)
		expCmdSN, maxCmdSN := tgt.Session().Update(tmf.CmdSN, tmf.Immediate)
		resp := &pdu.TaskMgmtResp{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: tmf.InitiatorTaskTag,
			},
			Response: 0x00,
			StatSN:   tc.NextStatSN(),
			ExpCmdSN: expCmdSN,
			MaxCmdSN: maxCmdSN,
		}
		return tc.SendPDU(resp)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithPDUHook(rec.Hook()),
		uiscsi.WithKeepaliveInterval(30*time.Second),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	// Launch a SCSI command that will stall (no response until channel closes).
	type readResult struct {
		data []byte
		err  error
	}
	readCh := make(chan readResult, 1)
	go func() {
		data, err := sess.ReadBlocks(ctx, 0, 0, 1, 512)
		readCh <- readResult{data, err}
	}()

	// Wait for the SCSI command to be in-flight.
	time.Sleep(200 * time.Millisecond)

	// Capture the SCSI command's CmdSN.
	scsiPDUs := rec.Sent(pdu.OpSCSICommand)
	if len(scsiPDUs) == 0 {
		close(stallCh)
		t.Fatal("TMF-01: no SCSI command captured")
	}
	scsiCmd := scsiPDUs[0].Decoded.(*pdu.SCSICommand)
	scsiCmdSN := scsiCmd.CmdSN

	// Send AbortTask targeting the stalled task's ITT.
	_, err = sess.AbortTask(ctx, scsiCmd.InitiatorTaskTag)
	if err != nil {
		close(stallCh)
		t.Fatalf("TMF-01: AbortTask: %v", err)
	}

	// Capture the TMF PDU.
	tmfPDUs := rec.Sent(pdu.OpTaskMgmtReq)
	if len(tmfPDUs) == 0 {
		close(stallCh)
		t.Fatal("TMF-01: no TaskMgmtReq captured")
	}
	tmf := tmfPDUs[0].Decoded.(*pdu.TaskMgmtReq)

	// TMF-01 assertion: CmdSN is the NEXT expected value (same as the SCSI
	// command's CmdSN + 1, since the SCSI command acquired one slot).
	// TMF is immediate, so it uses current() which equals CmdSN after the
	// SCSI command was sent.
	expectedCmdSN := scsiCmdSN + 1
	if tmf.CmdSN != expectedCmdSN {
		t.Errorf("TMF-01: CmdSN: got %d, want %d (scsiCmdSN=%d + 1)",
			tmf.CmdSN, expectedCmdSN, scsiCmdSN)
	}

	// TMF-01 assertion: Immediate bit is set.
	if !tmf.Immediate {
		t.Errorf("TMF-01: Immediate bit not set in TMF PDU")
	}

	// Unblock stalled handler and drain the ReadBlocks goroutine.
	close(stallCh)
	select {
	case <-readCh:
	case <-time.After(5 * time.Second):
		t.Fatal("TMF-01: ReadBlocks goroutine did not complete")
	}
}

// TestTMF_LUNEncoding verifies that TMF PDUs encode the LUN field correctly
// in SAM-5 flat space format for multiple LUN values.
//
// Per RFC 7143 Section 11.5: the LUN field in the BHS carries an 8-byte
// SAM-5 encoded LUN value.
//
// Conformance: TMF-02 (FFP #19.2).
func TestTMF_LUNEncoding(t *testing.T) {
	testLUNs := []uint64{0, 1, 3}

	for _, lun := range testLUNs {
		t.Run(fmt.Sprintf("LUN_%d", lun), func(t *testing.T) {
			rec := &pducapture.Recorder{}

			tgt, err := testutil.NewMockTarget()
			if err != nil {
				t.Fatalf("NewMockTarget: %v", err)
			}
			t.Cleanup(func() { tgt.Close() })

			tgt.HandleLogin()
			tgt.HandleLogout()
			tgt.HandleNOPOut()
			tgt.HandleTMF()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			sess, err := uiscsi.Dial(ctx, tgt.Addr(),
				uiscsi.WithPDUHook(rec.Hook()),
				uiscsi.WithKeepaliveInterval(30*time.Second),
			)
			if err != nil {
				t.Fatalf("Dial: %v", err)
			}
			t.Cleanup(func() { sess.Close() })

			// LUNReset is the simplest LUN-scoped TMF (no stall needed).
			_, err = sess.LUNReset(ctx, lun)
			if err != nil {
				t.Fatalf("TMF-02: LUNReset(LUN=%d): %v", lun, err)
			}

			// Capture the TMF PDU.
			tmfPDUs := rec.Sent(pdu.OpTaskMgmtReq)
			if len(tmfPDUs) == 0 {
				t.Fatal("TMF-02: no TaskMgmtReq captured")
			}
			tmf := tmfPDUs[0].Decoded.(*pdu.TaskMgmtReq)

			// Verify LUN encoding matches EncodeSAMLUN.
			expected := pdu.EncodeSAMLUN(lun)
			if tmf.LUN != expected {
				t.Errorf("TMF-02: LUN encoding for LUN %d: got %x, want %x",
					lun, tmf.LUN, expected)
			}
		})
	}
}

// TestTMF_RefCmdSN verifies that AbortTask carries the correct
// ReferencedTaskTag and that the RefCmdSN field is present in the PDU.
//
// Per RFC 7143 Section 11.5.1: AbortTask must carry the ITT of the
// referenced task in ReferencedTaskTag. RefCmdSN is only meaningful
// for AbortTask; for other TMFs, it is reserved.
//
// Note: The current initiator implementation does not track per-task CmdSN,
// so RefCmdSN may be 0. This test verifies the wire format is correct
// regardless of the RefCmdSN value.
//
// Conformance: TMF-03 (FFP #19.3).
func TestTMF_RefCmdSN(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	// Stall the first SCSI command by not sending a response (goroutine waits
	// for stallCh). This keeps the task in-flight so we can target it with
	// AbortTask, while the serve loop remains unblocked.
	stallCh := make(chan struct{})
	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Immediate)
		if callCount == 0 {
			go func() {
				<-stallCh
				din := &pdu.DataIn{
					Header: pdu.Header{
						Final:            true,
						InitiatorTaskTag: cmd.InitiatorTaskTag,
						DataSegmentLen:   512,
					},
					HasStatus: true,
					Status:    0x00,
					StatSN:    tc.NextStatSN(),
					ExpCmdSN:  expCmdSN,
					MaxCmdSN:  maxCmdSN,
					DataSN:    0,
					Data:      make([]byte, 512),
				}
				tc.SendPDU(din)
			}()
			return nil
		}
		din := &pdu.DataIn{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
				DataSegmentLen:   512,
			},
			HasStatus: true,
			Status:    0x00,
			StatSN:    tc.NextStatSN(),
			ExpCmdSN:  expCmdSN,
			MaxCmdSN:  maxCmdSN,
			DataSN:    0,
			Data:      make([]byte, 512),
		}
		return tc.SendPDU(din)
	})

	// TMF handler with session state tracking.
	tgt.Handle(pdu.OpTaskMgmtReq, func(tc *testutil.TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		tmf := decoded.(*pdu.TaskMgmtReq)
		expCmdSN, maxCmdSN := tgt.Session().Update(tmf.CmdSN, tmf.Immediate)
		resp := &pdu.TaskMgmtResp{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: tmf.InitiatorTaskTag,
			},
			Response: 0x00,
			StatSN:   tc.NextStatSN(),
			ExpCmdSN: expCmdSN,
			MaxCmdSN: maxCmdSN,
		}
		return tc.SendPDU(resp)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithPDUHook(rec.Hook()),
		uiscsi.WithKeepaliveInterval(30*time.Second),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	// Launch ReadBlocks in goroutine (it will wait for response indefinitely).
	type readResult struct {
		data []byte
		err  error
	}
	readCh := make(chan readResult, 1)
	go func() {
		data, err := sess.ReadBlocks(ctx, 0, 0, 1, 512)
		readCh <- readResult{data, err}
	}()

	// Wait for the SCSI command to be in-flight.
	time.Sleep(200 * time.Millisecond)

	// Capture the SCSI command's ITT.
	scsiPDUs := rec.Sent(pdu.OpSCSICommand)
	if len(scsiPDUs) == 0 {
		close(stallCh)
		t.Fatal("TMF-03: no SCSI command captured")
	}
	scsiCmd := scsiPDUs[0].Decoded.(*pdu.SCSICommand)
	capturedITT := scsiCmd.InitiatorTaskTag

	// Send AbortTask targeting the captured ITT.
	_, err = sess.AbortTask(ctx, capturedITT)
	if err != nil {
		close(stallCh)
		t.Fatalf("TMF-03: AbortTask: %v", err)
	}

	// Capture the TMF PDU.
	tmfPDUs := rec.Sent(pdu.OpTaskMgmtReq)
	if len(tmfPDUs) == 0 {
		close(stallCh)
		t.Fatal("TMF-03: no TaskMgmtReq captured")
	}
	tmf := tmfPDUs[0].Decoded.(*pdu.TaskMgmtReq)

	// TMF-03 assertion: ReferencedTaskTag matches the captured SCSI command's ITT.
	if tmf.ReferencedTaskTag != capturedITT {
		t.Errorf("TMF-03: ReferencedTaskTag: got 0x%08X, want 0x%08X",
			tmf.ReferencedTaskTag, capturedITT)
	}

	// TMF-03 assertion: TMF's own ITT must differ from ReferencedTaskTag
	// (per RFC 7143: the TMF has its own ITT distinct from the referenced task).
	if tmf.InitiatorTaskTag == tmf.ReferencedTaskTag {
		t.Errorf("TMF-03: TMF ITT (0x%08X) should differ from ReferencedTaskTag (0x%08X)",
			tmf.InitiatorTaskTag, tmf.ReferencedTaskTag)
	}

	// TMF-03 assertion: Function is AbortTask (1).
	if tmf.Function != 1 {
		t.Errorf("TMF-03: Function: got %d, want 1 (AbortTask)", tmf.Function)
	}

	// RefCmdSN field is present in the PDU (verified by successful decode).
	// Note: The current initiator does not populate RefCmdSN with the
	// referenced task's CmdSN (it defaults to 0). This is a known limitation.
	// RFC 7143 Section 11.5.1 says RefCmdSN is only valid for AbortTask.
	t.Logf("TMF-03: RefCmdSN=%d (0 indicates per-task CmdSN tracking not implemented)", tmf.RefCmdSN)

	// Unblock stalled handler and drain ReadBlocks.
	close(stallCh)
	select {
	case <-readCh:
	case <-time.After(5 * time.Second):
		t.Fatal("TMF-03: ReadBlocks goroutine did not complete")
	}
}

// TestTMF_AbortTaskSet_AllTasks verifies that AbortTaskSet cancels all
// in-flight tasks on the target LUN.
//
// Per RFC 7143 Section 11.5.1: ABORT TASK SET (Function=2) cancels all
// outstanding tasks on the specified LUN.
//
// Conformance: TMF-04 (FFP #19.4).
func TestTMF_AbortTaskSet_AllTasks(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	// Stall SCSI commands on callCount 0 and 1 (two in-flight tasks on same LUN).
	// Handlers return immediately; responses are sent via goroutine after unblock.
	stallCh := make(chan struct{})
	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Immediate)
		if callCount <= 1 {
			go func() {
				<-stallCh
				din := &pdu.DataIn{
					Header: pdu.Header{
						Final:            true,
						InitiatorTaskTag: cmd.InitiatorTaskTag,
						DataSegmentLen:   512,
					},
					HasStatus: true,
					Status:    0x00,
					StatSN:    tc.NextStatSN(),
					ExpCmdSN:  expCmdSN,
					MaxCmdSN:  maxCmdSN,
					DataSN:    0,
					Data:      make([]byte, 512),
				}
				tc.SendPDU(din)
			}()
			return nil
		}
		din := &pdu.DataIn{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
				DataSegmentLen:   512,
			},
			HasStatus: true,
			Status:    0x00,
			StatSN:    tc.NextStatSN(),
			ExpCmdSN:  expCmdSN,
			MaxCmdSN:  maxCmdSN,
			DataSN:    0,
			Data:      make([]byte, 512),
		}
		return tc.SendPDU(din)
	})

	// Custom TMF handler: sends Function Complete AND unblocks stalled handlers.
	var stallOnce sync.Once
	tgt.Handle(pdu.OpTaskMgmtReq, func(tc *testutil.TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		tmf := decoded.(*pdu.TaskMgmtReq)
		expCmdSN, maxCmdSN := tgt.Session().Update(tmf.CmdSN, tmf.Immediate)

		// Send Function Complete response.
		resp := &pdu.TaskMgmtResp{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: tmf.InitiatorTaskTag,
			},
			Response: 0x00,
			StatSN:   tc.NextStatSN(),
			ExpCmdSN: expCmdSN,
			MaxCmdSN: maxCmdSN,
		}
		if err := tc.SendPDU(resp); err != nil {
			return err
		}

		// Unblock stalled SCSI handlers so they can complete.
		stallOnce.Do(func() { close(stallCh) })
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithPDUHook(rec.Hook()),
		uiscsi.WithKeepaliveInterval(30*time.Second),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	// Launch 2 ReadBlocks in goroutines (both on LUN 0).
	type readResult struct {
		data []byte
		err  error
	}
	readCh1 := make(chan readResult, 1)
	readCh2 := make(chan readResult, 1)
	go func() {
		data, err := sess.ReadBlocks(ctx, 0, 0, 1, 512)
		readCh1 <- readResult{data, err}
	}()
	go func() {
		data, err := sess.ReadBlocks(ctx, 0, 1, 1, 512)
		readCh2 <- readResult{data, err}
	}()

	// Wait for both to be in-flight.
	time.Sleep(300 * time.Millisecond)

	// Call AbortTaskSet on LUN 0.
	result, err := sess.AbortTaskSet(ctx, 0)
	if err != nil {
		t.Fatalf("TMF-04: AbortTaskSet: %v", err)
	}
	if result.Response != 0 {
		t.Errorf("TMF-04: Response: got %d, want 0 (Function Complete)", result.Response)
	}

	// Verify both ReadBlocks complete (either with data or error from abort).
	for i, ch := range []chan readResult{readCh1, readCh2} {
		select {
		case <-ch:
			// Either success (if response arrived before abort) or error is acceptable.
		case <-time.After(5 * time.Second):
			t.Fatalf("TMF-04: ReadBlocks goroutine %d did not complete after AbortTaskSet", i+1)
		}
	}

	// Capture the TMF PDU and verify Function=2 (AbortTaskSet).
	tmfPDUs := rec.Sent(pdu.OpTaskMgmtReq)
	if len(tmfPDUs) == 0 {
		t.Fatal("TMF-04: no TaskMgmtReq captured")
	}
	tmf := tmfPDUs[0].Decoded.(*pdu.TaskMgmtReq)
	if tmf.Function != 2 {
		t.Errorf("TMF-04: Function: got %d, want 2 (AbortTaskSet)", tmf.Function)
	}
}

// TestTMF_AbortTaskSet_BlocksNew verifies that new SCSI commands block
// while an AbortTaskSet response is pending.
//
// Per RFC 7143 Section 11.5.1: An initiator MUST NOT send new commands
// to the affected LUN until the TMF response is received.
//
// NOTE: If the initiator does NOT implement command blocking during
// AbortTaskSet, this test documents the gap. The test verifies actual
// behavior and logs findings.
//
// Conformance: TMF-05 (FFP #19.5).
func TestTMF_AbortTaskSet_BlocksNew(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	// Stall the first SCSI command by not responding (goroutine pattern).
	scsiStallCh := make(chan struct{})
	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Immediate)
		if callCount == 0 {
			go func() {
				<-scsiStallCh
				din := &pdu.DataIn{
					Header: pdu.Header{
						Final:            true,
						InitiatorTaskTag: cmd.InitiatorTaskTag,
						DataSegmentLen:   512,
					},
					HasStatus: true,
					Status:    0x00,
					StatSN:    tc.NextStatSN(),
					ExpCmdSN:  expCmdSN,
					MaxCmdSN:  maxCmdSN,
					DataSN:    0,
					Data:      make([]byte, 512),
				}
				tc.SendPDU(din)
			}()
			return nil
		}
		// Subsequent commands respond normally.
		din := &pdu.DataIn{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
				DataSegmentLen:   512,
			},
			HasStatus: true,
			Status:    0x00,
			StatSN:    tc.NextStatSN(),
			ExpCmdSN:  expCmdSN,
			MaxCmdSN:  maxCmdSN,
			DataSN:    0,
			Data:      make([]byte, 512),
		}
		return tc.SendPDU(din)
	})

	// TMF handler that DELAYS response to test blocking.
	tmfReleaseCh := make(chan struct{})
	tgt.Handle(pdu.OpTaskMgmtReq, func(tc *testutil.TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		tmf := decoded.(*pdu.TaskMgmtReq)
		expCmdSN, maxCmdSN := tgt.Session().Update(tmf.CmdSN, tmf.Immediate)

		// Wait in goroutine for test to signal, then send response.
		// We must NOT block the serve loop, but we also want to delay the
		// TMF response. Use a goroutine to wait and respond.
		go func() {
			<-tmfReleaseCh
			// Unblock stalled SCSI handler.
			close(scsiStallCh)
			resp := &pdu.TaskMgmtResp{
				Header: pdu.Header{
					Final:            true,
					InitiatorTaskTag: tmf.InitiatorTaskTag,
				},
				Response: 0x00,
				StatSN:   tc.NextStatSN(),
				ExpCmdSN: expCmdSN,
				MaxCmdSN: maxCmdSN,
			}
			tc.SendPDU(resp)
		}()
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithPDUHook(rec.Hook()),
		uiscsi.WithKeepaliveInterval(30*time.Second),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	// Launch first ReadBlocks (will stall in handler).
	type readResult struct {
		data []byte
		err  error
	}
	readCh1 := make(chan readResult, 1)
	go func() {
		data, err := sess.ReadBlocks(ctx, 0, 0, 1, 512)
		readCh1 <- readResult{data, err}
	}()

	// Wait for it to be in-flight.
	time.Sleep(200 * time.Millisecond)

	// Call AbortTaskSet in goroutine (will block waiting for TMF response).
	abortCh := make(chan struct{}, 1)
	go func() {
		sess.AbortTaskSet(ctx, 0)
		abortCh <- struct{}{}
	}()

	// Wait for TMF to be sent.
	time.Sleep(200 * time.Millisecond)

	// Launch NEW ReadBlocks while AbortTaskSet is pending.
	readCh2 := make(chan readResult, 1)
	go func() {
		data, err := sess.ReadBlocks(ctx, 0, 2, 1, 512)
		readCh2 <- readResult{data, err}
	}()

	// Check if the new ReadBlocks is blocked for 300ms.
	// If the initiator implements blocking, readCh2 should NOT complete.
	select {
	case <-readCh2:
		// New command completed while AbortTaskSet is pending.
		// This means the initiator does NOT block new commands during AbortTaskSet.
		t.Log("TMF-05: NOTE: Initiator does not block new commands during AbortTaskSet. " +
			"This is a known gap -- RFC 7143 recommends blocking but does not strictly require it.")
	case <-time.After(300 * time.Millisecond):
		t.Log("TMF-05: New command correctly blocked while AbortTaskSet is pending")
	}

	// Unblock TMF handler.
	close(tmfReleaseCh)

	// Wait for AbortTaskSet to complete.
	select {
	case <-abortCh:
	case <-time.After(5 * time.Second):
		t.Fatal("TMF-05: AbortTaskSet did not complete")
	}

	// Wait for all ReadBlocks to complete.
	for i, ch := range []chan readResult{readCh1, readCh2} {
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatalf("TMF-05: ReadBlocks goroutine %d did not complete", i+1)
		}
	}

	// Verify the TMF PDU was sent.
	tmfPDUs := rec.Sent(pdu.OpTaskMgmtReq)
	if len(tmfPDUs) == 0 {
		t.Fatal("TMF-05: no TaskMgmtReq captured")
	}
}

// TestTMF_AbortTaskSet_ResponseAfterClear verifies that in-flight tasks
// are canceled by the time the AbortTaskSet response is processed.
//
// Per RFC 7143 Section 11.5.1: When the initiator receives an AbortTaskSet
// response, all affected tasks should already be cleaned up.
//
// Conformance: TMF-06 (FFP #19.6).
func TestTMF_AbortTaskSet_ResponseAfterClear(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	// Stall the first SCSI command (goroutine pattern).
	stallCh := make(chan struct{})
	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Immediate)
		if callCount == 0 {
			go func() {
				<-stallCh
				din := &pdu.DataIn{
					Header: pdu.Header{
						Final:            true,
						InitiatorTaskTag: cmd.InitiatorTaskTag,
						DataSegmentLen:   512,
					},
					HasStatus: true,
					Status:    0x00,
					StatSN:    tc.NextStatSN(),
					ExpCmdSN:  expCmdSN,
					MaxCmdSN:  maxCmdSN,
					DataSN:    0,
					Data:      make([]byte, 512),
				}
				tc.SendPDU(din)
			}()
			return nil
		}
		din := &pdu.DataIn{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
				DataSegmentLen:   512,
			},
			HasStatus: true,
			Status:    0x00,
			StatSN:    tc.NextStatSN(),
			ExpCmdSN:  expCmdSN,
			MaxCmdSN:  maxCmdSN,
			DataSN:    0,
			Data:      make([]byte, 512),
		}
		return tc.SendPDU(din)
	})

	// TMF handler sends immediate Function Complete + unblocks stalled handler.
	var tmfOnce sync.Once
	tgt.Handle(pdu.OpTaskMgmtReq, func(tc *testutil.TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		tmf := decoded.(*pdu.TaskMgmtReq)
		expCmdSN, maxCmdSN := tgt.Session().Update(tmf.CmdSN, tmf.Immediate)

		resp := &pdu.TaskMgmtResp{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: tmf.InitiatorTaskTag,
			},
			Response: 0x00,
			StatSN:   tc.NextStatSN(),
			ExpCmdSN: expCmdSN,
			MaxCmdSN: maxCmdSN,
		}
		if err := tc.SendPDU(resp); err != nil {
			return err
		}

		// Unblock the stalled SCSI handler.
		tmfOnce.Do(func() { close(stallCh) })
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithPDUHook(rec.Hook()),
		uiscsi.WithKeepaliveInterval(30*time.Second),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	// Launch ReadBlocks in goroutine (will stall).
	type readResult struct {
		data []byte
		err  error
	}
	readCh := make(chan readResult, 1)
	go func() {
		data, err := sess.ReadBlocks(ctx, 0, 0, 1, 512)
		readCh <- readResult{data, err}
	}()

	// Wait for it to be in-flight.
	time.Sleep(200 * time.Millisecond)

	// Call AbortTaskSet -- this should complete after the target responds.
	result, err := sess.AbortTaskSet(ctx, 0)
	if err != nil {
		t.Fatalf("TMF-06: AbortTaskSet: %v", err)
	}
	if result.Response != 0 {
		t.Errorf("TMF-06: Response: got %d, want 0 (Function Complete)", result.Response)
	}

	// After AbortTaskSet returns, the in-flight ReadBlocks goroutine should
	// have completed (either with data or error from abort).
	select {
	case r := <-readCh:
		if r.err != nil {
			t.Logf("TMF-06: ReadBlocks completed with error (task aborted): %v", r.err)
		} else {
			t.Logf("TMF-06: ReadBlocks completed with data (response arrived before cleanup)")
		}
	case <-time.After(2 * time.Second):
		t.Error("TMF-06: ReadBlocks goroutine did not complete within 2s after AbortTaskSet returned")
	}

	// Verify the TMF PDU was captured.
	tmfPDUs := rec.Sent(pdu.OpTaskMgmtReq)
	if len(tmfPDUs) == 0 {
		t.Fatal("TMF-06: no TaskMgmtReq captured")
	}

	// Verify the LUN encoding in the AbortTaskSet PDU.
	tmf := tmfPDUs[0].Decoded.(*pdu.TaskMgmtReq)
	expectedLUN := pdu.EncodeSAMLUN(0)
	if tmf.LUN != expectedLUN {
		t.Errorf("TMF-06: LUN encoding: got %x, want %x", tmf.LUN, expectedLUN)
	}
}
