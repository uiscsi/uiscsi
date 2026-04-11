package conformance_test

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/uiscsi/uiscsi"
	"github.com/uiscsi/uiscsi/internal/pdu"
	testutil "github.com/uiscsi/uiscsi/test"
	"github.com/uiscsi/uiscsi/test/pducapture"
)

// TestCmdWindow_ZeroWindow verifies that the initiator blocks new commands
// when the target closes the command window to zero, and resumes when the
// target reopens it via an unsolicited NOP-In.
// Conformance: CMDSEQ-04 (FFP #3.1)
func TestCmdWindow_ZeroWindow(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		if callCount == 0 {
			// Close the window: delta=-1 means MaxCmdSN = ExpCmdSN - 1.
			tgt.Session().SetMaxCmdSNDelta(-1)
			expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Immediate)

			// Send Data-In with status (read response).
			data := make([]byte, 512)
			din := &pdu.DataIn{
				Header: pdu.Header{
					Final:            true,
					InitiatorTaskTag: cmd.InitiatorTaskTag,
					DataSegmentLen:   uint32(len(data)),
				},
				HasStatus: true,
				Status:    0x00,
				StatSN:    tc.NextStatSN(),
				ExpCmdSN:  expCmdSN,
				MaxCmdSN:  maxCmdSN,
				DataSN:    0,
				Data:      data,
			}
			if err := tc.SendPDU(din); err != nil {
				return err
			}

			// After 500ms, reopen the window and send unsolicited NOP-In
			// to deliver the new ExpCmdSN/MaxCmdSN to the initiator.
			go func() {
				time.Sleep(500 * time.Millisecond)
				tgt.Session().SetMaxCmdSNDelta(10)
				currentExp := tgt.Session().ExpCmdSN()
				reopenMax := uint32(int32(currentExp) + 10)
				nopIn := &pdu.NOPIn{
					Header: pdu.Header{
						Final:            true,
						InitiatorTaskTag: 0xFFFFFFFF, // unsolicited
					},
					TargetTransferTag: 0xFFFFFFFF,
					StatSN:            tc.NextStatSN(),
					ExpCmdSN:          currentExp,
					MaxCmdSN:          reopenMax,
				}
				tc.SendPDU(nopIn)
			}()
			return nil
		}

		// callCount >= 1: respond normally with open window.
		tgt.Session().SetMaxCmdSNDelta(10)
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Immediate)
		data := make([]byte, 512)
		din := &pdu.DataIn{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
				DataSegmentLen:   uint32(len(data)),
			},
			HasStatus: true,
			Status:    0x00,
			StatSN:    tc.NextStatSN(),
			ExpCmdSN:  expCmdSN,
			MaxCmdSN:  maxCmdSN,
			DataSN:    0,
			Data:      data,
		}
		return tc.SendPDU(din)
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

	// First ReadBlocks: succeeds, response closes the window.
	if _, err := sess.ReadBlocks(ctx, 0, 0, 1, 512); err != nil {
		t.Fatalf("first ReadBlocks: %v", err)
	}

	// Second ReadBlocks in goroutine -- should block because window is closed.
	done := make(chan error, 1)
	go func() {
		_, err := sess.ReadBlocks(ctx, 0, 1, 1, 512)
		done <- err
	}()

	// Verify the goroutine does NOT return within 300ms (proves blocking).
	select {
	case err := <-done:
		t.Fatalf("second ReadBlocks returned immediately (should block on zero window): err=%v", err)
	case <-time.After(300 * time.Millisecond):
		// Expected: command is blocked waiting for window to reopen.
	}

	// Verify the goroutine DOES complete after NOP-In reopens the window.
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second ReadBlocks failed after window reopen: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second ReadBlocks did not complete after window reopen (5s timeout)")
	}

	// Verify pducapture shows exactly 2 SCSI Command PDUs with increasing CmdSN.
	cmds := rec.Sent(pdu.OpSCSICommand)
	if len(cmds) < 2 {
		t.Fatalf("captured SCSI commands: got %d, want >= 2", len(cmds))
	}
	first := cmds[0].Decoded.(*pdu.SCSICommand)
	second := cmds[1].Decoded.(*pdu.SCSICommand)
	if second.CmdSN <= first.CmdSN {
		t.Errorf("CmdSN not increasing: first=%d, second=%d", first.CmdSN, second.CmdSN)
	}
}

// TestCmdWindow_LargeWindow verifies that the initiator sends multiple
// concurrent commands through a large command window (256 slots).
// Conformance: CMDSEQ-05 (FFP #3.2)
func TestCmdWindow_LargeWindow(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		tgt.Session().SetMaxCmdSNDelta(255)
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Immediate)

		// Respond with Data-In carrying 512 bytes.
		data := make([]byte, 512)
		// Encode the LBA from the CDB into the response data for identification.
		if len(cmd.CDB) >= 10 {
			copy(data[:10], cmd.CDB[:10])
		}

		din := &pdu.DataIn{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
				DataSegmentLen:   uint32(len(data)),
			},
			HasStatus: true,
			Status:    0x00,
			StatSN:    tc.NextStatSN(),
			ExpCmdSN:  expCmdSN,
			MaxCmdSN:  maxCmdSN,
			DataSN:    0,
			Data:      data,
		}
		return tc.SendPDU(din)
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

	// Launch 8 goroutines each calling ReadBlocks with different LBAs.
	const numCmds = 8
	var wg sync.WaitGroup
	errs := make([]error, numCmds)
	for i := range numCmds {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = sess.ReadBlocks(ctx, 0, uint64(idx), 1, 512)
		}(i)
	}
	wg.Wait()

	for i, e := range errs {
		if e != nil {
			t.Errorf("ReadBlocks[lba=%d]: %v", i, e)
		}
	}

	// Verify all 8 SCSI Command PDUs were sent with unique, monotonically
	// increasing CmdSN values.
	cmds := rec.Sent(pdu.OpSCSICommand)
	if len(cmds) < numCmds {
		t.Fatalf("captured SCSI commands: got %d, want >= %d", len(cmds), numCmds)
	}

	// Collect all CmdSN values from the captured commands.
	var cmdSNs []uint32
	seen := make(map[uint32]bool)
	for i, c := range cmds[:numCmds] {
		cmd := c.Decoded.(*pdu.SCSICommand)
		if seen[cmd.CmdSN] {
			t.Errorf("duplicate CmdSN=%d at index %d", cmd.CmdSN, i)
		}
		seen[cmd.CmdSN] = true
		cmdSNs = append(cmdSNs, cmd.CmdSN)
	}

	// Verify CmdSN values form a contiguous range (allocated sequentially,
	// though wire order may differ due to concurrent dispatch).
	minSN := cmdSNs[0]
	maxSN := cmdSNs[0]
	for _, sn := range cmdSNs[1:] {
		if sn < minSN {
			minSN = sn
		}
		if sn > maxSN {
			maxSN = sn
		}
	}
	if maxSN-minSN != uint32(numCmds-1) {
		t.Errorf("CmdSN range: min=%d, max=%d, span=%d, want %d (contiguous)",
			minSN, maxSN, maxSN-minSN, numCmds-1)
	}
}

// TestCmdWindow_WindowOfOne verifies that the initiator sends only one
// command at a time with a window size of 1 (MaxCmdSN == ExpCmdSN).
// Conformance: CMDSEQ-06 (FFP #3.3)
func TestCmdWindow_WindowOfOne(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		// Window of 1: MaxCmdSN = ExpCmdSN (delta=0).
		tgt.Session().SetMaxCmdSNDelta(0)
		expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Immediate)

		data := make([]byte, 512)
		din := &pdu.DataIn{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: cmd.InitiatorTaskTag,
				DataSegmentLen:   uint32(len(data)),
			},
			HasStatus: true,
			Status:    0x00,
			StatSN:    tc.NextStatSN(),
			ExpCmdSN:  expCmdSN,
			MaxCmdSN:  maxCmdSN,
			DataSN:    0,
			Data:      data,
		}
		return tc.SendPDU(din)
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

	// Issue 3 sequential ReadBlocks. With window of 1, each must wait
	// for the prior response before the next command is sent.
	for i := range 3 {
		if _, err := sess.ReadBlocks(ctx, 0, uint64(i), 1, 512); err != nil {
			t.Fatalf("ReadBlocks[%d]: %v", i, err)
		}
	}

	// Verify 3 SCSI Command PDUs with consecutive CmdSN values.
	cmds := rec.Sent(pdu.OpSCSICommand)
	if len(cmds) < 3 {
		t.Fatalf("captured SCSI commands: got %d, want >= 3", len(cmds))
	}

	for i := 1; i < 3; i++ {
		prev := cmds[i-1].Decoded.(*pdu.SCSICommand)
		curr := cmds[i].Decoded.(*pdu.SCSICommand)
		delta := curr.CmdSN - prev.CmdSN
		if delta != 1 {
			t.Errorf("CmdSN[%d]=%d, CmdSN[%d]=%d: delta=%d, want 1",
				i-1, prev.CmdSN, i, curr.CmdSN, delta)
		}
	}

	// Verify serialization: for each pair, the SCSI Response for command[i]
	// must appear before the SCSI Command for command[i+1] in the capture.
	// This proves only one command was in flight at a time.
	allPDUs := rec.All()

	// Build a list of SCSI Command and Response/DataIn events with their Seq numbers.
	type cmdEvent struct {
		cmdSN uint32
		seq   int
		kind  string // "cmd" or "resp"
	}
	var events []cmdEvent
	for _, p := range allPDUs {
		switch d := p.Decoded.(type) {
		case *pdu.SCSICommand:
			events = append(events, cmdEvent{cmdSN: d.CmdSN, seq: p.Seq, kind: "cmd"})
		case *pdu.DataIn:
			if d.HasStatus {
				// DataIn with status is effectively the response.
				events = append(events, cmdEvent{seq: p.Seq, kind: "resp"})
			}
		case *pdu.SCSIResponse:
			events = append(events, cmdEvent{seq: p.Seq, kind: "resp"})
		}
	}

	// Sort by sequence number.
	sort.Slice(events, func(i, j int) bool { return events[i].seq < events[j].seq })

	// Find the first 3 cmd events and verify each has a resp between it and the next.
	var cmdSeqs []int
	var respSeqs []int
	for _, e := range events {
		if e.kind == "cmd" {
			cmdSeqs = append(cmdSeqs, e.seq)
		} else {
			respSeqs = append(respSeqs, e.seq)
		}
	}

	// For window-of-1: resp[i] should come before cmd[i+1].
	for i := 0; i < len(cmdSeqs)-1 && i < len(respSeqs); i++ {
		if respSeqs[i] > cmdSeqs[i+1] {
			t.Errorf("response[%d] (seq=%d) came after command[%d] (seq=%d): not serialized",
				i, respSeqs[i], i+1, cmdSeqs[i+1])
		}
	}
}

// TestCmdWindow_MaxCmdSNInResponse verifies that a SCSI Response (not NOP-In)
// can close the command window by carrying MaxCmdSN < ExpCmdSN.
// Conformance: CMDSEQ-09 (FFP #16.5)
func TestCmdWindow_MaxCmdSNInResponse(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		switch callCount {
		case 0:
			// First response: open window (delta=10).
			tgt.Session().SetMaxCmdSNDelta(10)
			expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Immediate)
			data := make([]byte, 512)
			din := &pdu.DataIn{
				Header: pdu.Header{
					Final:            true,
					InitiatorTaskTag: cmd.InitiatorTaskTag,
					DataSegmentLen:   uint32(len(data)),
				},
				HasStatus: true,
				Status:    0x00,
				StatSN:    tc.NextStatSN(),
				ExpCmdSN:  expCmdSN,
				MaxCmdSN:  maxCmdSN,
				DataSN:    0,
				Data:      data,
			}
			return tc.SendPDU(din)

		case 1:
			// Second response: close window via SCSI Response (delta=-1).
			tgt.Session().SetMaxCmdSNDelta(-1)
			expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Immediate)
			data := make([]byte, 512)
			din := &pdu.DataIn{
				Header: pdu.Header{
					Final:            true,
					InitiatorTaskTag: cmd.InitiatorTaskTag,
					DataSegmentLen:   uint32(len(data)),
				},
				HasStatus: true,
				Status:    0x00,
				StatSN:    tc.NextStatSN(),
				ExpCmdSN:  expCmdSN,
				MaxCmdSN:  maxCmdSN,
				DataSN:    0,
				Data:      data,
			}
			if err := tc.SendPDU(din); err != nil {
				return err
			}

			// After 500ms, reopen the window via NOP-In.
			go func() {
				time.Sleep(500 * time.Millisecond)
				tgt.Session().SetMaxCmdSNDelta(10)
				currentExp := tgt.Session().ExpCmdSN()
				reopenMax := uint32(int32(currentExp) + 10)
				nopIn := &pdu.NOPIn{
					Header: pdu.Header{
						Final:            true,
						InitiatorTaskTag: 0xFFFFFFFF,
					},
					TargetTransferTag: 0xFFFFFFFF,
					StatSN:            tc.NextStatSN(),
					ExpCmdSN:          currentExp,
					MaxCmdSN:          reopenMax,
				}
				tc.SendPDU(nopIn)
			}()
			return nil

		default:
			// callCount >= 2: respond normally with open window.
			tgt.Session().SetMaxCmdSNDelta(10)
			expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Immediate)
			data := make([]byte, 512)
			din := &pdu.DataIn{
				Header: pdu.Header{
					Final:            true,
					InitiatorTaskTag: cmd.InitiatorTaskTag,
					DataSegmentLen:   uint32(len(data)),
				},
				HasStatus: true,
				Status:    0x00,
				StatSN:    tc.NextStatSN(),
				ExpCmdSN:  expCmdSN,
				MaxCmdSN:  maxCmdSN,
				DataSN:    0,
				Data:      data,
			}
			return tc.SendPDU(din)
		}
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

	// First ReadBlocks: succeeds, window stays open.
	if _, err := sess.ReadBlocks(ctx, 0, 0, 1, 512); err != nil {
		t.Fatalf("first ReadBlocks: %v", err)
	}

	// Second ReadBlocks: succeeds, but response closes the window.
	if _, err := sess.ReadBlocks(ctx, 0, 1, 1, 512); err != nil {
		t.Fatalf("second ReadBlocks: %v", err)
	}

	// Third ReadBlocks in goroutine -- should block because window is closed.
	done := make(chan error, 1)
	go func() {
		_, err := sess.ReadBlocks(ctx, 0, 2, 1, 512)
		done <- err
	}()

	// Verify blocking (300ms).
	select {
	case err := <-done:
		t.Fatalf("third ReadBlocks returned immediately (should block): err=%v", err)
	case <-time.After(300 * time.Millisecond):
		// Expected: blocked on closed window.
	}

	// Verify completion after NOP-In (5s timeout).
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("third ReadBlocks failed after window reopen: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("third ReadBlocks did not complete after window reopen (5s timeout)")
	}

	// Verify 3 SCSI Commands were sent.
	cmds := rec.Sent(pdu.OpSCSICommand)
	if len(cmds) < 3 {
		t.Fatalf("captured SCSI commands: got %d, want >= 3", len(cmds))
	}
}

