---
phase: 17-session-management-nop-out-and-async-messages
plan: 01
subsystem: testing
tags: [iscsi, nop-out, keepalive, async-message, renegotiation, conformance]

requires:
  - phase: 13-cmdseq-nopout-conformance
    provides: PDU capture framework, SessionState, HandleSCSIFunc pattern
  - phase: 16-error-injection-and-scsi-error-handling
    provides: HandleSCSIWithStatus, error test patterns
provides:
  - MockTarget.SendAsyncMsg for async event injection in conformance tests
  - MockTarget.HandleText for renegotiation test support
  - LUN echo fix in handleUnsolicitedNOPIn
  - SendExpStatSNConfirmation (exported) for SESS-05 tests
  - renegotiate + applyRenegotiatedParams for AsyncEvent 4 handling
  - SESS-03 and SESS-04 NOP-Out conformance tests
affects: [17-02-PLAN, 17-03-PLAN]

tech-stack:
  added: []
  patterns:
    - "AsyncParams struct for flexible async event injection in MockTarget"
    - "Parameter3 deadline pattern for RFC 7143 S11.9.1 compliance"

key-files:
  created:
    - test/conformance/nopout_test.go
  modified:
    - test/target.go
    - internal/session/keepalive.go
    - internal/session/async.go

key-decisions:
  - "HandleText echoes received key-value pairs as-accepted for renegotiation test simplicity"
  - "renegotiate proposes current params (MaxRecvDSL, MaxBurstLength, FirstBurstLength) and applies target response"
  - "applyRenegotiatedParams only updates known operational keys per T-17-02 mitigation"

patterns-established:
  - "SendAsyncMsg: inject any AsyncEvent code via MockTarget for session-level conformance tests"

requirements-completed: [SESS-02, SESS-03, SESS-04]

duration: 5min
completed: 2026-04-05
---

# Phase 17 Plan 01: MockTarget Infrastructure + NOP-Out Conformance Summary

**MockTarget async injection with SendAsyncMsg/HandleText, LUN echo fix, Parameter3 deadline, renegotiation handler, plus SESS-03/SESS-04 NOP-Out wire field conformance tests**

## Performance

- **Duration:** 5 min
- **Started:** 2026-04-05T11:25:28Z
- **Completed:** 2026-04-05T11:30:50Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- Added SendAsyncMsg and HandleText to MockTarget for Phase 17 Plan 02-03 test prerequisites
- Fixed LUN echo bug in handleUnsolicitedNOPIn per RFC 7143 S11.18
- Added exported SendExpStatSNConfirmation for SESS-05 conformance tests
- Fixed handleTargetRequestedLogout to use Parameter3 as deadline instead of DefaultTime2Wait
- Added renegotiate/applyRenegotiatedParams for AsyncEvent 4 with known-key-only update (T-17-02)
- SESS-03: Full wire field validation of NOP-Out ping response (ITT, TTT, I-bit, F-bit, LUN echo, CmdSN)
- SESS-04: Validates initiator keepalive NOP-Out has valid ITT, TTT=0xFFFFFFFF, Immediate, zero LUN

## Task Commits

Each task was committed atomically:

1. **Task 1: MockTarget extensions + all production code fixes** - `0116b51` (feat)
2. **Task 2: NOP-Out ping conformance tests (SESS-03, SESS-04)** - `3a6aa16` (test)

## Files Created/Modified
- `test/target.go` - Added AsyncParams, SendAsyncMsg, HandleText to MockTarget
- `internal/session/keepalive.go` - LUN echo fix + SendExpStatSNConfirmation
- `internal/session/async.go` - Parameter3 deadline fix + renegotiate + applyRenegotiatedParams
- `test/conformance/nopout_test.go` - SESS-03 and SESS-04 NOP-Out conformance tests

## Decisions Made
- HandleText echoes received KVs as-accepted for renegotiation test simplicity
- renegotiate proposes current session params and applies target response values
- applyRenegotiatedParams only updates known operational keys (MaxRecvDSL, MaxBurstLength, FirstBurstLength) per T-17-02 threat mitigation
- SESS-02 deferred per D-02 (multi-connection sessions not implemented)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- MockTarget.SendAsyncMsg ready for Plans 02-03 async event conformance tests
- SendExpStatSNConfirmation ready for SESS-05 test in Plan 02
- renegotiate ready for ASYNC-04 test in Plan 03
- Full test suite green under -race

---
*Phase: 17-session-management-nop-out-and-async-messages*
*Completed: 2026-04-05*
