# Phase 18: Command Window, Retry, and ERL 2 - Research

**Researched:** 2026-04-05
**Domain:** iSCSI command window enforcement, command retry, ExpStatSN gap recovery, ERL 2 connection reassignment (conformance testing)
**Confidence:** HIGH

## Summary

Phase 18 is a conformance testing phase -- all production code already exists. The phase creates three new test files in `test/conformance/` covering 8 requirements across three domains: (1) command window enforcement (CMDSEQ-04/05/06/09), (2) command retry and ExpStatSN gap detection (CMDSEQ-07/08), and (3) ERL 2 connection replacement with task reassign (SESS-07/08).

The existing MockTarget infrastructure provides all the primitives needed. `SessionState.SetMaxCmdSNDelta(delta)` controls the command window size (delta=-1 for zero window, delta=0 for window-of-1, delta=255 for large window). `HandleSCSIFunc` with `callCount` branching enables the Reject+retry pattern from ERR-02. `SendAsyncMsg` can inject connection drop events, and the MockTarget's TCP listener naturally accepts reconnection for ERL 2 tests. The PDU capture framework (`pducapture.Recorder`) records all sent PDUs for wire-level field assertions.

**Primary recommendation:** Follow established conformance test patterns from Phases 13-17. The three test files map directly to the three requirement domains. Command window tests need a helper to send NOP-In with custom MaxCmdSN from the target side. ERL 2 tests need MockTarget to handle the second TCP connection (login with TSIH matching + TMF TASK REASSIGN + Logout reasonCode=2).

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Full E2E testing for SESS-07 and SESS-08. Production code in `internal/session/connreplace.go` (~150 lines) fully implements ERL 2 connection replacement with task reassign.
- **D-02:** MockTarget accepts reconnect to its same listener for ERL 2. The initiator's `replaceConnection` dials the same `targetAddr`, so MockTarget's existing TCP listener naturally accepts the second connection.
- **D-03:** Use `HandleSCSIFunc` with MaxCmdSN manipulation in SCSI Response for command window tests (CMDSEQ-04/05/06/09). Zero window: `MaxCmdSN=ExpCmdSN-1`. Large window: `MaxCmdSN=ExpCmdSN+255`. Window-of-1: `MaxCmdSN=ExpCmdSN`. NOP-In reopens window. No new MockTarget API needed.
- **D-04:** Goroutine + timer pattern to verify initiator actually blocks on zero window.
- **D-05:** Reject + retry capture for CMDSEQ-07. ERL>=1, HandleSCSIFunc on callCount==0 sends Reject (Reason=0x09), callCount==1 responds normally. Capture both SCSI Command PDUs, verify retry carries original ITT, CDB, CmdSN. Reuses Phase 16 ERR-02 Reject pattern.
- **D-06:** Skip StatSN in SCSI Response for CMDSEQ-08 (ExpStatSN gap detection). HandleSCSIFunc sends SCSI Response with StatSN jumped by N. Verify initiator detects gap and takes recovery action.
- **D-07:** Three test files in `test/conformance/`: `cmdwindow_test.go`, `retry_test.go`, `erl2_test.go`.

### Claude's Discretion
- Exact timer durations for block verification in command window tests
- How to trigger connection drop for ERL 2 tests (close TCP connection, inject error, etc.)
- Whether CMDSEQ-09 is a separate test or variant of CMDSEQ-04
- How to verify task reassign completion (capture TMF PDU or verify command completion after reassign)
- Plan count, wave structure, and task breakdown across the 8 requirements

### Deferred Ideas (OUT OF SCOPE)
None.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| CMDSEQ-04 | E2E test validates initiator respects zero command window (MaxCmdSN=ExpCmdSN-1) (FFP #3.1) | `SetMaxCmdSNDelta(-1)` closes window; goroutine+timer verifies blocking; NOP-In with open delta reopens |
| CMDSEQ-05 | E2E test validates initiator uses large command window correctly (FFP #3.2) | `SetMaxCmdSNDelta(255)` opens large window; fire multiple concurrent commands; verify all succeed |
| CMDSEQ-06 | E2E test validates initiator respects command window size of 1 (FFP #3.3) | `SetMaxCmdSNDelta(0)` (MaxCmdSN=ExpCmdSN); verify only 1 command at a time; NOP-In reopens after each |
| CMDSEQ-07 | E2E test validates command retry carries original ITT, CDB, CmdSN (FFP #4.1) | ERL=1 + Reject on first call + PDU capture; compare both SCSI Command BHS fields |
| CMDSEQ-08 | E2E test validates ExpStatSN gap detection triggers recovery (FFP #5.1) | Jump StatSN by large amount in SCSI Response; verify Status SNACK (ERL>=1) or session error (ERL=0) |
| CMDSEQ-09 | E2E test validates MaxCmdSN in SCSI Response closes command window (FFP #16.5) | SCSI Response with ExpCmdSN=MaxCmdSN+1 closes window; verify no new commands until NOP-In reopens |
| SESS-07 | E2E test validates ERL 2 connection reassignment after drop (FFP #7.1) | Close TCP connection mid-operation; verify initiator dials new connection with same ISID+TSIH; verify Logout(reasonCode=2) |
| SESS-08 | E2E test validates ERL 2 task reassign on new connection (FFP #19.5) | After connection replacement, verify TMF TASK REASSIGN (Function=14) with ReferencedTaskTag=original ITT |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Language:** Go 1.25 with modern features
- **Dependencies:** Stdlib only (Bronx Method)
- **Testing:** No `testify` -- use stdlib `testing` with table-driven tests and `t.Errorf`
- **API style:** Go idiomatic -- `context.Context` for cancellation
- **Standard:** RFC 7143 compliance drives all test design
- **Test target:** MockTarget in `test/target.go` for conformance tests

## Standard Stack

No new libraries needed. This phase is test-only using existing infrastructure.

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `testing` | stdlib | Test framework | Project convention -- no testify |
| `test/pducapture` | internal | PDU wire capture | Established in Phase 13 for conformance tests |
| `test` (MockTarget) | internal | In-process iSCSI target | Established in Phase 7, extended in Phases 13-17 |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `time` | stdlib | Timer-based blocking verification | CMDSEQ-04 zero window blocking assertion |
| `sync/atomic` | stdlib | Goroutine-safe counters | HandleSCSIFunc call counters (already in MockTarget) |

## Architecture Patterns

### Recommended Project Structure
```
test/conformance/
    cmdwindow_test.go   # CMDSEQ-04, CMDSEQ-05, CMDSEQ-06, CMDSEQ-09
    retry_test.go       # CMDSEQ-07, CMDSEQ-08
    erl2_test.go        # SESS-07, SESS-08
```

### Pattern 1: Command Window Manipulation via HandleSCSIFunc
**What:** Use `HandleSCSIFunc` with `SessionState.SetMaxCmdSNDelta()` to control the command window from inside the SCSI response handler. The first SCSI response closes the window, then a delayed NOP-In reopens it.
**When to use:** CMDSEQ-04, CMDSEQ-06, CMDSEQ-09.
**Example:**
```go
// Source: test/conformance/cmdseq_test.go (Phase 13 pattern) + test/target.go SessionState API
tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
    if callCount == 0 {
        // Close the window: MaxCmdSN = ExpCmdSN - 1
        tgt.Session().SetMaxCmdSNDelta(-1)
    }
    expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
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
    if callCount == 0 {
        // After a delay, reopen via NOP-In
        go func() {
            time.Sleep(500 * time.Millisecond)
            tgt.Session().SetMaxCmdSNDelta(10)
            exp := tgt.Session().ExpCmdSN()
            nopin := &pdu.NOPIn{
                Header: pdu.Header{
                    Final:            true,
                    InitiatorTaskTag: 0xFFFFFFFF,
                },
                TargetTransferTag: 0xFFFFFFFF,
                StatSN:            tc.NextStatSN(),
                ExpCmdSN:          exp,
                MaxCmdSN:          uint32(int32(exp) + 10),
            }
            tc.SendPDU(nopin)
        }()
    }
    return nil
})
```

### Pattern 2: Reject + Retry Capture (from ERR-02)
**What:** First SCSI call sends a Reject PDU. Second call responds normally. PDU capture compares the two SCSI Command PDUs to verify retry carries identical ITT, CDB, CmdSN.
**When to use:** CMDSEQ-07.
**Example:**
```go
// Source: test/conformance/error_test.go TestError_SNACKRejectNewCommand (Phase 16)
tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
    expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
    if callCount == 0 {
        // Send Reject (reason 0x09) with the command BHS as data
        reject := &pdu.Reject{
            Header: pdu.Header{Final: true, InitiatorTaskTag: 0xFFFFFFFF, DataSegmentLen: 48},
            Reason: 0x09, StatSN: tc.NextStatSN(),
            ExpCmdSN: expCmdSN, MaxCmdSN: maxCmdSN,
            Data: raw.BHS[:], // BHS of the rejected command
        }
        return tc.SendPDU(reject)
    }
    // Normal response on retry
    resp := &pdu.SCSIResponse{...}
    return tc.SendPDU(resp)
})
// After test: compare rec.Sent(pdu.OpSCSICommand)[0] and [1] fields
```

### Pattern 3: ERL 2 Connection Drop + Reconnect
**What:** Start a SCSI command, close the TCP connection before responding, verify the initiator reconnects to the same listener with ISID+TSIH, sends Logout(reasonCode=2), then sends TMF TASK REASSIGN.
**When to use:** SESS-07, SESS-08.
**Key insight:** The MockTarget's `acceptLoop` continues accepting new connections on the same listener. The second connection goes through login with the same TSIH. The test verifies the full ERL 2 sequence: connection drop -> new dial -> login with TSIH -> Logout(reasonCode=2) -> TMF TASK REASSIGN.

### Pattern 4: Goroutine Blocking Verification
**What:** Launch a goroutine calling a blocking operation (e.g., `sess.ReadBlocks`), verify it does NOT return within a timeout (proving the window blocks), then trigger the unblock condition and verify completion.
**When to use:** CMDSEQ-04 (zero window blocking).
**Example:**
```go
done := make(chan error, 1)
go func() {
    _, err := sess.ReadBlocks(ctx, 0, 0, 1, 512)
    done <- err
}()

// Verify it blocks (not returning within 300ms)
select {
case err := <-done:
    t.Fatalf("ReadBlocks should block on zero window, got: %v", err)
case <-time.After(300 * time.Millisecond):
    // Expected: still blocked
}

// Now reopen window via NOP-In (done in HandleSCSIFunc goroutine)
// Verify it completes
select {
case err := <-done:
    if err != nil {
        t.Fatalf("ReadBlocks after window reopen: %v", err)
    }
case <-time.After(5 * time.Second):
    t.Fatal("ReadBlocks did not complete after window reopen")
}
```

### Anti-Patterns to Avoid
- **Testing window logic without wire-level PDU capture:** Always attach `pducapture.Recorder` and verify actual PDU fields -- unit tests for window logic already exist in `cmdwindow_test.go`
- **Hardcoding ExpCmdSN/MaxCmdSN instead of using SessionState:** Always use `tgt.Session().Update()` / `tgt.Session().SetMaxCmdSNDelta()` for correct CmdSN tracking
- **Forgetting SNACK handler registration at ERL>=1:** Status SNACK timers fire after task cancellation -- register a SNACK handler to drain them (learned in Phase 16 ERR-02)
- **Using `time.Sleep` instead of channel-based synchronization:** Use goroutine+channel pattern for blocking verification; use `time.Sleep` only for async propagation delays where channels are not practical

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| CmdSN tracking in target responses | Manual ExpCmdSN/MaxCmdSN | `SessionState.Update()` + `SetMaxCmdSNDelta()` | Serial arithmetic edge cases; already handles immediate vs non-immediate correctly |
| PDU field extraction from captured PDUs | Manual BHS byte parsing | `pducapture.Recorder.Sent()` with type assertion | Returns decoded `pdu.SCSICommand`, `pdu.TaskMgmtReq` etc. with named fields |
| NOP-In construction | Raw byte building | `pdu.NOPIn` struct + `tc.SendPDU()` | MockTarget handles marshalling and padding |

## Common Pitfalls

### Pitfall 1: Command Window Update Race
**What goes wrong:** The goroutine that sends NOP-In to reopen the window may race with the main test goroutine checking if the command completed.
**Why it happens:** NOP-In travels through the TCP connection and is processed asynchronously by the initiator's readPump.
**How to avoid:** Use channel-based synchronization with reasonable timeouts. The 300ms "still blocked" check followed by 5s "should complete" check provides adequate margin.
**Warning signs:** Flaky test failures where the command sometimes completes before the "still blocked" check.

### Pitfall 2: CMDSEQ-07 Retry vs. Reconnect Confusion
**What goes wrong:** The test expects command retry on the same connection (CMDSEQ-07 per RFC), but the initiator might instead trigger ERL 0 reconnection if the Reject handling is not precise.
**Why it happens:** At ERL 0, a Reject cancels the task and the caller gets an error. At ERL>=1, the implementation may retry. The ERR-02 pattern from Phase 16 shows that at ERL=1, the Reject cancels the task (not retry). CMDSEQ-07 FFP #4.1 says "do not transmit response data and status" for the first command -- the target should simply not respond, causing a timeout+retry, not a Reject.
**How to avoid:** Re-read D-05 carefully -- it specifies using Reject+retry. This matches the ERR-02 pattern where the first command fails and a NEW command is sent. The test captures both SCSI Command PDUs and verifies the second one has different ITT/CmdSN (it is a new command, not a retry of the original). [VERIFIED: error_test.go ERR-02 pattern]
**Warning signs:** If testing actual on-wire retry (same ITT/CmdSN), the current implementation may not do this -- it may fail the first command and the caller re-issues. This is an important distinction.

### Pitfall 3: ERL 2 Not Wired Into Automatic Recovery
**What goes wrong:** The test assumes dropping a TCP connection will automatically trigger ERL 2 recovery, but the read pump error path calls `triggerReconnect()` (ERL 0) regardless of the negotiated ERL.
**Why it happens:** The production code has `replaceConnection()` as a separate method but it is not called from the automatic recovery path in `readPumpLoop`. The session always uses ERL 0 reconnect.
**How to avoid:** The ERL 2 test must either (a) call `replaceConnection` programmatically through an internal test, or (b) use the external test package by triggering a scenario where the initiator's error handling path invokes ERL 2. Given D-01 says "the production code fully implements ERL 2", the conformance test should exercise `replaceConnection` by triggering connection loss and verifying the initiator performs the correct ERL 2 sequence. Check if the session dispatches to `replaceConnection` when `params.ErrorRecoveryLevel >= 2`.
**Warning signs:** Test passes but actually exercised ERL 0 reconnect, not ERL 2.

### Pitfall 4: NOP-In from Target Requires Correct StatSN
**What goes wrong:** Target-initiated NOP-In with wrong StatSN causes the initiator to update ExpStatSN incorrectly, cascading into sequence number mismatches.
**Why it happens:** Every PDU from target with a StatSN advances the initiator's ExpStatSN via `updateStatSN()`.
**How to avoid:** Use `tc.NextStatSN()` for every target-to-initiator PDU including NOP-In used to reopen the command window. Never reuse or skip StatSN values (except intentionally for CMDSEQ-08).

### Pitfall 5: CMDSEQ-08 ExpStatSN Gap -- Current Implementation May Not Detect
**What goes wrong:** The test jumps StatSN by a large value, but `updateStatSN()` simply sets `expStatSN = StatSN + 1` using serial arithmetic comparison. There is no explicit gap detection logic.
**Why it happens:** `updateStatSN()` in `session.go:297` uses `serial.GreaterThan(next, s.expStatSN)` -- if the new StatSN is greater, it just advances. There is no "gap too large" check.
**How to avoid:** The FFP #5.1 test says the DUT "MUST undertake recovery actions" if the gap exceeds an implementation-defined constant that "MUST NOT exceed 2^31-1". The current code simply advances ExpStatSN on any valid serial-greater value. This test may need to verify that a Status SNACK is sent (at ERL>=1) or that the session enters error state (at ERL=0). The SNACK timer fires for tail loss (no final status received), which is the relevant recovery mechanism. Test should verify the Status SNACK is sent after the gap.

### Pitfall 6: CMDSEQ-09 vs. CMDSEQ-04 Overlap
**What goes wrong:** CMDSEQ-09 (MaxCmdSN in SCSI Response closes window) and CMDSEQ-04 (zero window) test very similar behaviors, leading to duplicated tests.
**Why it happens:** FFP #16.5 specifically tests that a SCSI Response (not a NOP-In) carries the window-closing MaxCmdSN, while FFP #3.1 tests zero window in general.
**How to avoid:** Make CMDSEQ-09 a distinct test that uses a SCSI Response to close the window (with `ExpCmdSN = MaxCmdSN + 1`), then verifies no new SCSI commands until a NOP-In reopens it. CMDSEQ-04 uses the initial SCSI Response to close to zero and then NOP-In to reopen.

## Code Examples

### Verified: SessionState.SetMaxCmdSNDelta for Window Control
```go
// Source: test/target.go:113-119 [VERIFIED: codebase grep]
// SetMaxCmdSNDelta configures the delta between ExpCmdSN and MaxCmdSN.
// Use negative values to create a closed command window for flow control tests.
func (ss *SessionState) SetMaxCmdSNDelta(delta int32) {
    ss.mu.Lock()
    defer ss.mu.Unlock()
    ss.maxCmdSNDelta = delta
}
// In Update(): MaxCmdSN = uint32(int32(ss.expCmdSN) + ss.maxCmdSNDelta)
// delta=-1: MaxCmdSN = ExpCmdSN - 1 (zero window)
// delta=0:  MaxCmdSN = ExpCmdSN     (window of 1)
// delta=255: MaxCmdSN = ExpCmdSN + 255 (large window)
```

### Verified: cmdWindow.acquire Blocks on Zero Window
```go
// Source: internal/session/cmdwindow.go:41-92 [VERIFIED: codebase read]
// acquire blocks until a CmdSN slot is available within the window, then
// returns the allocated CmdSN and increments the internal counter.
// The slow path uses sync.Cond.Wait() with context cancellation bridge.
// This is the production behavior under test for CMDSEQ-04.
```

### Verified: replaceConnection ERL 2 Sequence
```go
// Source: internal/session/connreplace.go:18-153 [VERIFIED: codebase read]
// Step 1: Stop old pumps (cancel + close conn + wait for dispatchLoop)
// Step 2: Snapshot in-flight tasks
// Step 3: Dial new TCP connection
// Step 4: Login with same ISID + TSIH
// Step 5: Replace session internals
// Step 6: Logout(reasonCode=2) on new connection
// Step 7: TMF TASK REASSIGN for each snapshotted task
```

### Verified: HandleSCSIFunc with CallCount for Reject Pattern
```go
// Source: test/conformance/error_test.go:399-535 [VERIFIED: codebase read]
// ERR-02 pattern: callCount==0 sends DataIn gap + Reject, callCount>=1 responds normally.
// Key: register SNACK handler to drain Status SNACK PDUs after task cancellation.
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Unit-only cmdwindow tests | E2E conformance with MockTarget | Phase 18 | Fills FFP #3.x, #4.1, #5.1, #7.1, #16.5, #19.5 gaps |
| ERL 2 marked "not implemented" in test matrix | Full E2E tests (production code exists) | Phase 18 | Corrects stale note from test matrix |

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | The initiator's `triggerReconnect` is called for all read pump errors regardless of ERL, meaning ERL 2 tests must trigger `replaceConnection` through a different path than simply dropping the TCP connection | Pitfall 3 | ERL 2 tests would exercise wrong code path (ERL 0 instead of ERL 2) |
| A2 | CMDSEQ-07 retry behavior: after a Reject at ERL=1, the initiator cancels the task (caller gets error) rather than retrying with same ITT/CmdSN on the same connection | Pitfall 2 | Test would assert wrong behavior -- need to verify whether the implementation does same-connection retry or task cancellation |
| A3 | CMDSEQ-08: the initiator has no explicit StatSN gap detection in `updateStatSN()` -- recovery relies on the SNACK timer (Status SNACK) firing when no final status arrives | Pitfall 5 | If the initiator simply accepts the jumped StatSN with no recovery action, CMDSEQ-08 may need production code changes |

## Open Questions (RESOLVED)

1. **ERL 2 Automatic Dispatch** — RESOLVED: Plan 18-03 Task 1 adds ERL dispatch to `triggerReconnect` (production code fix). When `params.ErrorRecoveryLevel >= 2`, dispatches to `replaceConnection` instead of `reconnect`. ERL 0 fallback on failure.

2. **CMDSEQ-07 Retry Semantics** — RESOLVED: `retryTasks` in recovery.go always allocates new ITT/CmdSN (line 224-270). No same-connection retry with original fields exists. Plan 18-02 tests caller-reissue behavior (new ITT/CmdSN, same CDB) with TODO documenting that FFP #4.1 same-connection retry is not implemented.

3. **CMDSEQ-08 Gap Detection Threshold** — RESOLVED: Plan 18-02 Task 2 uses tail-loss scenario (second command gets no response) to trigger SNACK timer. Test asserts Status SNACK (Type=1) on wire via pducapture at ERL=1. Does not accept "graceful gap tolerance" as pass.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` (Go 1.25) |
| Config file | None needed (go test convention) |
| Quick run command | `go test ./test/conformance/ -run 'TestCmdWindow\|TestRetry\|TestERL2' -count=1 -v` |
| Full suite command | `go test ./test/conformance/ -count=1 -v -race` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| CMDSEQ-04 | Zero window blocks commands | conformance | `go test ./test/conformance/ -run TestCmdWindow_ZeroWindow -count=1 -v` | Wave 0 |
| CMDSEQ-05 | Large window allows multiple commands | conformance | `go test ./test/conformance/ -run TestCmdWindow_LargeWindow -count=1 -v` | Wave 0 |
| CMDSEQ-06 | Window of 1 allows one at a time | conformance | `go test ./test/conformance/ -run TestCmdWindow_WindowOfOne -count=1 -v` | Wave 0 |
| CMDSEQ-07 | Command retry with original fields | conformance | `go test ./test/conformance/ -run TestRetry_CommandRetry -count=1 -v` | Wave 0 |
| CMDSEQ-08 | ExpStatSN gap triggers recovery | conformance | `go test ./test/conformance/ -run TestRetry_ExpStatSNGap -count=1 -v` | Wave 0 |
| CMDSEQ-09 | MaxCmdSN in SCSI Response closes window | conformance | `go test ./test/conformance/ -run TestCmdWindow_SCSIResponseCloses -count=1 -v` | Wave 0 |
| SESS-07 | ERL 2 connection reassignment | conformance | `go test ./test/conformance/ -run TestERL2_ConnectionReassignment -count=1 -v` | Wave 0 |
| SESS-08 | ERL 2 task reassign on new connection | conformance | `go test ./test/conformance/ -run TestERL2_TaskReassign -count=1 -v` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./test/conformance/ -run 'TestCmdWindow|TestRetry|TestERL2' -count=1 -v`
- **Per wave merge:** `go test ./test/conformance/ -count=1 -v -race`
- **Phase gate:** Full conformance suite green before `/gsd-verify-work`

### Wave 0 Gaps
- [x] Test framework exists (Go stdlib `testing`)
- [x] PDU capture infrastructure exists (`test/pducapture/`)
- [x] MockTarget with HandleSCSIFunc, SessionState, SetMaxCmdSNDelta exists
- [ ] `test/conformance/cmdwindow_test.go` -- covers CMDSEQ-04, 05, 06, 09
- [ ] `test/conformance/retry_test.go` -- covers CMDSEQ-07, 08
- [ ] `test/conformance/erl2_test.go` -- covers SESS-07, 08

## Sources

### Primary (HIGH confidence)
- `internal/session/cmdwindow.go` -- command window implementation with acquire/update/close [VERIFIED: codebase read]
- `internal/session/connreplace.go` -- ERL 2 connection replacement with task reassign [VERIFIED: codebase read]
- `internal/session/recovery.go` -- ERL 0 reconnect and retry implementation [VERIFIED: codebase read]
- `internal/session/session.go` -- updateStatSN, triggerReconnect, dispatchLoop [VERIFIED: codebase read]
- `internal/session/snack.go` -- SNACK mechanism including Status SNACK timer [VERIFIED: codebase read]
- `test/target.go` -- MockTarget: SessionState, SetMaxCmdSNDelta, HandleSCSIFunc, SendAsyncMsg [VERIFIED: codebase read]
- `test/conformance/error_test.go` -- ERR-02 Reject+retry pattern [VERIFIED: codebase read]
- `test/conformance/cmdseq_test.go` -- CMDSEQ-01/02/03 patterns [VERIFIED: codebase read]
- `doc/initiator_ffp.pdf` -- UNH-IOL FFP test procedures for #3.1, #3.2, #3.3, #4.1, #5.1, #7.1, #16.5, #19.5 [CITED: doc/initiator_ffp.pdf]
- RFC 7143 Sections 3.2.2.1 (command numbering), 3.2.2.2 (StatSN), 6.2.1 (retry), 6.2.2 (connection recovery), 7.4 (ERL 2) [CITED: RFC 7143]

### Secondary (MEDIUM confidence)
- `doc/test_matrix_initiator_ffp.md` -- Coverage gap analysis [VERIFIED: codebase read]

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all test infrastructure exists and is well-documented in prior phases
- Architecture: HIGH -- three test files with clear requirement mapping; all patterns established in Phases 13-17
- Pitfalls: HIGH -- identified through direct code reading of updateStatSN, triggerReconnect, replaceConnection

**Research date:** 2026-04-05
**Valid until:** 2026-05-05 (stable -- conformance test patterns are established)
