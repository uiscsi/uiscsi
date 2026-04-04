# Phase 13: PDU Wire Capture Framework, MockTarget Extensions, and Command Sequencing - Research

**Researched:** 2026-04-04
**Domain:** iSCSI test infrastructure (PDU capture, MockTarget extensions, CmdSN wire validation)
**Confidence:** HIGH

## Summary

Phase 13 builds reusable test infrastructure for all subsequent v1.1 phases: a PDU capture/assertion helper that records and queries every PDU exchanged during a test, MockTarget extensions for stateful session tracking and per-command handler routing, and three CmdSN sequencing conformance tests (FFP #1.1, #2.1, #2.2). The existing `WithPDUHook` API already captures raw BHS+DataSegment bytes on every send/receive -- the capture framework wraps this with decode-and-store logic. MockTarget already has handler dispatch by opcode; extensions add per-SCSI-command routing (`HandleSCSIFunc`) and stateful CmdSN/MaxCmdSN tracking.

This phase is strictly test infrastructure -- no changes to the production iSCSI initiator code. All new code lives in `test/pducapture/` (new package) and `test/target.go` (extension of existing MockTarget). The three validation tests prove the infrastructure works by asserting CmdSN behavior on the wire.

**Primary recommendation:** Build `test/pducapture/` as a thin wrapper around `WithPDUHook` that decodes raw bytes via `pdu.DecodeBHS`, stores `CapturedPDU` records, and provides `Filter(opcode)` plus field accessors. Extend MockTarget with `HandleSCSIFunc` and a `SessionState` struct for CmdSN/MaxCmdSN tracking. Validate with three tests covering FFP #1.1, #2.1, #2.2.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Capture on initiator-side only, using `WithPDUHook`. This works with both MockTarget and real LIO target -- single capture point for all tests.
- **D-02:** Capture helper decodes raw bytes into typed `pdu.PDU` structs. Assertions read fields directly (e.g., `capture.Filter(OpSCSICommand)` then check `.CmdSN`). No raw byte offset manipulation in tests.
- **D-03:** PDU capture helpers live in dedicated `test/pducapture/` package, importable by both conformance and E2E tests.
- **D-04:** Add `HandleSCSIFunc(func)` pattern -- a single handler registered on `OpSCSICommand` that receives the decoded SCSI command and lets the test author route by CDB opcode internally. Maximum flexibility for error injection, multi-command scenarios, etc.
- **D-05:** MockTarget tracks session state (CmdSN, ExpStatSN, MaxCmdSN) internally with correct-by-default behavior. Tests configure MaxCmdSN policy (e.g., `SetMaxCmdSNDelta`). Individual handlers don't need to manage sequencing.
- **D-06:** Five extension categories needed for v1.1: (1) stateful session tracking for command window control, (2) multi-PDU Data-In with configurable DataSN gaps, (3) async message injection via `TargetConn.SendPDU`, (4) per-command handler routing (HandleSCSIFunc), (5) PDU capture middleware. Build the foundation (1, 4, 5) in this phase; extensions (2, 3) can be added incrementally in their respective phases.
- **D-07:** FFP conformance test file organization is Claude's discretion.
- **D-08:** Field-strict validation where the FFP test specifies exact values (CmdSN delta, DataSN sequence, F-bit, TTT echo). Behavioral assertions only when field checks aren't meaningful for the test.
- **D-09:** Use `t.Error` (collect all violations) rather than `t.Fatal` for PDU field assertions. Report all violations in a single test run for better debugging.

### Claude's Discretion
- FFP test file organization (D-07)
- Exact API shape of the capture/assertion helpers
- Whether to build all five MockTarget extensions in Phase 13 or defer some to consuming phases

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| CMDSEQ-01 | E2E test validates CmdSN increments by 1 for each non-immediate command on wire (FFP #1.1) | PDU capture framework + Filter(OpSCSICommand) + CmdSN field comparison across sequential PDUs |
| CMDSEQ-02 | E2E test validates immediate delivery flag and CmdSN for non-TMF commands (FFP #2.1) | PDU capture + check Header.Immediate==true and CmdSN==window.current() for NOP-Out commands |
| CMDSEQ-03 | E2E test validates immediate delivery CmdSN for task management commands (FFP #2.2) | PDU capture + Filter(OpTaskMgmtReq) + verify Header.Immediate==true and CmdSN not advanced |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib `testing` | 1.25 | Test framework | Project constraint: no testify |
| `internal/pdu` (project) | - | PDU decode/encode | DecodeBHS converts raw bytes to typed PDU structs; all field access is direct struct reads |
| `test` package (project) | - | MockTarget | Existing handler-based mock target to extend |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `sync` | stdlib | Thread-safe capture storage | PDU hook called concurrently from read/write pumps |
| `slices` | stdlib | Filtering captured PDUs | Go 1.21+ slices.Collect pattern for Filter results |
| `testing/synctest` | stdlib (Go 1.25) | Deterministic concurrent tests | If timing-sensitive assertions needed (Phase 13 tests are likely straightforward enough to not need it) |

**No installation needed.** All code is stdlib + project-internal packages. No new external dependencies.

## Architecture Patterns

### Recommended Project Structure
```
test/
  pducapture/
    capture.go        # Recorder, CapturedPDU, Hook(), Filter(), assertions
    capture_test.go   # Unit tests for the capture framework itself
  target.go           # MockTarget (extended with HandleSCSIFunc, SessionState)
  target_test.go      # Existing + new MockTarget tests
  conformance/
    cmdseq_test.go    # CMDSEQ-01, CMDSEQ-02, CMDSEQ-03 tests (NEW)
    fullfeature_test.go  # (existing)
    ...
```

### Pattern 1: PDU Capture Recorder

**What:** A thread-safe recorder that plugs into `WithPDUHook` and stores decoded PDUs for post-hoc assertions.
**When to use:** Every FFP conformance test that needs to verify PDU field values on the wire.

```go
// test/pducapture/capture.go

// CapturedPDU holds a single recorded PDU with metadata.
type CapturedPDU struct {
    Direction uiscsi.PDUDirection // PDUSend or PDUReceive
    Decoded   pdu.PDU             // Typed PDU struct (SCSICommand, TaskMgmtReq, etc.)
    Raw       []byte              // Original BHS + DataSegment bytes
    Seq       int                 // Sequence number (order captured)
}

// Recorder collects PDUs during a test session.
type Recorder struct {
    mu   sync.Mutex
    pdus []CapturedPDU
    seq  int
}

// Hook returns a function compatible with uiscsi.WithPDUHook.
func (r *Recorder) Hook() func(context.Context, uiscsi.PDUDirection, []byte) {
    return func(_ context.Context, dir uiscsi.PDUDirection, data []byte) {
        if len(data) < pdu.BHSLength {
            return
        }
        var bhs [pdu.BHSLength]byte
        copy(bhs[:], data[:pdu.BHSLength])
        decoded, err := pdu.DecodeBHS(bhs)
        if err != nil {
            return // Skip unrecognizable PDUs
        }
        r.mu.Lock()
        r.pdus = append(r.pdus, CapturedPDU{
            Direction: dir,
            Decoded:   decoded,
            Raw:       append([]byte(nil), data...), // defensive copy
            Seq:       r.seq,
        })
        r.seq++
        r.mu.Unlock()
    }
}

// Filter returns all captured PDUs matching the given opcode and direction.
func (r *Recorder) Filter(opcode pdu.OpCode, dir uiscsi.PDUDirection) []CapturedPDU { ... }

// Sent returns all PDUs sent by the initiator.
func (r *Recorder) Sent(opcode pdu.OpCode) []CapturedPDU { ... }

// All returns a snapshot of all captured PDUs.
func (r *Recorder) All() []CapturedPDU { ... }
```

**Key design choices:**
- The `Decoded` field is a `pdu.PDU` interface -- tests type-assert to concrete types (`*pdu.SCSICommand`, `*pdu.TaskMgmtReq`) for field access
- Raw bytes stored defensively (copy) since the hook data may be reused
- Sequence number enables ordering assertions across opcodes
- `DecodeBHS` is already tested and handles all 18 opcodes

### Pattern 2: HandleSCSIFunc for Per-Command Routing

**What:** A MockTarget method that registers a flexible SCSI command handler where the test function receives the decoded `*pdu.SCSICommand` and a call counter.
**When to use:** When a test needs different behavior per SCSI command (e.g., first READ succeeds, second gets BUSY) or routing by CDB opcode within a single handler.

```go
// test/target.go

// HandleSCSIFunc registers a handler called for every SCSI Command PDU.
// The handler receives the TargetConn, decoded command, and a 0-based
// call counter. The handler is responsible for sending a response via tc.
func (mt *MockTarget) HandleSCSIFunc(h func(tc *TargetConn, cmd *pdu.SCSICommand, callCount int) error) {
    count := 0
    mt.Handle(pdu.OpSCSICommand, func(tc *TargetConn, raw *transport.RawPDU, decoded pdu.PDU) error {
        cmd := decoded.(*pdu.SCSICommand)
        n := count
        count++
        return h(tc, cmd, n)
    })
}
```

**Important:** This pattern replaces `HandleSCSIRead`/`HandleSCSIWrite` for the registered connection -- only one handler per opcode. Tests that need HandleSCSIFunc should NOT also call HandleSCSIRead.

### Pattern 3: Stateful Session Tracking in MockTarget

**What:** MockTarget tracks ExpCmdSN and MaxCmdSN internally, automatically computing correct values in responses. Tests can override MaxCmdSN policy via `SetMaxCmdSNDelta`.
**When to use:** All FFP conformance tests that need correct command window behavior (Phase 13 and beyond).

```go
// SessionState tracks target-side session state for correct-by-default
// response field values. All handlers should use these methods instead
// of hardcoding CmdSN+1 / CmdSN+10.
type SessionState struct {
    mu           sync.Mutex
    expCmdSN     uint32
    maxCmdSNDelta int32 // MaxCmdSN = ExpCmdSN + delta (default: 10)
}

// Update advances ExpCmdSN after receiving a non-immediate command.
// Returns (ExpCmdSN, MaxCmdSN) for use in the response.
func (ss *SessionState) Update(cmdSN uint32, immediate bool) (expCmdSN, maxCmdSN uint32) {
    ss.mu.Lock()
    defer ss.mu.Unlock()
    if !immediate {
        ss.expCmdSN = cmdSN + 1
    }
    return ss.expCmdSN, uint32(int32(ss.expCmdSN) + ss.maxCmdSNDelta)
}
```

### Anti-Patterns to Avoid
- **Raw byte offset in tests:** Do NOT write `data[24:28]` to extract CmdSN in test assertions. Use the PDU capture framework's decoded structs (`cmd.CmdSN`).
- **t.Fatal for field assertions:** Per D-09, use `t.Error` to collect all violations in one test run.
- **Multiple handlers on OpSCSICommand:** HandleSCSIRead, HandleSCSIWrite, HandleSCSIFunc, and HandleSCSIError all register on the same opcode. Only one can be active. Use HandleSCSIFunc when you need to handle multiple SCSI command types.
- **Hardcoded MaxCmdSN in handlers:** Current handlers hardcode `cmd.CmdSN + 10`. After introducing SessionState, all handlers should use `ss.Update()` to get consistent values.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| PDU byte decoding in tests | Manual `binary.BigEndian.Uint32(data[24:28])` | `pdu.DecodeBHS()` via capture framework | Already tested, handles all opcodes, type-safe field access |
| Thread-safe PDU collection | Channel-based collectors | `sync.Mutex` + slice in Recorder | Simple, no goroutine overhead, predictable ordering |
| Full target state machine | Complete RFC 7143 target FSM | Minimal `SessionState` struct | Only need CmdSN/MaxCmdSN tracking, not full protocol |
| Custom wire tap / TCP proxy | net.Conn wrapper that intercepts bytes | `WithPDUHook` (already exists) | Hook is already plumbed through session layer, captures both directions |

**Key insight:** The existing `WithPDUHook` + `pdu.DecodeBHS` covers 90% of what the capture framework needs. The new code is glue: store decoded PDUs, provide filter/query methods, and offer assertion helpers.

## Common Pitfalls

### Pitfall 1: Login PDUs Polluting CmdSN Assertions
**What goes wrong:** The capture hook fires during login phase too. Login PDUs have CmdSN fields but they follow different rules (CmdSN starts at a session-specific initial value).
**Why it happens:** `WithPDUHook` captures ALL PDUs including login.
**How to avoid:** Filter by opcode AND direction. For CmdSN sequencing tests, only assert on `OpSCSICommand` PDUs with `direction==PDUSend`. The capture framework's `Sent(OpSCSICommand)` method makes this natural.
**Warning signs:** First captured CmdSN does not match expectations because login CmdSN is included.

### Pitfall 2: Immediate Commands Counted in CmdSN Increment Test
**What goes wrong:** CMDSEQ-01 test counts NOP-Out or TMF PDUs and expects CmdSN+1 for each, but immediate commands do NOT advance CmdSN.
**Why it happens:** Confusing "carries CmdSN" with "advances CmdSN." Per RFC 7143 Section 3.2.2.1, immediate commands carry the current CmdSN but do not increment it.
**How to avoid:** CMDSEQ-01 filters only `OpSCSICommand` PDUs (which are non-immediate in this initiator). CMDSEQ-02 and CMDSEQ-03 explicitly test that immediate commands do NOT advance CmdSN.
**Warning signs:** CmdSN gap appears in sequence when NOP-Out was sent between SCSI commands.

### Pitfall 3: Handler Registration Order in Combined Tests
**What goes wrong:** A test calls both `HandleSCSIRead` and `HandleSCSIFunc` -- the second silently overwrites the first since both register on `OpSCSICommand`.
**Why it happens:** `MockTarget.Handle()` is a map write -- last registration wins.
**How to avoid:** Use `HandleSCSIFunc` as the single SCSI command handler when tests need both read and TMF/error behavior. Route by CDB opcode inside the function.
**Warning signs:** SCSI commands get no response (the wrong handler was overwritten).

### Pitfall 4: Race Between PDU Capture and Session Close
**What goes wrong:** Test closes session, then reads captured PDUs. But session close may trigger Logout PDU exchange, and the capture hook fires concurrently during close.
**Why it happens:** PDU hook callback is concurrent with test goroutine.
**How to avoid:** Close session first, THEN call `recorder.All()` or `recorder.Sent()`. The Recorder's mutex ensures visibility, but the test should ensure all PDU activity is complete before reading.
**Warning signs:** Flaky assertion failures where the PDU count varies by 1-2 between runs.

### Pitfall 5: MockTarget StatSN for Non-Status PDUs
**What goes wrong:** Multi-PDU Data-In sequences call `NextStatSN()` for every PDU, but only the status-bearing final PDU should carry StatSN.
**Why it happens:** Current handler pattern always calls `tc.NextStatSN()`.
**How to avoid:** In HandleSCSIFunc and future multi-PDU handlers, only call `NextStatSN()` for the final PDU that carries the S-bit. Intermediate Data-In PDUs should have StatSN=0 (target-specific; some targets use 0, others use the current value without incrementing).
**Warning signs:** Initiator logs "unexpected StatSN" or StatSN drift causes window tracking issues.

## Code Examples

### CMDSEQ-01: CmdSN Increment Validation (FFP #1.1)

```go
func TestCmdSN_SequentialIncrement(t *testing.T) {
    // Setup: MockTarget with login, SCSI read, logout handlers.
    // Capture: Create Recorder and pass Hook to WithPDUHook.

    rec := &pducapture.Recorder{}
    tgt, err := testutil.NewMockTarget()
    // ... setup handlers ...

    sess, err := uiscsi.Dial(ctx, tgt.Addr(), uiscsi.WithPDUHook(rec.Hook()))
    // ... error handling ...

    // Send multiple non-immediate SCSI commands.
    for i := 0; i < 5; i++ {
        sess.TestUnitReady(ctx, 0) // Each is a non-immediate SCSI Command
    }
    sess.Close()

    // Assert: Each SCSI Command CmdSN increments by exactly 1.
    cmds := rec.Sent(pdu.OpSCSICommand)
    if len(cmds) < 5 {
        t.Fatalf("expected at least 5 SCSI commands, got %d", len(cmds))
    }
    for i := 1; i < len(cmds); i++ {
        prev := cmds[i-1].Decoded.(*pdu.SCSICommand).CmdSN
        curr := cmds[i].Decoded.(*pdu.SCSICommand).CmdSN
        if curr != prev+1 {
            t.Errorf("CmdSN[%d]=%d, CmdSN[%d]=%d: delta=%d, want 1",
                i-1, prev, i, curr, curr-prev)
        }
    }
}
```

### CMDSEQ-02: Immediate Delivery for Non-TMF (FFP #2.1)

```go
func TestCmdSN_ImmediateDelivery_NonTMF(t *testing.T) {
    // NOP-Out is the primary non-TMF immediate command.
    // Verify: I-bit set, CmdSN == current window CmdSN (not advanced).

    rec := &pducapture.Recorder{}
    // ... setup with HandleNOPOut ...

    // Send a SCSI command (advances CmdSN), then trigger a NOP-Out.
    sess.TestUnitReady(ctx, 0) // CmdSN = N, advances to N+1
    // NOP-Out is sent by keepalive or explicit API; carries CmdSN = N+1 but does NOT advance

    nops := rec.Sent(pdu.OpNOPOut)
    for _, nop := range nops {
        p := nop.Decoded.(*pdu.NOPOut)
        if !p.Header.Immediate {
            t.Error("NOP-Out should have Immediate=true")
        }
    }

    // Verify that SCSI commands after NOP-Out still have sequential CmdSN
    // (NOP-Out did not consume a window slot).
}
```

### CMDSEQ-03: Immediate Delivery for TMF (FFP #2.2)

```go
func TestCmdSN_ImmediateDelivery_TMF(t *testing.T) {
    // TMF requests are always immediate.
    // Verify: I-bit set, CmdSN == current CmdSN (not advanced).

    rec := &pducapture.Recorder{}
    // ... setup with HandleTMF, HandleSCSIRead ...

    // Send SCSI command (CmdSN=N), then LUN Reset TMF, then another SCSI command.
    sess.TestUnitReady(ctx, 0)     // CmdSN = N
    sess.LUNReset(ctx, 0)         // Immediate, carries CmdSN = N+1, does NOT advance
    sess.TestUnitReady(ctx, 0)     // CmdSN = N+1 (NOT N+2)

    tmfs := rec.Sent(pdu.OpTaskMgmtReq)
    for _, tmf := range tmfs {
        p := tmf.Decoded.(*pdu.TaskMgmtReq)
        if !p.Header.Immediate {
            t.Error("TMF should have Immediate=true")
        }
    }

    // Verify SCSI commands before and after TMF have sequential CmdSN
    // with no gap from the TMF.
    cmds := rec.Sent(pdu.OpSCSICommand)
    if len(cmds) >= 2 {
        first := cmds[0].Decoded.(*pdu.SCSICommand).CmdSN
        second := cmds[1].Decoded.(*pdu.SCSICommand).CmdSN
        if second != first+1 {
            t.Errorf("CmdSN gap after TMF: first=%d, second=%d, want delta=1", first, second)
        }
    }
}
```

### HandleSCSIFunc Usage

```go
// Route by CDB opcode with different behavior per command count.
mt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
    expCmdSN, maxCmdSN := mt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
    statSN := tc.NextStatSN()

    switch cmd.CDB[0] {
    case 0x00: // TEST UNIT READY
        return tc.SendPDU(&pdu.SCSIResponse{
            Header:   pdu.Header{Final: true, InitiatorTaskTag: cmd.InitiatorTaskTag},
            Status:   0x00,
            StatSN:   statSN,
            ExpCmdSN: expCmdSN,
            MaxCmdSN: maxCmdSN,
        })
    case 0x28: // READ(10)
        return tc.SendPDU(&pdu.DataIn{
            Header:    pdu.Header{Final: true, InitiatorTaskTag: cmd.InitiatorTaskTag},
            HasStatus: true,
            Status:    0x00,
            StatSN:    statSN,
            ExpCmdSN:  expCmdSN,
            MaxCmdSN:  maxCmdSN,
            DataSN:    0,
            Data:      make([]byte, cmd.ExpectedDataTransferLength),
        })
    default:
        return tc.SendPDU(&pdu.SCSIResponse{
            Header:   pdu.Header{Final: true, InitiatorTaskTag: cmd.InitiatorTaskTag},
            Status:   0x00,
            StatSN:   statSN,
            ExpCmdSN: expCmdSN,
            MaxCmdSN: maxCmdSN,
        })
    }
})
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Manual byte offset extraction in PDU hook (`data[24:28]`) | Type-safe `pdu.DecodeBHS()` + struct field access | Phase 13 (this phase) | All new FFP tests use typed PDU access instead of raw bytes |
| Hardcoded `CmdSN+10` in every handler | `SessionState.Update()` with configurable delta | Phase 13 (this phase) | Enables command window tests (Phase 18) without handler rewrites |
| One handler behavior per SCSI command type | `HandleSCSIFunc` with CDB-opcode routing + call counter | Phase 13 (this phase) | Enables per-command fault injection in later phases |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` (Go 1.25) |
| Config file | None needed (stdlib) |
| Quick run command | `go test -race ./test/conformance/ -run TestCmdSN` |
| Full suite command | `go test -race ./...` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| CMDSEQ-01 | CmdSN increments by 1 for each non-immediate command | conformance | `go test -race ./test/conformance/ -run TestCmdSN_SequentialIncrement -v` | Wave 0 |
| CMDSEQ-02 | Immediate delivery flag + CmdSN for non-TMF (NOP-Out) | conformance | `go test -race ./test/conformance/ -run TestCmdSN_ImmediateDelivery_NonTMF -v` | Wave 0 |
| CMDSEQ-03 | Immediate delivery CmdSN for TMF commands | conformance | `go test -race ./test/conformance/ -run TestCmdSN_ImmediateDelivery_TMF -v` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test -race ./test/pducapture/ ./test/ ./test/conformance/ -run TestCmdSN -v`
- **Per wave merge:** `go test -race ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `test/pducapture/capture.go` -- New package: Recorder, CapturedPDU, Hook(), Filter(), Sent()
- [ ] `test/pducapture/capture_test.go` -- Unit tests for capture framework
- [ ] `test/conformance/cmdseq_test.go` -- CMDSEQ-01, CMDSEQ-02, CMDSEQ-03

## Open Questions

1. **NOP-Out triggering for CMDSEQ-02**
   - What we know: NOP-Out is sent by the keepalive goroutine or as a response to target-initiated NOP-In (TTT != 0xFFFFFFFF). The initiator does not expose a public "send NOP-Out now" API.
   - What is unclear: How to reliably trigger a NOP-Out PDU during a conformance test to capture it.
   - Recommendation: Have MockTarget send a solicited NOP-In (with TTT != 0xFFFFFFFF) to force the initiator to respond with a NOP-Out. This is the standard protocol way to trigger NOP-Out. Alternatively, wait for the keepalive timer to fire (but this is timing-dependent and slower).

2. **Existing handler migration to SessionState**
   - What we know: All existing handlers hardcode `ExpCmdSN: cmd.CmdSN + 1, MaxCmdSN: cmd.CmdSN + 10`. Introducing SessionState is a refactor of these handlers.
   - What is unclear: Should all existing handlers be migrated in Phase 13, or only new handlers use SessionState?
   - Recommendation: Migrate HandleLogin (which sets the initial MaxCmdSN) and add SessionState to all handlers used in Phase 13 tests. Defer migrating HandleSCSIRead/HandleSCSIWrite/HandleSCSIError to when they are next touched -- they still work correctly for existing tests.

3. **Data segment attachment in capture**
   - What we know: `WithPDUHook` at the public API level provides `[]byte` = BHS + DataSegment concatenated. `pdu.DecodeBHS` only decodes the BHS (48 bytes). The data segment is not attached to the decoded PDU.
   - What is unclear: Whether captured PDUs need data segment access for Phase 13 tests (CmdSN tests don't need data).
   - Recommendation: Store the raw bytes in `CapturedPDU.Raw` for future use, but do NOT call `attachDataSegment` in the capture hook for Phase 13. Later phases (DATA-*, R2T-*) can add data segment attachment when needed.

## Sources

### Primary (HIGH confidence)
- `test/target.go` -- MockTarget implementation, handler-based architecture, 710 lines
- `options.go:100` -- `WithPDUHook` public API, captures BHS+DataSegment as `[]byte`
- `internal/pdu/header.go` -- `DecodeBHS()` handles all 18 opcodes, returns typed `pdu.PDU` interface
- `internal/session/session.go:118` -- `Submit()` acquires CmdSN via `window.acquire()`, sets `scsiCmd.CmdSN = cmdSN`
- `internal/session/tmf.go:20-29` -- TMF always `Immediate: true`, uses `window.current()` not `acquire()`
- `internal/session/keepalive.go:51-56` -- NOP-Out always `Immediate: true`, uses `window.current()`
- `internal/session/cmdwindow.go` -- Full CmdSN window implementation with `acquire()` (advances) and `current()` (read-only)
- `.planning/research/FAULT-INJECTION.md` -- MockTarget extension research confirming approach

### Secondary (MEDIUM confidence)
- RFC 7143 Section 3.2.2.1 -- CmdSN rules: non-immediate commands increment by 1; immediate commands carry CmdSN but do not advance it
- RFC 7143 Section 11.5 -- TMF requests are always immediate
- RFC 7143 Section 11.18 -- NOP-Out can be immediate or non-immediate (this initiator always uses immediate)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all stdlib, no external deps, code inspection confirms patterns
- Architecture: HIGH -- capture framework design follows directly from existing WithPDUHook + DecodeBHS APIs
- Pitfalls: HIGH -- based on direct code reading of handler registration, CmdSN windowing, and hook concurrency
- Test patterns: HIGH -- CmdSN behavior verified by reading Submit() and sendTMF() source code

**Research date:** 2026-04-04
**Valid until:** 2026-05-04 (stable -- no external dependency changes, all stdlib)
