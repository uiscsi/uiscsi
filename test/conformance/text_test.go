package conformance_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/uiscsi/uiscsi"
	"github.com/uiscsi/uiscsi/internal/login"
	"github.com/uiscsi/uiscsi/internal/pdu"
	"github.com/uiscsi/uiscsi/internal/transport"
	testutil "github.com/uiscsi/uiscsi/test"
	"github.com/uiscsi/uiscsi/test/pducapture"
)

// triggerRenegotiationViaAsync injects AsyncMsg code 4 on the Nth SCSI
// command (callCount == triggerOnCall) and polls the recorder until at least
// wantCount TextReqs have been captured or the 5s deadline expires.
// The caller must have registered HandleText, HandleLogin, and HandleNOPOut.
func triggerRenegotiationViaAsync(
	t *testing.T,
	tgt *testutil.MockTarget,
	tc *testutil.TargetConn,
	cmd *pdu.SCSICommand,
	callCount, triggerOnCall int,
) error {
	t.Helper()
	expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Immediate)

	resp := &pdu.SCSIResponse{
		Header: pdu.Header{
			Final:            true,
			InitiatorTaskTag: cmd.InitiatorTaskTag,
		},
		Status:   0x00,
		StatSN:   tc.NextStatSN(),
		ExpCmdSN: expCmdSN,
		MaxCmdSN: maxCmdSN,
	}
	if err := tc.SendPDU(resp); err != nil {
		return err
	}

	if callCount == triggerOnCall {
		// Parameter3=3 means initiator has 3 seconds to renegotiate.
		if err := tgt.SendAsyncMsg(tc, 4, testutil.AsyncParams{Parameter3: 3}); err != nil {
			return err
		}
	}
	return nil
}

// pollTextReqs polls the recorder until at least wantCount TextReqs are
// captured or the 5s deadline expires.
func pollTextReqs(t *testing.T, rec *pducapture.Recorder, wantCount int) []pducapture.CapturedPDU {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		textReqs := rec.Sent(pdu.OpTextReq)
		if len(textReqs) >= wantCount {
			return textReqs
		}
		select {
		case <-deadline:
			t.Fatalf("only captured %d TextReqs, want at least %d", len(textReqs), wantCount)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// pollTextResps polls the recorder until at least wantCount TextResps are
// received or the 5s deadline expires. Used to ensure renegotiation has
// fully completed before triggering the next one.
func pollTextResps(t *testing.T, rec *pducapture.Recorder, wantCount int) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		textResps := rec.Received(pdu.OpTextResp)
		if len(textResps) >= wantCount {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("only received %d TextResps, want at least %d", len(textResps), wantCount)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// TestText_Fields verifies that a Text Request PDU carries the correct
// opcode (0x04), F-bit (Final=true for single exchange), and a data
// segment containing valid key=value pairs with at least one operational
// parameter (MaxRecvDataSegmentLength, MaxBurstLength, FirstBurstLength).
// Conformance: TEXT-01.
func TestText_Fields(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleNOPOut()
	tgt.HandleText()

	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		return triggerRenegotiationViaAsync(t, tgt, tc, cmd, callCount, 0)
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

	// Trigger async injection via SCSI command.
	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady: %v", err)
	}

	textReqs := pollTextReqs(t, rec, 1)
	textReq := textReqs[0].Decoded.(*pdu.TextReq)

	// Opcode must be OpTextReq (0x04).
	if textReq.Opcode() != pdu.OpTextReq {
		t.Errorf("opcode: got 0x%02X, want 0x%02X", textReq.Opcode(), pdu.OpTextReq)
	}

	// F-bit must be true for single-shot renegotiation.
	if !textReq.Final {
		t.Error("Final (F-bit): got false, want true")
	}

	// Data segment must contain valid KV pairs with operational params.
	rawData := textReqs[0].Raw
	if len(rawData) <= pdu.BHSLength {
		t.Fatal("TextReq has no data segment")
	}
	dataSegment := rawData[pdu.BHSLength:]
	kvs := login.DecodeTextKV(dataSegment)
	if len(kvs) == 0 {
		t.Fatal("TextReq data segment has no key-value pairs")
	}

	kvMap := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		kvMap[kv.Key] = kv.Value
	}

	operationalKeys := []string{"MaxRecvDataSegmentLength", "MaxBurstLength", "FirstBurstLength"}
	found := false
	for _, key := range operationalKeys {
		if _, ok := kvMap[key]; ok {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("data segment missing operational parameters; got keys: %s",
			formatKVKeys(kvMap))
	}
}

// TestText_ITTUniqueness verifies that each Text Request uses a unique
// Initiator Task Tag (ITT) across multiple text exchanges. None of the
// ITTs may be the reserved value 0xFFFFFFFF.
// Conformance: TEXT-02.
func TestText_ITTUniqueness(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleNOPOut()
	tgt.HandleText()

	// Inject async code 4 one at a time, only on the call we're ready for.
	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		return triggerRenegotiationViaAsync(t, tgt, tc, cmd, callCount, callCount)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithPDUHook(rec.Hook()),
		uiscsi.WithKeepaliveInterval(30*time.Second),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	// Trigger 3 renegotiations sequentially, waiting for each to fully
	// complete (TextReq sent AND TextResp received) before the next.
	for i := 0; i < 3; i++ {
		if err := sess.TestUnitReady(ctx, 0); err != nil {
			t.Fatalf("TestUnitReady[%d]: %v", i, err)
		}
		// Wait for the TextReq from this renegotiation to appear.
		pollTextReqs(t, rec, i+1)
		// Wait for the TextResp to arrive and renegotiation to complete,
		// preventing concurrent access to session params.
		pollTextResps(t, rec, i+1)
		time.Sleep(200 * time.Millisecond)
	}

	textReqs := rec.Sent(pdu.OpTextReq)
	if len(textReqs) < 3 {
		t.Fatalf("captured %d TextReqs, want at least 3", len(textReqs))
	}

	ittSet := make(map[uint32]bool)
	for i, tr := range textReqs[:3] {
		itt := tr.Decoded.(*pdu.TextReq).InitiatorTaskTag
		if itt == 0xFFFFFFFF {
			t.Errorf("TextReq[%d] ITT is reserved 0xFFFFFFFF", i)
		}
		if ittSet[itt] {
			t.Errorf("TextReq[%d] ITT 0x%08X is a duplicate", i, itt)
		}
		ittSet[itt] = true
	}
}

// TestText_TTTInitial verifies that the initial Text Request uses
// TargetTransferTag = 0xFFFFFFFF, indicating an initiator-initiated
// exchange (not a continuation).
// Conformance: TEXT-03.
func TestText_TTTInitial(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleNOPOut()
	tgt.HandleText()

	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		return triggerRenegotiationViaAsync(t, tgt, tc, cmd, callCount, 0)
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

	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady: %v", err)
	}

	textReqs := pollTextReqs(t, rec, 1)
	textReq := textReqs[0].Decoded.(*pdu.TextReq)

	if textReq.TargetTransferTag != 0xFFFFFFFF {
		t.Errorf("initial TTT: got 0x%08X, want 0xFFFFFFFF", textReq.TargetTransferTag)
	}
}

// TestText_TTTContinuation verifies that when a Text Response has
// Continue=true and a non-0xFFFFFFFF TTT, the initiator echoes that
// TTT in the continuation Text Request. Uses SendTargets (via Discover)
// because renegotiate() does not handle C-bit continuation.
// Conformance: TEXT-04.
func TestText_TTTContinuation(t *testing.T) {
	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleLogout()
	tgt.HandleNOPOut()

	const continuationTTT uint32 = 0x12345678

	// Track TTT values from incoming TextReqs on the target side.
	type capturedReq struct {
		ITT uint32
		TTT uint32
	}
	var (
		mu       sync.Mutex
		captured []capturedReq
	)

	// Custom text handler: first response has C=1 and TTT=continuationTTT,
	// second response has C=0 and TTT=0xFFFFFFFF with final data.
	tgt.Handle(pdu.OpTextReq, func(tc *testutil.TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
		req := decoded.(*pdu.TextReq)
		expCmdSN, maxCmdSN := tgt.Session().Update(req.CmdSN, req.Immediate)

		mu.Lock()
		captured = append(captured, capturedReq{
			ITT: req.InitiatorTaskTag,
			TTT: req.TargetTransferTag,
		})
		reqNum := len(captured)
		mu.Unlock()

		if reqNum == 1 {
			// First request: respond with partial data, C=1, non-0xFFFFFFFF TTT.
			partialData := login.EncodeTextKV([]login.KeyValue{
				{Key: "TargetName", Value: "iqn.2026-04.com.example:target1"},
			})
			resp := &pdu.TextResp{
				Header: pdu.Header{
					Final:            false,
					InitiatorTaskTag: req.InitiatorTaskTag,
					DataSegmentLen:   uint32(len(partialData)),
				},
				Continue:          true,
				TargetTransferTag: continuationTTT,
				StatSN:            tc.NextStatSN(),
				ExpCmdSN:          expCmdSN,
				MaxCmdSN:          maxCmdSN,
				Data:              partialData,
			}
			return tc.SendPDU(resp)
		}

		// Continuation request: respond with final data, C=0, TTT=0xFFFFFFFF.
		finalData := login.EncodeTextKV([]login.KeyValue{
			{Key: "TargetAddress", Value: "192.168.1.100:3260,1"},
		})
		resp := &pdu.TextResp{
			Header: pdu.Header{
				Final:            true,
				InitiatorTaskTag: req.InitiatorTaskTag,
				DataSegmentLen:   uint32(len(finalData)),
			},
			Continue:          false,
			TargetTransferTag: 0xFFFFFFFF,
			StatSN:            tc.NextStatSN(),
			ExpCmdSN:          expCmdSN,
			MaxCmdSN:          maxCmdSN,
			Data:              finalData,
		}
		return tc.SendPDU(resp)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Use Discover which calls SendTargets internally and handles continuation.
	_, err = uiscsi.Discover(ctx, tgt.Addr())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	// Validate the captured TextReqs from the target side.
	mu.Lock()
	reqs := make([]capturedReq, len(captured))
	copy(reqs, captured)
	mu.Unlock()

	if len(reqs) < 2 {
		t.Fatalf("captured %d TextReqs, want at least 2 (initial + continuation)", len(reqs))
	}

	// First TextReq: TTT must be 0xFFFFFFFF (initiator-initiated).
	if reqs[0].TTT != 0xFFFFFFFF {
		t.Errorf("first TextReq TTT: got 0x%08X, want 0xFFFFFFFF", reqs[0].TTT)
	}

	// Second TextReq: TTT must echo the target's continuationTTT.
	if reqs[1].TTT != continuationTTT {
		t.Errorf("continuation TextReq TTT: got 0x%08X, want 0x%08X", reqs[1].TTT, continuationTTT)
	}
}

// TestText_OtherParams verifies that a Text Request:
//   - Is non-immediate (I-bit clear, CmdSN is acquired)
//   - Has a valid CmdSN (greater than the preceding SCSI command's CmdSN)
//   - Has a valid ExpStatSN
//
// Conformance: TEXT-05.
func TestText_OtherParams(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleNOPOut()
	tgt.HandleText()

	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		return triggerRenegotiationViaAsync(t, tgt, tc, cmd, callCount, 0)
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

	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady: %v", err)
	}

	// Get the SCSI command's CmdSN for comparison.
	scsiCmds := rec.Sent(pdu.OpSCSICommand)
	if len(scsiCmds) == 0 {
		t.Fatal("no SCSI commands captured")
	}
	initialCmdSN := scsiCmds[0].Decoded.(*pdu.SCSICommand).CmdSN

	textReqs := pollTextReqs(t, rec, 1)
	textReq := textReqs[0].Decoded.(*pdu.TextReq)

	// Immediate bit must be false (TextReq acquires CmdSN).
	if textReq.Immediate {
		t.Error("Immediate: got true, want false (TextReq is non-immediate)")
	}

	// CmdSN must be greater than the SCSI command's CmdSN (acquired slot).
	if textReq.CmdSN <= initialCmdSN {
		t.Errorf("CmdSN: got %d, want > %d (initial SCSI CmdSN)", textReq.CmdSN, initialCmdSN)
	}

	// ExpStatSN must be valid (non-zero, reflects received StatSNs).
	if textReq.ExpStatSN == 0 {
		t.Error("ExpStatSN: got 0, want non-zero (should reflect received StatSNs)")
	}
}

// TestText_NegotiationReset verifies that after a completed text exchange,
// a new exchange uses a fresh ITT and TTT=0xFFFFFFFF, proving no stale
// state persists between exchanges.
// Conformance: TEXT-06.
func TestText_NegotiationReset(t *testing.T) {
	rec := &pducapture.Recorder{}

	tgt, err := testutil.NewMockTarget()
	if err != nil {
		t.Fatalf("NewMockTarget: %v", err)
	}
	t.Cleanup(func() { tgt.Close() })

	tgt.HandleLogin()
	tgt.HandleNOPOut()
	tgt.HandleText()

	// Inject async code 4 on each call.
	tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
		return triggerRenegotiationViaAsync(t, tgt, tc, cmd, callCount, callCount)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	sess, err := uiscsi.Dial(ctx, tgt.Addr(),
		uiscsi.WithPDUHook(rec.Hook()),
		uiscsi.WithKeepaliveInterval(30*time.Second),
	)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	// Trigger first renegotiation and wait for full completion.
	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady[0]: %v", err)
	}
	pollTextReqs(t, rec, 1)
	pollTextResps(t, rec, 1)
	time.Sleep(200 * time.Millisecond)

	// Trigger second renegotiation.
	if err := sess.TestUnitReady(ctx, 0); err != nil {
		t.Fatalf("TestUnitReady[1]: %v", err)
	}
	pollTextReqs(t, rec, 2)
	pollTextResps(t, rec, 2)

	textReqs := rec.Sent(pdu.OpTextReq)
	if len(textReqs) < 2 {
		t.Fatalf("captured %d TextReqs, want at least 2", len(textReqs))
	}

	first := textReqs[0].Decoded.(*pdu.TextReq)
	second := textReqs[1].Decoded.(*pdu.TextReq)

	// Fresh ITT: second exchange must use a different ITT.
	if first.InitiatorTaskTag == second.InitiatorTaskTag {
		t.Errorf("second TextReq has same ITT (0x%08X) as first; want fresh ITT",
			first.InitiatorTaskTag)
	}

	// Fresh exchange: TTT must be 0xFFFFFFFF (not stale from prior exchange).
	if second.TargetTransferTag != 0xFFFFFFFF {
		t.Errorf("second TextReq TTT: got 0x%08X, want 0xFFFFFFFF", second.TargetTransferTag)
	}
}

// formatKVKeys returns a comma-separated list of map keys for diagnostic output.
func formatKVKeys(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}
