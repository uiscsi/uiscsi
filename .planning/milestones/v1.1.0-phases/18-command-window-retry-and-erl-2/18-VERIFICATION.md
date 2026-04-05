---
phase: 18-command-window-retry-and-erl-2
verified: 2026-04-05T18:53:00Z
status: passed
score: 4/4 must-haves verified
re_verification:
  previous_status: gaps_found
  previous_score: 3/4
  gaps_closed:
    - "Tests verify command retry carries original ITT, CDB, and CmdSN on the wire (CMDSEQ-07 / FFP #4.1)"
  gaps_remaining: []
  regressions: []
---

# Phase 18: Command Window, Retry, and ERL 2 Verification Report

**Phase Goal:** Initiator correctly enforces command window boundaries, retries commands with original fields, recovers from ExpStatSN gaps, and performs ERL 2 connection reassignment with task reassign
**Verified:** 2026-04-05T18:53:00Z
**Status:** passed
**Re-verification:** Yes — after gap closure (Plan 18-04 executed to close CMDSEQ-07 gap)

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Tests verify initiator blocks new commands when MaxCmdSN=ExpCmdSN-1 (zero window) and resumes when window opens, correctly uses large windows, and respects window size of 1 | VERIFIED | TestCmdWindow_ZeroWindow (blocking via goroutine+timer, NOP-In reopens), TestCmdWindow_LargeWindow (8 concurrent cmds with unique contiguous CmdSNs), TestCmdWindow_WindowOfOne (serialized ordering via Seq numbers) — all PASS with -race |
| 2 | Tests verify command retry carries original ITT, CDB, and CmdSN on the wire | VERIFIED | TestRetry_SameConnectionRetry: Reject at ERL=1 triggers `retrySameConnection`; wire captures ITT[1]==ITT[0] (0x00000000), CmdSN[1]==CmdSN[0] (1), CDB identical. Production code in `session.go` adds `retrySameConnection` method using `tk.itt`, `tk.cmdSN`, `tk.cmd.CDB` — all original fields. PASS with -race. |
| 3 | Tests verify ExpStatSN gap detection triggers recovery and MaxCmdSN in SCSI Response correctly closes the command window | VERIFIED | TestRetry_ExpStatSNGap: SNACK timer fires Status SNACK (Type=1) at ERL=1 — PASS. TestCmdWindow_MaxCmdSNInResponse: SCSI Response closes window (delta=-1), third ReadBlocks blocks, NOP-In reopens — PASS with -race |
| 4 | Tests verify ERL 2 connection reassignment after drop and task reassign on the new connection with correct PDU fields | VERIFIED | TestERL2_ConnectionReassignment: Logout(reasonCode=2) captured on wire, fails explicitly if ERL 0 fallback taken — PASS. TestERL2_TaskReassign: TMF TASK REASSIGN (Function=14) captured with ReferencedTaskTag==original ITT — PASS with -race |

**Score:** 4/4 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `test/conformance/cmdwindow_test.go` | Command window conformance tests | VERIFIED | Contains TestCmdWindow_ZeroWindow, TestCmdWindow_LargeWindow, TestCmdWindow_WindowOfOne, TestCmdWindow_MaxCmdSNInResponse; uses pducapture.Recorder and SetMaxCmdSNDelta |
| `test/conformance/retry_test.go` | Command retry and ExpStatSN gap tests | VERIFIED | 461 lines; contains TestRetry_RejectCallerReissue (ERL=0 path), TestRetry_SameConnectionRetry (ERL=1 same-conn retry), TestRetry_ExpStatSNGap; all assertions wired with t.Fatalf |
| `test/conformance/erl2_test.go` | ERL 2 connection reassignment tests | VERIFIED | Contains TestERL2_ConnectionReassignment and TestERL2_TaskReassign; Logout(reasonCode=2) assertion; TMF Function=14 + ReferencedTaskTag verification |
| `internal/session/datain.go` | task struct with cmdSN field for same-connection retry | VERIFIED | Line 21: `cmdSN uint32 // stored for same-connection retry at ERL >= 1 (RFC 7143 Section 6.2.1)` |
| `internal/session/session.go` | Same-connection retry logic: retrySameConnection, ERL-aware Reject handlers | VERIFIED | `retrySameConnection` at line 633 uses `tk.itt`, `tk.cmdSN`, `tk.cmd.CDB`; taskLoop Reject handler checks `tk.erl >= 1` at line 599; handleUnsolicited checks `tk.erl >= 1` at line 503 |
| `internal/session/recovery.go` | ERL dispatch in triggerReconnect | VERIFIED | Reads erl under mutex, dispatches to replaceConnection when erl >= 2 with ERL 0 fallback on failure |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `test/conformance/retry_test.go` | `internal/session/session.go` | Reject at ERL=1 triggers retrySameConnection | WIRED | `TestRetry_SameConnectionRetry` proves ITT[1]==ITT[0] and CmdSN[1]==CmdSN[0] on wire — `retrySameConnection` is called and produces correct output |
| `internal/session/session.go` | `internal/session/datain.go` | `tk.cmdSN` field used in retrySameConnection | WIRED | Line 677 in session.go: `CmdSN: tk.cmdSN` — reads from field added in datain.go |
| `test/conformance/cmdwindow_test.go` | `test/target.go` | `SetMaxCmdSNDelta` | WIRED | Pattern found at multiple lines across all 4 window tests |
| `test/conformance/retry_test.go` | `test/pducapture/capture.go` | `pducapture.Recorder` | WIRED | Recorder used for Sent(pdu.OpSCSICommand) assertions in CMDSEQ-07 and CMDSEQ-08 tests |
| `internal/session/recovery.go` | `internal/session/connreplace.go` | ERL >= 2 dispatch | WIRED | `s.replaceConnection(cause)` called when erl >= 2 |
| `test/conformance/erl2_test.go` | `test/target.go` | `MockTarget` | WIRED | SetNegotiationConfig ErrorRecoveryLevel=2, HandleSCSIFunc tc.Close() on callCount==0 |

### Data-Flow Trace (Level 4)

Not applicable — all artifacts are test files and production session/recovery code. No UI rendering or data presentation layers involved.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Zero window blocks then unblocks | `go test ./test/conformance/ -run TestCmdWindow_ZeroWindow -race -count=1` | PASS (0.50s) | PASS |
| Large window: 8 concurrent commands | `go test ./test/conformance/ -run TestCmdWindow_LargeWindow -race -count=1` | PASS (0.00s) | PASS |
| Window of 1: serialized commands | `go test ./test/conformance/ -run TestCmdWindow_WindowOfOne -race -count=1` | PASS (0.00s) | PASS |
| SCSI Response closes window | `go test ./test/conformance/ -run TestCmdWindow_MaxCmdSNInResponse -race -count=1` | PASS (0.51s) | PASS |
| Same-connection retry: ITT[1]==ITT[0], CmdSN[1]==CmdSN[0] (CMDSEQ-07) | `go test ./test/conformance/ -run TestRetry_SameConnectionRetry -race -count=1` | PASS (0.00s) | PASS |
| Caller-reissue at ERL=0: ITT[1]!=ITT[0] | `go test ./test/conformance/ -run TestRetry_RejectCallerReissue -race -count=1` | PASS (0.21s) | PASS |
| ExpStatSN gap triggers Status SNACK | `go test ./test/conformance/ -run TestRetry_ExpStatSNGap -race -count=1` | PASS (2.01s) | PASS |
| ERL 2 Logout(reasonCode=2) on wire | `go test ./test/conformance/ -run TestERL2_ConnectionReassignment -race -count=1` | PASS (0.11s) | PASS |
| ERL 2 TMF TASK REASSIGN Function=14 | `go test ./test/conformance/ -run TestERL2_TaskReassign -race -count=1` | PASS (0.01s) | PASS |
| Existing session tests unaffected | `go test ./internal/session/ -race -count=1 -timeout 60s` | PASS (13.2s) | PASS |
| Full conformance suite | `go test ./test/conformance/ -race -count=1 -timeout 120s` | PASS (37.1s) | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| CMDSEQ-04 | 18-01 | E2E test validates initiator respects zero command window (MaxCmdSN=ExpCmdSN-1) (FFP #3.1) | SATISFIED | TestCmdWindow_ZeroWindow: goroutine+timer proves blocking, NOP-In reopens, 2 SCSI Commands captured with increasing CmdSN |
| CMDSEQ-05 | 18-01 | E2E test validates initiator uses large command window correctly (FFP #3.2) | SATISFIED | TestCmdWindow_LargeWindow: 8 concurrent ReadBlocks succeed through window delta=255 |
| CMDSEQ-06 | 18-01 | E2E test validates initiator respects command window size of 1 (FFP #3.3) | SATISFIED | TestCmdWindow_WindowOfOne: 3 sequential commands with delta=1 CmdSN, serialized ordering verified |
| CMDSEQ-07 | 18-02 + 18-04 | E2E test validates command retry carries original ITT, CDB, CmdSN (FFP #4.1) | SATISFIED | TestRetry_SameConnectionRetry: Reject at ERL=1 triggers retrySameConnection; wire shows ITT[1]==ITT[0] (0x00000000), CmdSN[1]==CmdSN[0] (1), CDB identical. Production `retrySameConnection` in session.go uses tk.itt, tk.cmdSN, tk.cmd.CDB per RFC 7143 Section 6.2.1. |
| CMDSEQ-08 | 18-02 | E2E test validates ExpStatSN gap detection triggers recovery (FFP #5.1) | SATISFIED | TestRetry_ExpStatSNGap: StatSN jumped by 5, tail loss triggers Status SNACK (Type=1) captured via pducapture within 2s at ERL=1 |
| CMDSEQ-09 | 18-01 | E2E test validates MaxCmdSN in SCSI Response closes command window (FFP #16.5) | SATISFIED | TestCmdWindow_MaxCmdSNInResponse: SCSI Response with delta=-1 closes window, third command blocks 300ms, NOP-In reopens, 3 SCSI Commands captured |
| SESS-07 | 18-03 | E2E test validates ERL 2 connection reassignment after drop (FFP #7.1) | SATISFIED | TestERL2_ConnectionReassignment: Logout(reasonCode=2) captured on wire (ERL 2 discriminating signal), fails explicitly if ERL 0 fallback taken |
| SESS-08 | 18-03 | E2E test validates ERL 2 task reassign on new connection (FFP #19.5) | SATISFIED | TestERL2_TaskReassign: TMF Function=14 with ReferencedTaskTag==original ITT, cross-validated via target-side channel and pducapture |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| None | - | - | - | No anti-patterns found in 18-04 gap closure files |

The TODO(conformance) comment that appeared in the previous verification has been removed — the gap it documented (same-connection retry not implemented) is now closed.

### Human Verification Required

No items require human verification. All behaviors are verified programmatically via automated conformance tests with wire-level PDU capture.

### Re-verification Summary

The CMDSEQ-07 gap identified in the initial verification is now closed. Plan 18-04 implemented:

1. **Production code**: `retrySameConnection` method added to `*Session` in `session.go`. Uses `tk.itt` (original ITT), `tk.cmdSN` (original CmdSN, stored in new `datain.go` field), and `tk.cmd.CDB` (original CDB). The taskLoop and handleUnsolicited Reject handlers are now ERL-aware — at ERL>=1 they call `retrySameConnection` instead of canceling the task.

2. **Conformance test**: `TestRetry_SameConnectionRetry` in `retry_test.go` proves all three fields are identical on the wire after retry. Wire capture shows `ITT: first=0x00000000, second=0x00000000 (same)` and `CmdSN: first=1, second=1 (same)`. ReadBlocks succeeds transparently (no error returned to caller).

3. **Regression check**: `TestRetry_RejectCallerReissue` updated to use ERL=0 where caller-reissue behavior is correct. `TestError_SNACKRejectNewCommand` in `error_test.go` updated to expect same-connection retry at ERL=1. Full conformance suite (37s, -race) and internal/session tests (13s, -race) pass with zero failures.

All 8 phase requirements (CMDSEQ-04/05/06/07/08/09, SESS-07/08) are now satisfied. The ROADMAP success criterion "Tests verify command retry carries original ITT, CDB, and CmdSN on the wire" is met by `TestRetry_SameConnectionRetry` passing with wire-level ITT==ITT and CmdSN==CmdSN assertions.

---

_Verified: 2026-04-05T18:53:00Z_
_Verifier: Claude (gsd-verifier)_
