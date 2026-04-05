# Phase 19: Task Management and Text Negotiation - Context

**Gathered:** 2026-04-05
**Status:** Ready for planning

<domain>
## Phase Boundary

Wire-level conformance tests for TMF PDU fields (CmdSN, LUN encoding, RefCmdSN, Abort Task Set behavior) and Text Request negotiation (ITT uniqueness, TTT continuation, negotiation reset). All 12 requirements (TMF-01 through TMF-06, TEXT-01 through TEXT-06) are conformance tests using the established MockTarget + pducapture pattern.

</domain>

<decisions>
## Implementation Decisions

### TMF Wire Test Mechanics
- **D-01:** Stall in HandleSCSIFunc to create in-flight tasks for TMF targeting. HandleSCSIFunc on callCount==0 blocks (channel wait or sleep) so the SCSI command stays in-flight. Test sends AbortTask/AbortTaskSet with that ITT, then unblocks the handler. Proven pattern from Phase 18 ERL tests.
- **D-02:** Test multiple LUN encoding formats for TMF-02: flat space, peripheral device, and extended LUN formats per SAM-5. Verify the initiator encodes the LUN field correctly in the TMF PDU for each format.
- **D-03:** RefCmdSN verification (TMF-03) scoped to AbortTask only. AbortTask is the only TMF that references a specific task by ITT+RefCmdSN. Other TMFs (LUNReset, AbortTaskSet) don't carry RefCmdSN per RFC 7143 Section 11.5.1.

### Text Request Negotiation
- **D-04:** Sequential Renegotiate calls for ITT uniqueness (TEXT-02). Call sess.Renegotiate() multiple times, capture all Text Request PDUs via pducapture, verify all ITTs are distinct. Text negotiation is inherently serial per RFC 7143.
- **D-05:** HandleText returns partial response with Continue=true and non-0xFFFFFFFF TTT for TTT continuation (TEXT-04). Initiator echoes TTT in next Text Request. Handler sends final response with TTT=0xFFFFFFFF. Reuses existing HandleText pattern from Phase 17.
- **D-06:** Negotiation reset (TEXT-06) means verifying the initiator can start a fresh text exchange after a completed one. After a complete Text Request/Response exchange (TTT=0xFFFFFFFF), verify a new Text Request uses a new ITT and TTT=0xFFFFFFFF (no stale state from prior exchange).

### Abort Task Set Concurrency
- **D-07:** Goroutine + timer pattern to prove the initiator blocks new commands while AbortTaskSet is in flight (TMF-05). Same pattern as Phase 18 zero-window test: launch ReadBlocks in goroutine, verify it doesn't complete within N ms while AbortTaskSet response is pending, then send TMF Response, verify ReadBlocks completes.
- **D-08:** Immediate TMF Response for TMF-06 (response after tasks cleared). HandleTMF sends TMF Response immediately. The test verifies that by the time the initiator receives the response, all prior in-flight tasks on that LUN have been canceled (check task error channels). Tests initiator behavior, not target behavior.

### Test File Organization
- **D-09:** Two test files by domain: `test/conformance/tmf_test.go` (TMF-01 through TMF-06) and `test/conformance/text_test.go` (TEXT-01 through TEXT-06). Follows the one-file-per-domain pattern from Phases 14-18.

### Claude's Discretion
- Exact timer durations for blocking proof tests (suggested 300ms based on Phase 18 precedent)
- HandleTMF response codes and field values for non-AbortTaskSet TMFs
- Text Request key-value content for negotiation tests

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Protocol Specification
- `doc/initiator_ffp.pdf` — UNH-IOL iSCSI Initiator Full Feature Phase test definitions (FFP #18.x for Text, FFP #19.x for TMF)

### Production Code
- `internal/session/tmf.go` — sendTMF, AbortTask, AbortTaskSet, ClearTaskSet, LUNReset, TargetWarmReset, TargetColdReset
- `internal/session/discovery.go` — SendTargets and text exchange implementation (Renegotiate)
- `internal/pdu/initiator.go` — TaskMgmtReq, TextReq PDU structures
- `internal/pdu/target.go` — TaskMgmtResp, TextResp PDU structures

### Test Infrastructure
- `test/target.go` — MockTarget with HandleTMF(), HandleText(), HandleSCSIFunc(), SessionState
- `test/pducapture/capture.go` — pducapture.Recorder for wire-level PDU assertions
- `test/conformance/erl2_test.go` — TASK REASSIGN test pattern (Phase 18, stall + TMF targeting)
- `test/conformance/cmdwindow_test.go` — goroutine + timer blocking proof pattern (Phase 18)

### Prior Phase Context
- `.planning/phases/17-session-management-nop-out-and-async-messages/17-CONTEXT.md` — HandleText pattern decisions
- `.planning/phases/18-command-window-retry-and-erl-2/18-CONTEXT.md` — HandleSCSIFunc stall + blocking proof patterns

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `HandleTMF()` on MockTarget: registers TMF handler, sends TMF Response with configurable response code
- `HandleText()` on MockTarget: registers Text handler, echoes back key-values
- `SessionState.Update()`: tracks CmdSN/MaxCmdSN for correct TMF and Text Response fields
- `pducapture.Recorder`: captures all PDUs for wire-level field assertions
- `sess.AbortTask()`, `sess.AbortTaskSet()`, `sess.LUNReset()`: public TMF API
- `sess.Renegotiate()`: public Text negotiation API

### Established Patterns
- Conformance tests use external package (`conformance_test`) with public API only
- HandleSCSIFunc with callCount branching for per-invocation behavior
- goroutine + timer pattern for blocking proof (tested in Phase 18 cmdwindow)
- pducapture `Sent()` and `Received()` for directional PDU queries

### Integration Points
- New test files wire into `test/conformance/` package
- TMF tests use existing `sess.AbortTask()` etc. from public API
- Text tests use existing `sess.Renegotiate()` from public API

</code_context>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches based on established patterns from Phases 13-18.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 19-task-management-and-text-negotiation*
*Context gathered: 2026-04-05*
