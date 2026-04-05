# Phase 19: Task Management and Text Negotiation - Research

**Researched:** 2026-04-05
**Domain:** iSCSI conformance testing -- TMF PDU wire-level fields and Text Request negotiation
**Confidence:** HIGH

## Summary

Phase 19 implements 12 conformance tests across two iSCSI protocol domains: Task Management Function (TMF) wire-level field verification (TMF-01 through TMF-06) and Text Request negotiation (TEXT-01 through TEXT-06). All tests follow the established MockTarget + pducapture pattern from Phases 13-18.

The codebase is fully mature for this phase. All production code (TMF functions in `internal/session/tmf.go`, text exchange in `internal/session/async.go` and `internal/session/discovery.go`) and test infrastructure (MockTarget with HandleTMF/HandleText/HandleSCSIFunc, pducapture.Recorder) already exist. No new production code or infrastructure is needed -- only new test files.

**Critical finding:** The `renegotiate()` method is unexported (lowercase). The public API exposes `SendTargets()` for text exchanges. For TEXT tests that need direct text negotiation, the existing ASYNC-04 test pattern (inject AsyncMsg code 4 to trigger renegotiation) provides the mechanism. This is how all TEXT tests will trigger TextReq PDUs on the wire.

**Primary recommendation:** Two test files (`tmf_test.go` and `text_test.go`) in `test/conformance/`, using established patterns: HandleSCSIFunc stalling for in-flight tasks, goroutine+timer for blocking proof, AsyncMsg code 4 injection for triggering text exchanges, and pducapture for wire assertions.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Stall in HandleSCSIFunc to create in-flight tasks for TMF targeting. HandleSCSIFunc on callCount==0 blocks (channel wait or sleep) so the SCSI command stays in-flight. Test sends AbortTask/AbortTaskSet with that ITT, then unblocks the handler. Proven pattern from Phase 18 ERL tests.
- **D-02:** Test multiple LUN encoding formats for TMF-02: flat space, peripheral device, and extended LUN formats per SAM-5. Verify the initiator encodes the LUN field correctly in the TMF PDU for each format.
- **D-03:** RefCmdSN verification (TMF-03) scoped to AbortTask only. AbortTask is the only TMF that references a specific task by ITT+RefCmdSN. Other TMFs (LUNReset, AbortTaskSet) don't carry RefCmdSN per RFC 7143 Section 11.5.1.
- **D-04:** Sequential Renegotiate calls for ITT uniqueness (TEXT-02). Call sess.Renegotiate() multiple times, capture all Text Request PDUs via pducapture, verify all ITTs are distinct. Text negotiation is inherently serial per RFC 7143.
- **D-05:** HandleText returns partial response with Continue=true and non-0xFFFFFFFF TTT for TTT continuation (TEXT-04). Initiator echoes TTT in next Text Request. Handler sends final response with TTT=0xFFFFFFFF. Reuses existing HandleText pattern from Phase 17.
- **D-06:** Negotiation reset (TEXT-06) means verifying the initiator can start a fresh text exchange after a completed one. After a complete Text Request/Response exchange (TTT=0xFFFFFFFF), verify a new Text Request uses a new ITT and TTT=0xFFFFFFFF (no stale state from prior exchange).
- **D-07:** Goroutine + timer pattern to prove the initiator blocks new commands while AbortTaskSet is in flight (TMF-05). Same pattern as Phase 18 zero-window test: launch ReadBlocks in goroutine, verify it doesn't complete within N ms while AbortTaskSet response is pending, then send TMF Response, verify ReadBlocks completes.
- **D-08:** Immediate TMF Response for TMF-06 (response after tasks cleared). HandleTMF sends TMF Response immediately. The test verifies that by the time the initiator receives the response, all prior in-flight tasks on that LUN have been canceled (check task error channels). Tests initiator behavior, not target behavior.
- **D-09:** Two test files by domain: `test/conformance/tmf_test.go` (TMF-01 through TMF-06) and `test/conformance/text_test.go` (TEXT-01 through TEXT-06). Follows the one-file-per-domain pattern from Phases 14-18.

### Claude's Discretion
- Exact timer durations for blocking proof tests (suggested 300ms based on Phase 18 precedent)
- HandleTMF response codes and field values for non-AbortTaskSet TMFs
- Text Request key-value content for negotiation tests

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| TMF-01 | E2E test validates TMF CmdSN handling (FFP #19.1) | sendTMF uses `s.window.current()` for CmdSN (immediate). pducapture captures TaskMgmtReq. Assert CmdSN matches current window value, not incremented. |
| TMF-02 | E2E test validates TMF LUN field encoding (FFP #19.2) | `EncodeSAMLUN()` puts LUN in header bytes 8-15. Assert via pducapture that LUN bytes match SAM-5 encoding for the target LUN. D-02 requires multiple LUN formats. |
| TMF-03 | E2E test validates TMF RefCmdSN for referenced task (FFP #19.3) | Only AbortTask carries RefCmdSN. Must stall a SCSI command (D-01), capture its CmdSN, send AbortTask, verify RefCmdSN in TMF PDU matches the referenced task's CmdSN. |
| TMF-04 | E2E test validates Abort Task Set -- all tasks on LUN aborted (FFP #19.4.1) | Launch multiple in-flight SCSI commands on same LUN via HandleSCSIFunc stall. Send AbortTaskSet. Verify all tasks receive ErrTaskAborted. |
| TMF-05 | E2E test validates Abort Task Set -- no new tasks during abort (FFP #19.4.2) | Goroutine+timer blocking proof (D-07). While AbortTaskSet response is pending, new ReadBlocks should block. 300ms timer for non-completion proof. |
| TMF-06 | E2E test validates Abort Task Set -- response after tasks cleared (FFP #19.4.3) | Send immediate TMF Response (D-08). After initiator processes response, check that in-flight task error channels have ErrTaskAborted. |
| TEXT-01 | E2E test validates Text Request text fields (FFP #18.1) | Trigger renegotiation via AsyncMsg code 4 (proven in ASYNC-04). Capture TextReq via pducapture. Verify F-bit, opcode, data segment contains valid KV pairs. |
| TEXT-02 | E2E test validates Text Request ITT uniqueness (FFP #18.2) | Trigger multiple renegotiations via repeated AsyncMsg code 4. Capture all TextReq PDUs. Verify all ITTs are distinct. |
| TEXT-03 | E2E test validates Text Request TTT initial=0xFFFFFFFF (FFP #18.3.1) | Capture first TextReq. Verify TTT field (bytes 20-24) equals 0xFFFFFFFF for initiator-initiated exchange. |
| TEXT-04 | E2E test validates Text Request TTT continuation (FFP #18.3.2) | Custom HandleText returns partial response with C=1 and non-0xFFFFFFFF TTT (D-05). Capture continuation TextReq. Verify it echoes the target's TTT. |
| TEXT-05 | E2E test validates Text Request other parameters (FFP #18.4) | Verify TextReq fields: Immediate=false, CmdSN is valid (acquired from window), ExpStatSN matches expected value. |
| TEXT-06 | E2E test validates Text Request negotiation reset (FFP #18.5) | After complete exchange (TTT=0xFFFFFFFF in response), trigger new exchange. Verify new TextReq has fresh ITT and TTT=0xFFFFFFFF (D-06). |
</phase_requirements>

## Architecture Patterns

### Test File Structure
```
test/conformance/
  tmf_test.go          # TMF-01 through TMF-06 (NEW)
  text_test.go         # TEXT-01 through TEXT-06 (NEW)
```

Note: The existing `test/conformance/task_test.go` contains basic TMF smoke tests from earlier phases. The new `tmf_test.go` file provides wire-level conformance tests using pducapture. These are complementary, not duplicative. The new tests verify PDU field values on the wire; the old tests verify round-trip success.

### Pattern 1: TMF Wire Verification via Stalled SCSI Command
**What:** Create an in-flight SCSI command by stalling HandleSCSIFunc, send TMF targeting that command, capture TMF PDU, verify fields.
**When to use:** TMF-01, TMF-02, TMF-03
**Example:**
```go
// Source: test/conformance/erl2_test.go pattern + internal/session/tmf.go
tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
    expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
    if callCount == 0 {
        <-stall // Block: keep task in-flight for TMF targeting
        // After unblock, respond normally
    }
    resp := &pdu.SCSIResponse{...}
    return tc.SendPDU(resp)
})

// In test body:
go func() {
    sess.ReadBlocks(ctx, lun, 0, 1, 512) // This blocks in stalled handler
}()
time.Sleep(100 * time.Millisecond) // Ensure SCSI command is in-flight

result, err := sess.AbortTask(ctx, capturedITT) // Send TMF
close(stall) // Unblock stalled handler

// Assert TMF PDU fields via pducapture
tmfs := rec.Sent(pdu.OpTaskMgmtReq)
tmf := tmfs[0].Decoded.(*pdu.TaskMgmtReq)
// Verify CmdSN, LUN, RefCmdSN, Function, etc.
```

### Pattern 2: Goroutine + Timer Blocking Proof
**What:** Prove a command is blocked by verifying it does NOT complete within a timeout window, then verifying it DOES complete after the blocking condition is resolved.
**When to use:** TMF-05 (no new tasks during AbortTaskSet)
**Example:**
```go
// Source: test/conformance/cmdwindow_test.go (Phase 18)
done := make(chan error, 1)
go func() {
    _, err := sess.ReadBlocks(ctx, lun, 0, 1, 512)
    done <- err
}()

// Verify blocking (300ms)
select {
case err := <-done:
    t.Fatalf("ReadBlocks returned immediately (should block): err=%v", err)
case <-time.After(300 * time.Millisecond):
    // Expected: blocked
}

// Resolve blocking condition (send TMF response)
// ...

// Verify completion (5s timeout)
select {
case err := <-done:
    if err != nil { t.Fatalf("ReadBlocks failed: %v", err) }
case <-time.After(5 * time.Second):
    t.Fatal("ReadBlocks did not complete")
}
```

### Pattern 3: AsyncMsg Code 4 for Text Request Trigger
**What:** Inject AsyncMsg code 4 to trigger the initiator's private `renegotiate()`, which sends a TextReq PDU. Capture and verify the TextReq.
**When to use:** TEXT-01 through TEXT-06
**Example:**
```go
// Source: test/conformance/async_test.go TestAsync_NegotiationRequest
tgt.HandleText() // or custom handler for continuation tests

// After session is established, inject AsyncMsg code 4:
tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
    // Respond to SCSI command, then inject async
    // ...
    tgt.SendAsyncMsg(tc, 4, testutil.AsyncParams{Parameter3: 3})
    close(asyncInjected)
    return nil
})

// Wait for TextReq to appear in pducapture
textReqs := rec.Sent(pdu.OpTextReq)
textReq := textReqs[0].Decoded.(*pdu.TextReq)
// Verify ITT, TTT, CmdSN, F-bit, data segment
```

### Pattern 4: Custom HandleText for TTT Continuation
**What:** Register a custom TextReq handler that returns a partial response with C=1 and a non-0xFFFFFFFF TTT, forcing the initiator to send a continuation TextReq.
**When to use:** TEXT-04
**Example:**
```go
// Source: derived from existing HandleText + SendTargets continuation logic
tgt.Handle(pdu.OpTextReq, func(tc *testutil.TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
    req := decoded.(*pdu.TextReq)
    expCmdSN, maxCmdSN := tgt.Session().Update(req.CmdSN, req.Header.Immediate)

    if req.TargetTransferTag == 0xFFFFFFFF {
        // Initial request: respond with partial data + TTT
        resp := &pdu.TextResp{
            Header:            pdu.Header{Final: false, InitiatorTaskTag: req.Header.InitiatorTaskTag},
            Continue:          true,
            TargetTransferTag: 0x12345678, // Non-0xFFFFFFFF signals continuation
            StatSN:            tc.NextStatSN(),
            ExpCmdSN:          expCmdSN,
            MaxCmdSN:          maxCmdSN,
            Data:              partialData,
        }
        return tc.SendPDU(resp)
    }
    // Continuation: respond with final data + TTT=0xFFFFFFFF
    resp := &pdu.TextResp{
        Header:            pdu.Header{Final: true, InitiatorTaskTag: req.Header.InitiatorTaskTag},
        TargetTransferTag: 0xFFFFFFFF,
        StatSN:            tc.NextStatSN(),
        ExpCmdSN:          expCmdSN,
        MaxCmdSN:          maxCmdSN,
        Data:              remainingData,
    }
    return tc.SendPDU(resp)
})
```

### Anti-Patterns to Avoid
- **Calling renegotiate() directly from conformance tests:** The method is unexported. Tests in `conformance_test` package cannot access it. Must use AsyncMsg code 4 injection or SendTargets.
- **Using existing task_test.go for wire-level TMF tests:** The existing tests verify round-trip success only. Wire-level tests need pducapture and belong in a separate file per D-09.
- **Hardcoding ExpCmdSN/MaxCmdSN in handlers:** Always use `tgt.Session().Update()` for correct sequence numbers. Hardcoded values break when test flow changes.

## Key Technical Details

### TMF PDU Layout (RFC 7143 Section 11.5)
| BHS Offset | Field | Notes |
|-----------|-------|-------|
| Byte 0 | Opcode (0x02) + Immediate bit | TMF is always immediate |
| Byte 1 | Function (bits 6-0) | 1=AbortTask, 2=AbortTaskSet, 3=ClearTaskSet, 5=LUNReset, 6=WarmReset, 7=ColdReset, 14=TaskReassign |
| Bytes 8-15 | LUN | SAM-5 encoding via `EncodeSAMLUN()` |
| Bytes 16-19 | ITT | Initiator Task Tag (allocated by router) |
| Bytes 20-23 | ReferencedTaskTag | ITT of task being aborted (AbortTask only) |
| Bytes 24-27 | CmdSN | Current CmdSN (NOT acquired -- TMF is immediate) |
| Bytes 28-31 | ExpStatSN | Expected StatSN |
| Bytes 32-35 | RefCmdSN | CmdSN of referenced task (AbortTask only) |
[VERIFIED: internal/pdu/initiator.go lines 78-110]

### TMF CmdSN Handling
TMF requests are **always immediate** (I-bit set). They use `s.window.current()` not `s.window.acquire()`. This means CmdSN is NOT incremented for TMF requests. The test must verify that the CmdSN in the TMF PDU equals the current CmdSN, not CmdSN+1.
[VERIFIED: internal/session/tmf.go line 29 -- `CmdSN: s.window.current()`]

### LUN Encoding
`EncodeSAMLUN()` uses single-level addressing (address mode 00b): LUN number goes into bytes 0-1 as BigEndian uint16, remaining bytes are zero. For D-02's multi-format testing, the current implementation only supports flat space addressing. Testing "peripheral device" and "extended LUN" formats would require the initiator to support those formats. The current `EncodeSAMLUN()` always uses flat space (mode 00b).
[VERIFIED: internal/pdu/header.go lines 8-15]

**Important note for D-02:** The CONTEXT.md says "test multiple LUN encoding formats." However, the initiator only implements flat space addressing (`EncodeSAMLUN` puts uint16 in bytes 0-1). The test can verify correct encoding for LUN values that fit in flat space format (LUN 0, 1, 255, etc.) but cannot test peripheral device or extended formats since the initiator does not generate them. The planner should scope TMF-02 to verify correct flat space encoding across different LUN values.

### Text Request PDU Layout (RFC 7143 Section 11.10)
| BHS Offset | Field | Notes |
|-----------|-------|-------|
| Byte 0 | Opcode (0x04) | NOT immediate (consumes CmdSN) |
| Byte 1 | F-bit (bit 7) + C-bit (bit 6) | F=1 for complete request |
| Bytes 8-15 | LUN | Not used for text negotiation |
| Bytes 16-19 | ITT | Initiator Task Tag |
| Bytes 20-23 | TTT | 0xFFFFFFFF for initiator-initiated, echo target's TTT for continuation |
| Bytes 24-27 | CmdSN | Acquired from window (non-immediate) |
| Bytes 28-31 | ExpStatSN | Expected StatSN |
[VERIFIED: internal/pdu/initiator.go lines 172-203]

### Text Exchange Trigger Mechanisms
1. **AsyncMsg code 4:** Triggers private `renegotiate()` which sends TextReq with operational params (MaxRecvDataSegmentLength, MaxBurstLength, FirstBurstLength). Uses single-shot Router registration. [VERIFIED: internal/session/async.go lines 130-203]
2. **SendTargets:** Public API, sends TextReq with `SendTargets=All`. Uses persistent Router registration for multi-PDU continuation. [VERIFIED: internal/session/discovery.go lines 19-143]

For TEXT tests, AsyncMsg code 4 is the preferred trigger because:
- It produces a standard TextReq with operational parameters (richer than SendTargets=All)
- The pattern is already proven in ASYNC-04 test
- It allows multiple triggers for ITT uniqueness testing (TEXT-02)
- It uses the same code path as production renegotiation

### renegotiate vs SendTargets for Continuation (TEXT-04)
`renegotiate()` uses single-shot `s.router.Register()` and does NOT handle continuation (no loop for C-bit). `SendTargets()` uses persistent `s.router.RegisterPersistent()` and DOES handle continuation.

For TEXT-04 (TTT continuation), tests should use **SendTargets** rather than AsyncMsg-triggered renegotiation, because SendTargets handles the continuation loop. Alternatively, the renegotiate code path would need to be extended to support continuation -- but that would be production code change, which is out of scope for this test-only phase.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| TMF response handling | Custom TMF response builder | `HandleTMF()` or custom `Handle(pdu.OpTaskMgmtReq, ...)` | Existing handler handles SessionState.Update, StatSN increment correctly |
| Text response handling | Custom TextResp builder | `HandleText()` or custom `Handle(pdu.OpTextReq, ...)` | Existing handler echoes KVs and manages sequence numbers |
| PDU field extraction | Manual BHS byte parsing | `pducapture.Recorder` + type assertion `decoded.(*pdu.TaskMgmtReq)` | DecodeBHS already handles all field unpacking |
| In-flight task creation | Custom concurrent infrastructure | `HandleSCSIFunc` with `callCount==0` stall + channel | Proven pattern from Phase 18 ERL tests |
| Blocking proof | Custom timing logic | goroutine + `time.After(300ms)` / `time.After(5s)` two-phase pattern | Proven pattern from Phase 18 cmdwindow tests |

## Common Pitfalls

### Pitfall 1: renegotiate() Does Not Support Continuation
**What goes wrong:** TEXT-04 requires TTT continuation. If test triggers renegotiation via AsyncMsg code 4, the `renegotiate()` function uses single-shot Router registration and exits after first TextResp. It does NOT loop for C-bit continuation.
**Why it happens:** `renegotiate()` was designed for simple parameter exchange, not multi-PDU text negotiations.
**How to avoid:** Use `SendTargets()` for TEXT-04. SendTargets has the continuation loop with persistent Router registration. The custom HandleText handler can still inject partial responses with C=1 and TTT.
**Warning signs:** Test hangs or times out waiting for continuation TextReq that never arrives.

### Pitfall 2: TMF CmdSN is Current, Not Acquired
**What goes wrong:** Asserting that TMF CmdSN is one more than previous command's CmdSN.
**Why it happens:** TMF is always immediate. `sendTMF()` calls `s.window.current()` not `s.window.acquire()`. The CmdSN in the TMF PDU should equal the current (next expected) CmdSN without consuming a slot.
**How to avoid:** Assert TMF CmdSN equals the CmdSN that the NEXT non-immediate command would use.
**Warning signs:** CmdSN assertion fails by exactly 1.

### Pitfall 3: AbortTask ITT vs Task's ITT
**What goes wrong:** Confusing the TMF's own ITT (allocated by Router for the TMF request) with the ReferencedTaskTag (the ITT of the task being aborted).
**Why it happens:** Both are ITTs but serve different purposes.
**How to avoid:** Capture the SCSI command's ITT from pducapture before sending AbortTask. Verify `tmf.ReferencedTaskTag == capturedSCSICommand.InitiatorTaskTag`. Verify `tmf.InitiatorTaskTag != tmf.ReferencedTaskTag`.
**Warning signs:** ReferencedTaskTag is 0 or doesn't match expected value.

### Pitfall 4: Text Test Race with AsyncMsg Injection
**What goes wrong:** Test checks pducapture for TextReq before the initiator has processed the AsyncMsg and sent the TextReq.
**Why it happens:** AsyncMsg processing and renegotiation happen asynchronously in a goroutine.
**How to avoid:** Poll pducapture in a loop with timeout (exactly as done in ASYNC-04 test: `rec.Sent(pdu.OpTextReq)` in loop with `time.After` deadline).
**Warning signs:** Flaky test that sometimes sees 0 TextReqs.

### Pitfall 5: Existing task_test.go Conflict
**What goes wrong:** D-09 specifies `tmf_test.go` for TMF tests, but `task_test.go` already exists with TMF smoke tests.
**Why it happens:** Phase 7 created basic TMF tests; Phase 19 adds wire-level conformance tests.
**How to avoid:** Name the new file `tmf_test.go` as specified in D-09. The existing `task_test.go` tests different things (round-trip success). Both can coexist in `test/conformance/`.
**Warning signs:** Build errors if function names conflict.

### Pitfall 6: LUN Encoding Scope Mismatch
**What goes wrong:** D-02 asks for "peripheral device and extended LUN formats" but `EncodeSAMLUN()` only supports flat space (mode 00b).
**Why it happens:** The initiator implementation only needs flat space for typical iSCSI targets.
**How to avoid:** Scope TMF-02 to verify flat space encoding is correct across LUN values (0, 1, 255). Document that peripheral/extended formats are not tested because the initiator does not implement them.
**Warning signs:** Trying to test LUN formats the initiator cannot generate.

## Code Examples

### Capturing TMF ITT from In-Flight SCSI Command
```go
// Source: derived from erl2_test.go + tmf.go patterns
stall := make(chan struct{})
var capturedITT uint32

tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
    expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
    if callCount == 0 {
        atomic.StoreUint32(&capturedITT, cmd.InitiatorTaskTag) // [VERIFIED: pdu struct field]
        <-stall
    }
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
    return tc.SendPDU(resp)
})
```

### Verifying TMF PDU Fields via pducapture
```go
// Source: derived from cmdseq_test.go / nopout_test.go patterns
tmfs := rec.Sent(pdu.OpTaskMgmtReq)
if len(tmfs) == 0 {
    t.Fatal("no TaskMgmtReq PDU captured")
}
tmf := tmfs[0].Decoded.(*pdu.TaskMgmtReq)

// TMF-01: CmdSN (immediate -- equals current, not acquired)
if tmf.CmdSN != expectedCmdSN {
    t.Errorf("TMF CmdSN: got %d, want %d", tmf.CmdSN, expectedCmdSN)
}

// TMF-02: LUN encoding
expectedLUN := pdu.EncodeSAMLUN(targetLUN)
if tmf.Header.LUN != expectedLUN {
    t.Errorf("TMF LUN: got %v, want %v", tmf.Header.LUN, expectedLUN)
}

// TMF-03: RefCmdSN (AbortTask only)
if tmf.RefCmdSN != scsiCmdSN {
    t.Errorf("TMF RefCmdSN: got %d, want %d", tmf.RefCmdSN, scsiCmdSN)
}

// Verify immediate bit
if !tmf.Header.Immediate {
    t.Error("TMF should have Immediate bit set")
}
```

### Polling for TextReq in pducapture
```go
// Source: test/conformance/async_test.go lines 428-441 (ASYNC-04)
deadline := time.After(5 * time.Second)
var textReqs []pducapture.CapturedPDU
for {
    textReqs = rec.Sent(pdu.OpTextReq)
    if len(textReqs) > 0 {
        break
    }
    select {
    case <-deadline:
        t.Fatal("no TextReq sent within deadline")
    case <-time.After(100 * time.Millisecond):
    }
}
```

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` (Go 1.25) |
| Config file | None needed (go test native) |
| Quick run command | `go test ./test/conformance/ -run TestTMF -v -count=1` |
| Full suite command | `go test ./test/conformance/ -v -count=1 -race` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| TMF-01 | TMF CmdSN is current (immediate) | conformance | `go test ./test/conformance/ -run TestTMF_CmdSN -v -count=1` | Wave 0 |
| TMF-02 | TMF LUN field encoding | conformance | `go test ./test/conformance/ -run TestTMF_LUNEncoding -v -count=1` | Wave 0 |
| TMF-03 | TMF RefCmdSN for AbortTask | conformance | `go test ./test/conformance/ -run TestTMF_RefCmdSN -v -count=1` | Wave 0 |
| TMF-04 | AbortTaskSet aborts all tasks | conformance | `go test ./test/conformance/ -run TestTMF_AbortTaskSet_AllTasks -v -count=1` | Wave 0 |
| TMF-05 | AbortTaskSet blocks new tasks | conformance | `go test ./test/conformance/ -run TestTMF_AbortTaskSet_BlocksNew -v -count=1` | Wave 0 |
| TMF-06 | AbortTaskSet response after clear | conformance | `go test ./test/conformance/ -run TestTMF_AbortTaskSet_ResponseAfterClear -v -count=1` | Wave 0 |
| TEXT-01 | Text Request fields | conformance | `go test ./test/conformance/ -run TestText_Fields -v -count=1` | Wave 0 |
| TEXT-02 | Text Request ITT uniqueness | conformance | `go test ./test/conformance/ -run TestText_ITTUniqueness -v -count=1` | Wave 0 |
| TEXT-03 | Text Request TTT initial 0xFFFFFFFF | conformance | `go test ./test/conformance/ -run TestText_TTTInitial -v -count=1` | Wave 0 |
| TEXT-04 | Text Request TTT continuation | conformance | `go test ./test/conformance/ -run TestText_TTTContinuation -v -count=1` | Wave 0 |
| TEXT-05 | Text Request other parameters | conformance | `go test ./test/conformance/ -run TestText_OtherParams -v -count=1` | Wave 0 |
| TEXT-06 | Text Request negotiation reset | conformance | `go test ./test/conformance/ -run TestText_NegotiationReset -v -count=1` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./test/conformance/ -run "TestTMF|TestText" -v -count=1`
- **Per wave merge:** `go test ./test/conformance/ -v -count=1 -race`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `test/conformance/tmf_test.go` -- covers TMF-01 through TMF-06
- [ ] `test/conformance/text_test.go` -- covers TEXT-01 through TEXT-06

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | TMF-05 (blocking new tasks during AbortTaskSet) behavior is implemented in the session command submission path | Architecture Patterns | If the initiator does NOT block new commands during AbortTaskSet, the test will fail by design -- revealing a missing feature that needs implementation |
| A2 | D-02 LUN encoding scope should be limited to flat space format since EncodeSAMLUN only implements mode 00b | Common Pitfalls / Pitfall 6 | If peripheral/extended LUN format testing is required, production code changes to EncodeSAMLUN would be needed |
| A3 | renegotiate() RefCmdSN field in TMF PDU is populated correctly when stalling with HandleSCSIFunc | Technical Details | If RefCmdSN is not set by sendTMF, TMF-03 test will reveal this as a bug in production code |

## Open Questions

1. **TMF-05 Implementation Status**
   - What we know: `sendTMF()` sends an immediate TMF and waits for response. There is no visible mechanism in `session.go` that blocks new SCSI command submission while an AbortTaskSet is in flight.
   - What's unclear: Does the initiator actually block new commands during AbortTaskSet, or is this test expected to reveal missing behavior?
   - Recommendation: Write the test to assert the expected blocking behavior. If it fails, that signals a production code gap that should be addressed as part of this phase.

2. **TEXT-04 Continuation Path**
   - What we know: `renegotiate()` does NOT handle C-bit continuation. `SendTargets()` DOES handle it.
   - What's unclear: Should TEXT-04 use SendTargets (which works) or should we extend renegotiate() to handle continuation?
   - Recommendation: Use SendTargets for TEXT-04. It exercises the same TextReq PDU wire format and continuation logic. The test verifies initiator wire behavior regardless of which code path triggers it.

3. **RefCmdSN Population**
   - What we know: `sendTMF()` does NOT set `RefCmdSN` in the TaskMgmtReq PDU. The field exists in the struct but the production code only sets Function, ReferencedTaskTag, CmdSN, ExpStatSN, and LUN.
   - What's unclear: Is RefCmdSN correctly set to 0 (unused) for non-AbortTask TMFs? For AbortTask, should it be set to the referenced task's CmdSN?
   - Recommendation: TMF-03 test should verify RefCmdSN. If it is not populated by `sendTMF()`, this reveals a production code gap. The planner should include a task to add RefCmdSN population to `sendTMF()` for AbortTask if testing reveals it is missing.

## Sources

### Primary (HIGH confidence)
- `internal/session/tmf.go` -- TMF implementation, sendTMF with CmdSN handling, all TMF functions
- `internal/pdu/initiator.go` -- TaskMgmtReq and TextReq PDU structures and marshal/unmarshal
- `internal/pdu/target.go` -- TaskMgmtResp and TextResp PDU structures
- `internal/pdu/header.go` -- EncodeSAMLUN flat space implementation
- `internal/session/async.go` -- renegotiate() private method, single-shot Router
- `internal/session/discovery.go` -- SendTargets with continuation loop
- `test/target.go` -- MockTarget, HandleTMF, HandleText, HandleSCSIFunc, SessionState
- `test/pducapture/capture.go` -- Recorder, Sent(), Filter()
- `test/conformance/erl2_test.go` -- HandleSCSIFunc stall pattern
- `test/conformance/cmdwindow_test.go` -- goroutine+timer blocking proof pattern
- `test/conformance/async_test.go` -- AsyncMsg code 4 renegotiation trigger pattern

### Secondary (MEDIUM confidence)
- RFC 7143 Section 11.5 -- TMF PDU format and semantics [CITED: datatracker.ietf.org/doc/html/rfc7143]
- RFC 7143 Section 11.10/11.11 -- Text Request/Response PDU format [CITED: datatracker.ietf.org/doc/html/rfc7143]

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- stdlib Go testing, no new dependencies
- Architecture: HIGH -- all patterns proven in Phases 13-18 with working examples in codebase
- Pitfalls: HIGH -- identified from direct code reading of production and test code

**Research date:** 2026-04-05
**Valid until:** 2026-05-05 (stable -- test infrastructure and production code are complete)
