# Phase 17: Session Management, NOP-Out, and Async Messages - Research

**Researched:** 2026-04-05
**Domain:** iSCSI session lifecycle, NOP-Out variants, async message handling (RFC 7143 Sections 11.9, 11.14, 11.18, 11.19)
**Confidence:** HIGH

## Summary

Phase 17 covers three interrelated areas of iSCSI session management: (1) logout after async messages, (2) NOP-Out PDU variants (ping response, ping request, ExpStatSN confirmation), and (3) async message event code handling (codes 1-4). The existing production code already handles most of these scenarios -- `handleAsyncMsg` dispatches all five event codes, `handleUnsolicitedNOPIn` responds to target-initiated pings, `keepaliveLoop` sends initiator pings, and `logout()` performs the Logout exchange. Two pieces of production code are missing: `sendExpStatSNConfirmation()` (SESS-05) and renegotiation after AsyncEvent 4 (ASYNC-04/D-06).

The primary work is building the MockTarget `SendAsyncMsg` injection method, implementing the two missing production code features, and writing conformance tests that use PDU capture to validate wire-level field correctness. All PDU types (AsyncMsg, NOPOut, NOPIn, LogoutReq, LogoutResp, TextReq, TextResp) already have complete Marshal/Unmarshal implementations. The test infrastructure from Phases 13-16 (pducapture.Recorder, HandleSCSIFunc, SessionState) is directly reusable.

**Primary recommendation:** Build SendAsyncMsg on MockTarget first (it is the prerequisite for all async tests), then implement the two production code additions (ExpStatSN confirmation NOP-Out + renegotiation), then write the three test files in `test/conformance/`.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Generic `SendAsyncMsg(tc *TargetConn, event uint8, params AsyncParams)` method on MockTarget. Single method, `AsyncParams` struct carries optional fields.
- **D-02:** Skip SESS-02 (multi-connection session logout after AsyncEvent 1). Mark as deferred.
- **D-03:** Implement `sendExpStatSNConfirmation()` in production code. ITT=0xFFFFFFFF, TTT=0xFFFFFFFF, Immediate=true.
- **D-04:** Full RFC 7143 Section 11.18 wire field validation for NOP-Out ping response (SESS-03).
- **D-05:** Combine PDU capture (pducapture.Recorder) with side-effect verification for async reaction tests.
- **D-06:** Implement full renegotiation after AsyncEvent code 4. Send Text Request exchange.
- **D-07:** Three test files: `session_test.go` (SESS-01, SESS-06), `nopout_test.go` (SESS-03, SESS-04, SESS-05), `async_test.go` (ASYNC-01 through ASYNC-04).

### Claude's Discretion
- Exact signature and fields of `AsyncParams` struct
- Whether `sendExpStatSNConfirmation` is exposed publicly or stays internal
- How renegotiation Text Request parameters are constructed (which keys to renegotiate)
- Whether ASYNC-01 duplicates SESS-01 or tests a distinct aspect
- Plan count and task breakdown across the 9 requirements + 2 production code additions

### Deferred Ideas (OUT OF SCOPE)
- **SESS-02** (multi-connection session logout after AsyncEvent 1) -- multi-connection sessions not implemented
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SESS-01 | Logout after AsyncMessage code 1 on single connection (FFP #14.1) | Existing `handleTargetRequestedLogout` + new SendAsyncMsg + PDU capture for LogoutReq field validation |
| SESS-03 | NOP-Out ping response with TTT echo, ITT, I-bit, LUN (FFP #15.1) | Existing `handleUnsolicitedNOPIn` + target sends NOP-In with TTT + PDU capture for field assertions |
| SESS-04 | NOP-Out ping request with valid ITT (FFP #15.2) | Existing `keepaliveLoop` + short keepalive interval + PDU capture |
| SESS-05 | NOP-Out ExpStatSN confirmation variant (FFP #15.3) | NEW production code: `sendExpStatSNConfirmation()` + PDU capture |
| SESS-06 | Clean logout exchange (FFP #17.1) | Existing `Logout()` + SendAsyncMsg to trigger logout + capture LogoutReq/LogoutResp |
| ASYNC-01 | Async Message logout request handling (FFP #20.1) | SendAsyncMsg(code=1) + verify LogoutReq within Parameter3 seconds |
| ASYNC-02 | Async Message connection drop handling (FFP #20.2) | SendAsyncMsg(code=2) + verify disconnect + reconnect attempt |
| ASYNC-03 | Async Message session drop handling (FFP #20.3) | SendAsyncMsg(code=3) + verify all connections closed + session error |
| ASYNC-04 | Async Message negotiation request handling (FFP #20.4) | NEW production code: renegotiation + SendAsyncMsg(code=4) + capture TextReq |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Language:** Go 1.25, stdlib only for production code
- **Dependencies:** Minimal (Bronx Method)
- **Testing:** stdlib `testing` package, table-driven tests, no testify
- **API style:** Go idiomatic with context.Context
- **Standard:** RFC 7143 compliance drives implementation

## Architecture Patterns

### Recommended Project Structure (changes)
```
test/
  target.go           # + SendAsyncMsg method, AsyncParams struct
  conformance/
    session_test.go    # NEW: SESS-01, SESS-06
    nopout_test.go     # NEW: SESS-03, SESS-04, SESS-05
    async_test.go      # NEW: ASYNC-01, ASYNC-02, ASYNC-03, ASYNC-04
internal/session/
  keepalive.go         # + sendExpStatSNConfirmation()
  async.go             # + renegotiation handler for event code 4
  renegotiate.go       # NEW (optional): Text Request renegotiation exchange
```

### Pattern 1: MockTarget Async Injection
**What:** `SendAsyncMsg(tc *TargetConn, event uint8, params AsyncParams)` builds and sends an AsyncMsg PDU to the initiator via an existing TargetConn. [VERIFIED: codebase -- pdu.AsyncMsg.MarshalBHS already exists]
**When to use:** Every async/session test needs this to inject target-initiated events.
**Example:**
```go
// Source: test/target.go pattern from HandleSCSIFunc + pdu.AsyncMsg.MarshalBHS
type AsyncParams struct {
    Parameter1 uint16
    Parameter2 uint16
    Parameter3 uint16
    AsyncVCode uint8
    SenseData  []byte // for event code 0
}

func (mt *MockTarget) SendAsyncMsg(tc *TargetConn, event uint8, params AsyncParams) error {
    async := &pdu.AsyncMsg{
        Header: pdu.Header{
            Final: true,
        },
        StatSN:     tc.NextStatSN(),
        ExpCmdSN:   mt.session.ExpCmdSN(),
        MaxCmdSN:   mt.session.ExpCmdSN() + 10, // or use SessionState
        AsyncEvent: event,
        AsyncVCode: params.AsyncVCode,
        Parameter1: params.Parameter1,
        Parameter2: params.Parameter2,
        Parameter3: params.Parameter3,
    }
    if len(params.SenseData) > 0 {
        async.Data = params.SenseData
        async.Header.DataSegmentLen = uint32(len(params.SenseData))
    }
    return tc.SendPDU(async)
}
```

### Pattern 2: PDU Capture + Side-Effect Verification (D-05)
**What:** Tests use `pducapture.Recorder` to capture PDUs on the wire AND verify behavioral side effects (session closed, reconnect triggered, error surfaced). [VERIFIED: codebase -- established in Phases 13-16]
**When to use:** All tests in this phase.
**Example:**
```go
// Source: test/conformance/error_test.go pattern
rec := &pducapture.Recorder{}
sess, _ := uiscsi.Dial(ctx, tgt.Addr(),
    uiscsi.WithPDUHook(rec.Hook()),
    uiscsi.WithKeepaliveInterval(30*time.Second),
)
// ... trigger async message ...
time.Sleep(200 * time.Millisecond) // async propagation
logouts := rec.Sent(pdu.OpLogoutReq)
// assert field values on logouts[0].Decoded.(*pdu.LogoutReq)
```

### Pattern 3: ExpStatSN Confirmation NOP-Out (D-03)
**What:** A NOP-Out with ITT=0xFFFFFFFF, TTT=0xFFFFFFFF, Immediate=true. No response expected. Used purely to confirm ExpStatSN to the target. CmdSN is NOT advanced. [VERIFIED: FFP #15.3 procedure + RFC 7143 Section 11.18]
**When to use:** After receiving a burst of target PDUs where the initiator wants to confirm its ExpStatSN without initiating a ping.
**Example:**
```go
// Source: RFC 7143 Section 11.18 + existing keepalive.go pattern
func (s *Session) sendExpStatSNConfirmation() error {
    nopOut := &pdu.NOPOut{
        Header: pdu.Header{
            Immediate:        true,
            Final:            true,
            InitiatorTaskTag: 0xFFFFFFFF, // no response expected
        },
        TargetTransferTag: 0xFFFFFFFF,
        CmdSN:             s.window.current(), // carried but NOT advanced
        ExpStatSN:         s.getExpStatSN(),
    }
    bhs, err := nopOut.MarshalBHS()
    if err != nil {
        return fmt.Errorf("encode ExpStatSN NOP-Out: %w", err)
    }
    raw := &transport.RawPDU{BHS: bhs}
    s.stampDigests(raw)
    select {
    case s.writeCh <- raw:
        return nil
    default:
        return fmt.Errorf("write channel full")
    }
}
```

### Pattern 4: Renegotiation After AsyncEvent 4 (D-06)
**What:** When the initiator receives AsyncEvent code 4, it MUST send a Text Request within Parameter3 seconds. The Text Request initiates parameter renegotiation. [VERIFIED: FFP #20.4 + RFC 7143 Section 11.9]
**When to use:** Event code 4 handler in `async.go`.
**Example:**
```go
// Source: internal/session/discovery.go SendTargets pattern (Text Request exchange)
func (s *Session) renegotiate(ctx context.Context) error {
    // Build renegotiation keys -- propose current operational params
    data := login.EncodeTextKV([]login.KeyValue{
        {Key: "MaxRecvDataSegmentLength", Value: strconv.Itoa(int(s.params.MaxRecvDataSegmentLength))},
        {Key: "MaxBurstLength", Value: strconv.Itoa(int(s.params.MaxBurstLength))},
        {Key: "FirstBurstLength", Value: strconv.Itoa(int(s.params.FirstBurstLength))},
    })
    cmdSN, err := s.window.acquire(ctx)
    if err != nil {
        return err
    }
    itt, respCh := s.router.Register()
    textReq := &pdu.TextReq{
        Header: pdu.Header{
            Final:            true,
            InitiatorTaskTag: itt,
            DataSegmentLen:   uint32(len(data)),
        },
        TargetTransferTag: 0xFFFFFFFF,
        CmdSN:             cmdSN,
        ExpStatSN:         s.getExpStatSN(),
        Data:              data,
    }
    // ... marshal, send, wait for TextResp, update params ...
}
```

### Anti-Patterns to Avoid
- **Hardcoding ExpCmdSN/MaxCmdSN in AsyncMsg:** Always use `SessionState.Update()` or `SessionState.ExpCmdSN()` to get correct sequence numbers. Hardcoded values cause CmdSN tracking to diverge.
- **Blocking on SendAsyncMsg from serveConn goroutine:** SendAsyncMsg must be called from the test goroutine (via a stored TargetConn reference), NOT from a handler callback. The serveConn loop is blocking on ReadPDU and cannot concurrently send.
- **Missing time.Sleep for async propagation:** The initiator processes async messages asynchronously in the dispatch loop. Tests must wait for propagation (100-200ms pattern from Phases 14-16).
- **Forgetting HandleLogout in async tests:** When the initiator performs logout after AsyncEvent 1, the target must have a Logout handler registered or the test hangs.

## RFC 7143 Wire Format Details

### NOP-Out Ping Response (SESS-03, FFP #15.1)
Per RFC 7143 Section 11.18, when responding to a target-initiated NOP-In (TTT != 0xFFFFFFFF): [ASSUMED -- based on RFC 7143 training knowledge, verified against FFP #15.1 observable results]

| Field | Required Value |
|-------|---------------|
| ITT | 0xFFFFFFFF (response, not a new task) |
| TTT | Copied from NOP-In TTT |
| Immediate | 1 (I-bit set) |
| Final | 1 (F-bit set) |
| LUN | Copied from NOP-In LUN field |
| CmdSN | Present, NOT advanced (immediate) |
| ExpStatSN | Current expected StatSN |
| DSL | 0 (no ping data in response to solicited NOP-In per test) |

**Critical detail from FFP #15.1:** "Verify that the next command sent by the DUT does not have CmdSN incremented from the value sent in the NOP-Out." This confirms CmdSN is not advanced for this NOP-Out variant. [VERIFIED: FFP document page 64]

### NOP-Out Ping Request (SESS-04, FFP #15.2)
Per RFC 7143 Section 11.18, initiator-originated ping: [ASSUMED -- RFC 7143 training + FFP #15.2]

| Field | Required Value |
|-------|---------------|
| ITT | Valid value (NOT 0xFFFFFFFF) |
| TTT | 0xFFFFFFFF |
| LUN | 0 (per FFP #15.2) |
| I-bit | 1 or 0 (per initiator choice) |
| CmdSN | If I-bit=1, not advanced; if I-bit=0, advanced |

**From FFP #15.2:** The test waits 2:30 with no response to provoke the DUT keepalive timer. Configure `WithKeepaliveInterval` to a short value (e.g., 2s) to trigger this deterministically. [VERIFIED: FFP document page 65]

### NOP-Out ExpStatSN Confirmation (SESS-05, FFP #15.3)
Per RFC 7143 Section 11.18: [ASSUMED -- RFC 7143 training + FFP #15.3]

| Field | Required Value |
|-------|---------------|
| ITT | 0xFFFFFFFF |
| TTT | 0xFFFFFFFF |
| Immediate | 1 (I-bit MUST be 1 since ITT=0xFFFFFFFF) |
| LUN | 0 |
| CmdSN | Present, NOT advanced |

**From FFP #15.3:** "It may not be possible to configure the device to transmit NOP-Out to confirm ExpStatSN. If so this item is Not Testable." The test is optional but we implement it per D-03. [VERIFIED: FFP document page 67]

### AsyncMsg Event Code Parameters
From RFC 7143 Section 11.9 and FFP test procedures: [ASSUMED -- RFC 7143 training, verified against FFP observable results]

| Code | Meaning | Parameter1 | Parameter2 | Parameter3 |
|------|---------|-----------|-----------|-----------|
| 0 | SCSI async event | reserved | reserved | reserved |
| 1 | Target requests logout | reserved | reserved | Time (seconds) to logout |
| 2 | Connection drop | CID of dropped conn | Time2Wait (seconds) | Time2Retain (seconds) |
| 3 | Session drop | reserved | Time2Wait (seconds) | Time2Retain (seconds) |
| 4 | Negotiation request | reserved | reserved | Time (seconds) to send TextReq |

### LogoutReq Field Requirements (SESS-01, SESS-06)
From FFP #14.1 observable results: [VERIFIED: FFP document pages 59-60]

- Reason code: 0 (close session) or 1 (close connection)
- TotalAHSLength MUST be 0
- DataSegmentLength MUST be 0
- ExpStatSN MUST match last ExpStatSN for the connection
- CID valid unless reason is "close session"
- No new iSCSI commands after Logout PDU sent

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| AsyncMsg PDU construction | Manual BHS byte packing for tests | `pdu.AsyncMsg.MarshalBHS()` via `tc.SendPDU()` | Already tested and correct |
| Text KV encoding | Manual null-delimited string building | `login.EncodeTextKV()` / `login.DecodeTextKV()` | Handles escaping, null terminators |
| PDU field assertions | Manual BHS byte extraction | `pducapture.Recorder.Sent()` + type assertion to `*pdu.NOPOut` etc. | Decoded structs give named fields |
| SessionState tracking | Manual ExpCmdSN counters per test | `test.SessionState.Update()` | Handles immediate vs non-immediate correctly |

## Common Pitfalls

### Pitfall 1: SendAsyncMsg Timing Relative to Command Flow
**What goes wrong:** AsyncMsg sent before the initiator has finished login or before a command is in flight -- the test sees no reaction because the session is not in the expected state.
**Why it happens:** The FFP procedures explicitly interleave commands with async injection (e.g., "Wait for WRITE, send R2T, receive Data-Out, THEN send AsyncMsg").
**How to avoid:** For SESS-01/ASYNC-01, use HandleSCSIFunc to synchronize -- inject the AsyncMsg from within the SCSI handler callback AFTER the command exchange is established. Alternatively, use a channel to signal when a command has been received.
**Warning signs:** Test passes sometimes, fails others (race between async injection and session readiness).

### Pitfall 2: LUN Field Not Echoed in NOP-Out Response
**What goes wrong:** The NOP-Out response to a target-initiated NOP-In does not copy the LUN field from the NOP-In.
**Why it happens:** The existing `handleUnsolicitedNOPIn` code copies TTT and ping data, but the LUN is carried in the Header struct which IS copied via the NOP-Out construction. Need to verify the LUN from the NOP-In is explicitly set in the NOP-Out.
**How to avoid:** Looking at the code, `handleUnsolicitedNOPIn` constructs a new NOPOut with `Header{...}` -- the LUN defaults to zero bytes. The NOP-In's LUN from `nopin.Header.LUN` is NOT copied. This is a **production bug** that SESS-03 testing will expose.
**Warning signs:** SESS-03 test fails on LUN field comparison.

### Pitfall 3: Renegotiation Needs Router Registration
**What goes wrong:** The Text Request for renegotiation is sent but the TextResp is never received because no ITT is registered with the router.
**Why it happens:** The renegotiation code must follow the same pattern as `SendTargets` -- allocate ITT, register with router, send TextReq, wait on channel.
**How to avoid:** Follow the `discovery.go` `SendTargets` pattern exactly. Use `router.Register()` (single-use) since renegotiation is a single exchange.
**Warning signs:** Renegotiation hangs with timeout.

### Pitfall 4: DefaultTime2Wait in Logout After AsyncEvent 1
**What goes wrong:** The existing `handleTargetRequestedLogout` sleeps for `DefaultTime2Wait` seconds before sending logout. If DefaultTime2Wait is non-zero, the test must wait for that duration plus async propagation time.
**Why it happens:** The production code correctly waits per RFC, but tests may time out if they don't account for this.
**How to avoid:** Either negotiate DefaultTime2Wait=0 in the test, or set the test timeout high enough. FFP #14.1 uses Parameter3=5 (5 seconds), and #20.1 uses Parameter3=1 (1 second). The production code uses DefaultTime2Wait from session params, NOT Parameter3 from the AsyncMsg. The FFP says initiator MUST logout within Parameter3 seconds -- there may be a discrepancy.
**Warning signs:** SESS-01 test timeout or logout happens too late.

### Pitfall 5: HandleNOPOut Not Using SessionState
**What goes wrong:** The default `HandleNOPOut()` handler hardcodes `ExpCmdSN: req.CmdSN + 1` instead of using `SessionState.Update()`. This may produce incorrect ExpCmdSN values that cause CmdSN tracking issues in combined tests.
**Why it happens:** HandleNOPOut was written before SessionState was introduced in Phase 13.
**How to avoid:** For NOP-Out conformance tests, use a custom NOP-Out handler with `SessionState.Update(cmdSN, immediate=true)` as done in Phase 13's `cmdseq_test.go`.
**Warning signs:** CmdSN tracking drift in tests that mix SCSI commands and NOP-Out.

### Pitfall 6: ASYNC-01 vs SESS-01 Overlap
**What goes wrong:** ASYNC-01 (FFP #20.1) and SESS-01 (FFP #14.1) both test logout after AsyncEvent code 1, leading to duplicate tests.
**Why it happens:** They are different FFP test numbers but test the same behavior from different angles.
**How to avoid:** Per D-07, SESS-01 goes in `session_test.go` and ASYNC-01 goes in `async_test.go`. Make SESS-01 focus on LogoutReq wire field validation (reason code, CID, ExpStatSN, TotalAHSLength=0, DSL=0 per FFP #14.1). Make ASYNC-01 focus on timing (logout within Parameter3 seconds per FFP #20.1) and no-new-commands behavior.
**Warning signs:** Code review flags duplicate coverage.

## Code Examples

### Example 1: SESS-03 NOP-Out Ping Response Test
```go
// Source: Synthesis of existing conformance test patterns + FFP #15.1 procedure
func TestNOPOut_PingResponse(t *testing.T) {
    rec := &pducapture.Recorder{}
    tgt, err := testutil.NewMockTarget()
    if err != nil {
        t.Fatalf("NewMockTarget: %v", err)
    }
    t.Cleanup(func() { tgt.Close() })

    tgt.HandleLogin()
    tgt.HandleLogout()
    tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
        expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
        // Send solicited NOP-In with valid TTT and LUN to provoke NOP-Out response
        nopin := &pdu.NOPIn{
            Header: pdu.Header{
                Final:            true,
                InitiatorTaskTag: 0xFFFFFFFF, // unsolicited
                LUN:              pdu.EncodeSAMLUN(1), // non-zero LUN to test echo
            },
            TargetTransferTag: 0x00000042, // valid TTT, not 0xFFFFFFFF
            StatSN:            tc.NextStatSN(),
            ExpCmdSN:          expCmdSN,
            MaxCmdSN:          maxCmdSN,
        }
        if err := tc.SendPDU(nopin); err != nil {
            return err
        }
        // Now send SCSI response
        resp := &pdu.SCSIResponse{
            Header:   pdu.Header{Final: true, InitiatorTaskTag: cmd.InitiatorTaskTag},
            StatSN:   tc.NextStatSN(),
            ExpCmdSN: expCmdSN,
            MaxCmdSN: maxCmdSN,
        }
        return tc.SendPDU(resp)
    })

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

    // Trigger a SCSI command -- the handler injects NOP-In mid-flow
    if err := sess.TestUnitReady(ctx, 0); err != nil {
        t.Fatalf("TestUnitReady: %v", err)
    }
    time.Sleep(200 * time.Millisecond) // async propagation

    // Verify NOP-Out response
    nopouts := rec.Sent(pdu.OpNOPOut)
    if len(nopouts) == 0 {
        t.Fatal("no NOP-Out sent in response to NOP-In")
    }
    nop := nopouts[0].Decoded.(*pdu.NOPOut)
    if nop.InitiatorTaskTag != 0xFFFFFFFF {
        t.Errorf("ITT: got 0x%08X, want 0xFFFFFFFF", nop.InitiatorTaskTag)
    }
    if nop.TargetTransferTag != 0x00000042 {
        t.Errorf("TTT: got 0x%08X, want 0x00000042", nop.TargetTransferTag)
    }
    if !nop.Header.Immediate {
        t.Error("I-bit not set")
    }
    if nop.Header.LUN != pdu.EncodeSAMLUN(1) {
        t.Errorf("LUN: got %v, want LUN 1", nop.Header.LUN)
    }
}
```

### Example 2: ASYNC-01 Logout Request After AsyncEvent 1
```go
// Source: Synthesis of FFP #20.1 + existing error_test.go pattern
func TestAsync_LogoutRequest(t *testing.T) {
    rec := &pducapture.Recorder{}
    tgt, err := testutil.NewMockTarget()
    if err != nil {
        t.Fatalf("NewMockTarget: %v", err)
    }
    t.Cleanup(func() { tgt.Close() })

    tgt.HandleLogin()
    tgt.HandleLogout()
    injected := make(chan struct{})
    tgt.HandleSCSIFunc(func(tc *testutil.TargetConn, cmd *pdu.SCSICommand, callCount int) error {
        expCmdSN, maxCmdSN := tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)
        // Send SCSI response first
        resp := &pdu.SCSIResponse{
            Header:   pdu.Header{Final: true, InitiatorTaskTag: cmd.InitiatorTaskTag},
            StatSN:   tc.NextStatSN(),
            ExpCmdSN: expCmdSN,
            MaxCmdSN: maxCmdSN,
        }
        if err := tc.SendPDU(resp); err != nil {
            return err
        }
        // Then inject AsyncMsg code 1 with Parameter3=1 (1 second deadline)
        if callCount == 0 {
            if err := tgt.SendAsyncMsg(tc, 1, testutil.AsyncParams{Parameter3: 1}); err != nil {
                return err
            }
            close(injected)
        }
        return nil
    })

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

    _ = sess.TestUnitReady(ctx, 0)
    <-injected
    time.Sleep(2 * time.Second) // wait for logout within Parameter3 deadline

    logouts := rec.Sent(pdu.OpLogoutReq)
    if len(logouts) == 0 {
        t.Fatal("no LogoutReq sent after AsyncEvent code 1")
    }
    lr := logouts[0].Decoded.(*pdu.LogoutReq)
    if lr.ReasonCode != 0 && lr.ReasonCode != 1 {
        t.Errorf("ReasonCode: got %d, want 0 or 1", lr.ReasonCode)
    }
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| No async injection on MockTarget | SendAsyncMsg method | Phase 17 | Enables all async/session tests |
| Event code 4 logged and discarded | Full renegotiation via TextReq | Phase 17 | RFC compliance for parameter renegotiation |
| No ExpStatSN confirmation NOP-Out | sendExpStatSNConfirmation | Phase 17 | Covers FFP #15.3 |

## Production Code Gaps Found

### Gap 1: LUN Not Echoed in NOP-Out Response
**Location:** `internal/session/keepalive.go` lines 120-131 (`handleUnsolicitedNOPIn`)
**Issue:** The NOP-Out constructed in response to a target-initiated NOP-In creates a new Header without copying the LUN field from the incoming NOP-In. Per RFC 7143 Section 11.18 and FFP #15.1, the LUN MUST be copied. [VERIFIED: codebase inspection -- `nopOut.Header.LUN` not set]
**Fix:** Add `LUN: nopin.Header.LUN` to the NOPOut Header construction.
**Impact:** SESS-03 test will fail without this fix.

### Gap 2: handleTargetRequestedLogout Uses DefaultTime2Wait, Not Parameter3
**Location:** `internal/session/async.go` lines 90-106
**Issue:** The function sleeps for `DefaultTime2Wait` seconds before logout. The FFP says the initiator MUST logout within Parameter3 seconds. Parameter3 carries the timeout in the AsyncEvent code 1 message, but the current code ignores it and uses the negotiated DefaultTime2Wait instead. [VERIFIED: codebase line 91 vs FFP #14.1 page 60 + FFP #20.1 page 116]
**Fix:** Pass Parameter3 from the AsyncEvent to `handleTargetRequestedLogout` and use it as the deadline, not DefaultTime2Wait. DefaultTime2Wait is the delay BEFORE logout can start; Parameter3 is the deadline BY WHICH logout must happen.
**Impact:** SESS-01 and ASYNC-01 tests may see logout happening too late or at wrong time.

### Gap 3: Event Code 4 Has No Renegotiation (Known -- D-06)
**Location:** `internal/session/async.go` line 67-68
**Issue:** Comment says "renegotiation is Phase 6+" but was never implemented.
**Fix:** Implement renegotiation per D-06.
**Impact:** ASYNC-04 test requires this.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` (Go 1.25) |
| Config file | None needed (go test) |
| Quick run command | `go test ./test/conformance/ -run 'TestSession\|TestNOPOut\|TestAsync' -count=1 -timeout 60s` |
| Full suite command | `go test ./... -count=1 -timeout 120s` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SESS-01 | Logout after AsyncEvent 1 (single conn) | conformance | `go test ./test/conformance/ -run TestSession_LogoutAfterAsync -count=1` | Wave 0 |
| SESS-03 | NOP-Out ping response field validation | conformance | `go test ./test/conformance/ -run TestNOPOut_PingResponse -count=1` | Wave 0 |
| SESS-04 | NOP-Out ping request field validation | conformance | `go test ./test/conformance/ -run TestNOPOut_PingRequest -count=1` | Wave 0 |
| SESS-05 | NOP-Out ExpStatSN confirmation | conformance | `go test ./test/conformance/ -run TestNOPOut_ExpStatSNConfirmation -count=1` | Wave 0 |
| SESS-06 | Clean logout exchange | conformance | `go test ./test/conformance/ -run TestSession_CleanLogout -count=1` | Wave 0 |
| ASYNC-01 | Async logout request handling | conformance | `go test ./test/conformance/ -run TestAsync_LogoutRequest -count=1` | Wave 0 |
| ASYNC-02 | Async connection drop handling | conformance | `go test ./test/conformance/ -run TestAsync_ConnectionDrop -count=1` | Wave 0 |
| ASYNC-03 | Async session drop handling | conformance | `go test ./test/conformance/ -run TestAsync_SessionDrop -count=1` | Wave 0 |
| ASYNC-04 | Async negotiation request handling | conformance | `go test ./test/conformance/ -run TestAsync_NegotiationRequest -count=1` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./test/conformance/ -run 'TestSession\|TestNOPOut\|TestAsync' -count=1 -timeout 60s`
- **Per wave merge:** `go test ./... -count=1 -timeout 120s`
- **Phase gate:** Full suite green before `/gsd-verify-work`

### Wave 0 Gaps
- [ ] `test/conformance/session_test.go` -- covers SESS-01, SESS-06
- [ ] `test/conformance/nopout_test.go` -- covers SESS-03, SESS-04, SESS-05
- [ ] `test/conformance/async_test.go` -- covers ASYNC-01, ASYNC-02, ASYNC-03, ASYNC-04

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | NOP-Out response to target-initiated NOP-In MUST echo LUN field | RFC Wire Format | Medium -- FFP #15.1 explicitly checks this, production fix required |
| A2 | AsyncEvent code 1 Parameter3 is the deadline (in seconds) by which logout MUST happen | RFC Wire Format | High -- current production code uses DefaultTime2Wait instead; tests may fail |
| A3 | AsyncEvent code 4 requires Text Request renegotiation (not just acknowledgment) | RFC Wire Format | Low -- FFP #20.4 explicitly verifies TextReq is sent |
| A4 | NOP-Out with ITT=0xFFFFFFFF requires I-bit=1 | RFC Wire Format | Low -- FFP #15.1 and #15.3 both check I-bit=1 when ITT=0xFFFFFFFF |

**Note on A1 and A2:** These are based on RFC 7143 training knowledge cross-verified against the FFP test procedures which explicitly test these behaviors. The FFP document is authoritative for test expectations.

## Open Questions

1. **Parameter3 vs DefaultTime2Wait for AsyncEvent 1**
   - What we know: FFP #14.1 says "logout within Parameter3 seconds." Current code uses `DefaultTime2Wait`.
   - What's unclear: RFC 7143 Section 11.9.1 says initiator MUST logout "as early as possible, but no later than Parameter3 seconds." DefaultTime2Wait is the delay before logout CAN start. These are different constraints.
   - Recommendation: The production fix should use `Parameter3` as the deadline and `DefaultTime2Wait` as the initial delay (if DefaultTime2Wait < Parameter3). If DefaultTime2Wait >= Parameter3, skip the wait and logout immediately. The test should use DefaultTime2Wait=0 (or very small) to simplify.

2. **Which parameters to renegotiate in AsyncEvent 4 TextReq**
   - What we know: FFP #20.4 says "Verify that the DUT transmits a Text Request within 3 seconds." Does not specify which keys.
   - What's unclear: Whether the TextReq can be empty (F=1, no data), or must contain operational parameters.
   - Recommendation: Send the three core operational parameters (MaxRecvDataSegmentLength, MaxBurstLength, FirstBurstLength) as a re-proposal of current values. This is the most defensible RFC-compliant approach. The TextReq CAN be empty per RFC (it is just initiating the exchange), but sending parameters is more realistic.

3. **Triggering sendExpStatSNConfirmation deterministically**
   - What we know: FFP #15.3 says "Configure the DUT to send a NOP-Out to confirm ExpStatSN from the target" and "It may not be possible to configure the device."
   - What's unclear: When should the library automatically send this? After every N commands? On demand?
   - Recommendation: Expose it as an internal method and trigger it explicitly in the test via a test-only hook or after a sequence of commands. Do not add an automatic trigger in production code for now -- the FFP acknowledges this may not be testable.

## Sources

### Primary (HIGH confidence)
- UNH-IOL FFP Test Suite v0.1 (`doc/initiator_ffp.pdf`) -- Tests #14.1 (p59), #14.2 (p61), #15.1 (p63), #15.2 (p65), #15.3 (p67), #17.1 (p92), #20.1 (p115), #20.2 (p117), #20.3 (p119), #20.4 (p121)
- Codebase inspection -- `internal/session/async.go`, `internal/session/keepalive.go`, `internal/session/logout.go`, `internal/pdu/target.go`, `internal/pdu/initiator.go`, `test/target.go`, `test/pducapture/capture.go`

### Secondary (MEDIUM confidence)
- RFC 7143 (via WebFetch, partial extraction) -- Section 11.9 async events, Section 11.18 NOP-Out, Section 11.14 Logout

### Tertiary (LOW confidence)
- None

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- stdlib Go, no new dependencies
- Architecture: HIGH -- follows established Phase 13-16 patterns exactly
- Pitfalls: HIGH -- verified against both codebase inspection and FFP test procedures
- Production gaps: HIGH -- found by direct code inspection

**Research date:** 2026-04-05
**Valid until:** 2026-05-05 (stable RFC, stable codebase patterns)
