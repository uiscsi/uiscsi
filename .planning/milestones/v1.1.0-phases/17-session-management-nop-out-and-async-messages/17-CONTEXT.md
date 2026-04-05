# Phase 17: Session Management, NOP-Out, and Async Messages - Context

**Gathered:** 2026-04-05
**Status:** Ready for planning

<domain>
## Phase Boundary

MockTarget async message injection infrastructure plus initiator conformance tests for session lifecycle scenarios. Covers 9 requirements (SESS-02 deferred): SESS-01, SESS-03 through SESS-06 (logout after async, NOP-Out variants, clean logout), and ASYNC-01 through ASYNC-04 (async message handling for logout request, connection drop, session drop, negotiation request). This phase adds async injection capability to MockTarget, implements ExpStatSN confirmation NOP-Out and renegotiation after AsyncEvent 4 as new production code, and verifies the initiator handles all session management scenarios correctly.

</domain>

<decisions>
## Implementation Decisions

### MockTarget Async Injection API
- **D-01:** Generic `SendAsyncMsg(tc *TargetConn, event uint8, params AsyncParams)` method on MockTarget. Single method keeps the API minimal — tests build specific event codes and parameters inline. Consistent with HandleSCSIFunc pattern where complex scenarios stay in test code. `AsyncParams` struct carries optional fields: Parameter1/2/3, AsyncVCode, Data (sense data for event 0).

### Multi-Connection Session Testing
- **D-02:** Skip SESS-02 (multi-connection session logout after AsyncEvent 1). Multi-connection sessions are not implemented in the library. Mark as deferred. Test the single-connection variant (SESS-01) thoroughly instead.

### ExpStatSN Confirmation NOP-Out
- **D-03:** Implement `sendExpStatSNConfirmation()` in production code (`internal/session/keepalive.go` or new file). This is a NOP-Out with ITT=0xFFFFFFFF, TTT=0xFFFFFFFF, Immediate=true — used purely to confirm ExpStatSN to the target without expecting a response. Small addition, required for SESS-05.

### NOP-Out Wire Field Depth
- **D-04:** Full RFC 7143 Section 11.18 wire field validation for NOP-Out ping response (SESS-03): ITT=0xFFFFFFFF, TTT echoed from NOP-In, Immediate=true, Final=true, CmdSN present, ExpStatSN correct, LUN echoed if present, ping data echoed. Comprehensive field-level assertions, not just key fields.

### Async Reaction Verification Strategy
- **D-05:** Combine PDU capture (pducapture.Recorder) with side-effect verification. For code 1: capture LogoutReq PDU on wire + verify session closes. For code 2: verify reconnect triggered or error surfaced. For code 3: verify session terminated + error from subsequent commands. For code 4: capture Text Request PDU for renegotiation.

### Renegotiation After AsyncEvent 4
- **D-06:** Implement full renegotiation after AsyncEvent code 4. This is a scope expansion beyond pure conformance tests. The initiator should initiate a Text Request exchange to renegotiate operational parameters when it receives AsyncEvent 4. This involves: sending Text Request with renegotiation parameters, processing Text Response, updating session parameters. New production code in `internal/session/async.go` or a new `renegotiate.go` file.

### Test File Organization
- **D-07:** Three test files in `test/conformance/`:
  - `session_test.go` — SESS-01 (logout after AsyncEvent 1) and SESS-06 (clean logout exchange)
  - `nopout_test.go` — SESS-03 (NOP-Out ping response), SESS-04 (NOP-Out ping request), SESS-05 (ExpStatSN confirmation)
  - `async_test.go` — ASYNC-01 through ASYNC-04 (async message handling)

### Claude's Discretion
- Exact signature and fields of `AsyncParams` struct
- Whether `sendExpStatSNConfirmation` is exposed publicly or stays internal
- How renegotiation Text Request parameters are constructed (which keys to renegotiate)
- Whether ASYNC-01 duplicates SESS-01 or tests a distinct aspect
- Plan count and task breakdown across the 9 requirements + 2 production code additions

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Session Management Implementation
- `internal/session/async.go` — AsyncMsg handler with all 5 event codes, dispatchAsyncEvent callback, handleTargetRequestedLogout
- `internal/session/keepalive.go` — NOP-Out ping (initiator-originated and target-initiated), handleUnsolicitedNOPIn
- `internal/session/logout.go` — Logout PDU exchange, CmdSN acquisition, LogoutResp processing
- `internal/session/session.go` — Session lifecycle, dispatchLoop, unsolicited PDU handling

### PDU Definitions
- `internal/pdu/target.go` — AsyncMsg struct (line 371), NOPIn struct (line 7), LogoutResp struct (line 296)
- `internal/pdu/initiator.go` — NOPOut struct (line 8), LogoutReq struct (line 238)

### Test Infrastructure (from Phases 13-16)
- `test/target.go` — MockTarget with HandleLogin, HandleLogout, HandleNOPOut, HandleSCSIFunc, SessionState
- `test/pducapture/capture.go` — PDU capture framework (Recorder, Hook, Sent)
- `test/conformance/error_test.go` — ERR tests pattern (Phase 16, for HandleSCSIFunc + side-effect verification)
- `test/conformance/cmdseq_test.go` — CmdSN conformance test pattern (Phase 13, for NOP-Out handler)
- `options.go` — WithPDUHook, WithAsyncHandler, WithKeepaliveInterval, WithOperationalOverrides

### Protocol References
- `doc/test_matrix_initiator_ffp.md` — FFP test matrix with coverage status for #14.x, #15.x, #17.x, #20.x
- `doc/initiator_ffp.pdf` — UNH-IOL FFP test suite specification

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `pdu.AsyncMsg` with MarshalBHS — ready for target-side injection, all fields exposed
- `HandleNOPOut()` on MockTarget — existing NOP-Out handler that echoes NOP-In, can be used as base
- `pducapture.Recorder` — capture NOP-Out, LogoutReq PDUs sent by initiator
- `WithAsyncHandler` option — register callback to receive AsyncEvent, useful for callback-based assertions
- `handleTargetRequestedLogout` — already implements DefaultTime2Wait sleep + logout + Close for code 1

### Established Patterns
- HandleSCSIFunc for complex per-test scenarios (from Phase 13)
- Recorder.Sent(opcode) for wire-level PDU field assertions (from Phase 13)
- time.Sleep(100ms) for async propagation in tests (from Phases 14-16)
- ERL=1 via SetNegotiationConfig + WithOperationalOverrides (from Phases 14-16)

### Integration Points
- New `SendAsyncMsg` method on MockTarget in `test/target.go`
- New `sendExpStatSNConfirmation()` in `internal/session/` (production code)
- New renegotiation logic in `internal/session/` (production code — Text Request exchange)
- Three new test files in `test/conformance/`

</code_context>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches within the decisions above.

</specifics>

<deferred>
## Deferred Ideas

- **SESS-02** (multi-connection session logout after AsyncEvent 1) — multi-connection sessions not implemented. Defer to a future phase that adds multi-connection support.

</deferred>

---

*Phase: 17-session-management-nop-out-and-async-messages*
*Context gathered: 2026-04-05*
