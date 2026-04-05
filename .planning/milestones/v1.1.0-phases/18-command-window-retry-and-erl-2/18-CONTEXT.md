# Phase 18: Command Window, Retry, and ERL 2 - Context

**Gathered:** 2026-04-05
**Status:** Ready for planning

<domain>
## Phase Boundary

Command window enforcement conformance tests, command retry wire validation, ExpStatSN gap recovery testing, and ERL 2 connection reassignment with task reassign. Covers 8 requirements: CMDSEQ-04 through CMDSEQ-09 (command window zero/large/size-1, command retry, ExpStatSN gap, MaxCmdSN in SCSI Response) and SESS-07, SESS-08 (ERL 2 connection replacement and task reassign). All production code exists — this phase is purely conformance testing with MockTarget manipulation.

</domain>

<decisions>
## Implementation Decisions

### ERL 2 Testing
- **D-01:** Full E2E testing for SESS-07 and SESS-08. The production code in `internal/session/connreplace.go` (~150 lines) fully implements ERL 2 connection replacement with task reassign. The test matrix note "not implemented" was stale — the code was written in v1.0 Phase 6.
- **D-02:** MockTarget accepts reconnect to its same listener for ERL 2. The initiator's `replaceConnection` dials the same `targetAddr`, so MockTarget's existing TCP listener naturally accepts the second connection. Login handler already works with TSIH matching. Minimal new code needed — no dedicated ERL 2 MockTarget API.

### Command Window Test Mechanics
- **D-03:** Use `HandleSCSIFunc` with MaxCmdSN manipulation in SCSI Response for command window tests (CMDSEQ-04/05/06/09). For zero window: set `MaxCmdSN=ExpCmdSN-1`. For large window: set `MaxCmdSN=ExpCmdSN+255`. For window-of-1: set `MaxCmdSN=ExpCmdSN`. Then send NOP-In to reopen window. No new MockTarget API needed — uses existing `SessionState.Update` override patterns.
- **D-04:** Goroutine + timer pattern to verify the initiator actually blocks on zero window (not just queues). Launch goroutine calling `sess.ReadBlocks`, verify it doesn't return within N ms while window is zero, then send NOP-In with `MaxCmdSN > ExpCmdSN` to open window, verify command completes.

### Command Retry and ExpStatSN Gap
- **D-05:** Reject + retry capture for CMDSEQ-07. Configure ERL>=1. HandleSCSIFunc on callCount==0 sends Reject PDU (Reason=0x09 Invalid PDU Field). On callCount==1 responds normally. Capture both SCSI Command PDUs and verify retry carries original ITT, CDB, CmdSN. Reuses Phase 16 ERR-02 Reject pattern.
- **D-06:** Skip StatSN in SCSI Response for CMDSEQ-08 (ExpStatSN gap detection). HandleSCSIFunc sends SCSI Response with StatSN jumped by N (e.g., from 2 to 10). Verify the initiator detects the gap and takes recovery action (Status SNACK at ERL>=1, or error surfacing at ERL 0).

### Test File Organization
- **D-07:** Three test files in `test/conformance/`:
  - `cmdwindow_test.go` — CMDSEQ-04 (zero window), CMDSEQ-05 (large window), CMDSEQ-06 (window size 1), CMDSEQ-09 (MaxCmdSN in SCSI Response closes window)
  - `retry_test.go` — CMDSEQ-07 (command retry with original fields), CMDSEQ-08 (ExpStatSN gap recovery)
  - `erl2_test.go` — SESS-07 (connection reassignment after drop), SESS-08 (task reassign on new connection)

### Claude's Discretion
- Exact timer durations for block verification in command window tests
- How to trigger the connection drop for ERL 2 tests (close TCP connection, inject error, etc.)
- Whether CMDSEQ-09 is a separate test or a variant of CMDSEQ-04
- How to verify task reassign completion (capture TMF PDU or verify command completion after reassign)
- Plan count, wave structure, and task breakdown across the 8 requirements

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Command Window Implementation
- `internal/session/cmdwindow.go` — cmdWindow struct, acquire (blocks until slot available), update (adjusts ExpCmdSN/MaxCmdSN), serial arithmetic for window bounds
- `internal/session/cmdwindow_test.go` — Unit tests for command window logic (patterns for window manipulation)

### ERL 2 Connection Replacement
- `internal/session/connreplace.go` — replaceConnection: stop pumps, snapshot tasks, dial new connection, login with ISID/TSIH, replace session internals, Logout(reasonCode=2), TMF TASK REASSIGN loop
- `internal/session/connreplace_test.go` — Unit tests for connection replacement

### Recovery and Retry
- `internal/session/recovery.go` — reconnect logic (ERL 0), retry with original Command fields
- `internal/session/recovery_test.go` — Unit tests for reconnect and retry
- `internal/session/session.go` — dispatchLoop, command dispatch, error handling

### Error Injection (Phase 16 patterns)
- `test/conformance/error_test.go` — TestError_SNACKRejectNewCommand (ERR-02) — Reject + retry pattern to reuse
- `test/target.go` — HandleSCSIFunc, HandleSCSIWithStatus, SessionState.Update, SendAsyncMsg

### Test Infrastructure (from Phases 13-17)
- `test/pducapture/capture.go` — PDU capture framework (Recorder, Sent)
- `test/conformance/cmdseq_test.go` — CmdSN conformance test patterns (Phase 13)
- `options.go` — WithPDUHook, WithOperationalOverrides, WithKeepaliveInterval

### Protocol References
- `doc/test_matrix_initiator_ffp.md` — FFP test matrix with coverage status for #3.x, #4.x, #5.x, #7.x, #16.5, #19.5
- `doc/initiator_ffp.pdf` — UNH-IOL FFP test suite specification

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `cmdWindow.acquire()` blocks on zero window — the behavior under test for CMDSEQ-04
- `HandleSCSIFunc` with `callCount` — Reject on first call, normal on second (ERR-02 pattern from Phase 16)
- `pducapture.Recorder` — capture SCSI Command, Reject, TMF PDUs
- `SetNegotiationConfig` with `ErrorRecoveryLevel` pointer — configure ERL 1/2 on target
- `WithOperationalOverrides` — configure ERL on initiator side
- `SendAsyncMsg` — can trigger connection drop (AsyncEvent 2) for ERL 2 tests

### Established Patterns
- ERL=1 configuration on both sides (Phases 14-16)
- HandleSCSIFunc with SessionState.Update for CmdSN tracking (Phase 13)
- time.Sleep(100ms) for async propagation (Phases 14-17)
- Goroutine + context timeout for command execution

### Integration Points
- Three new test files in `test/conformance/`
- May need a helper on MockTarget to send NOP-In with custom MaxCmdSN (to open/close window)
- ERL 2 tests may need MockTarget to handle second TCP connection (login + task reassign TMF)

</code_context>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches within the decisions above.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 18-command-window-retry-and-erl-2*
*Context gathered: 2026-04-05*
