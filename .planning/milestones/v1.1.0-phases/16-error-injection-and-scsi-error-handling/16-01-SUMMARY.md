---
phase: 16-error-injection-and-scsi-error-handling
plan: 01
subsystem: testing
tags: [scsi-error, sense-data, reject-pdu, snack, conformance-tests]

# Dependency graph
requires:
  - phase: 13-conformance-test-framework-and-cmdsn
    provides: HandleSCSIFunc, SessionState.Update, PDU Recorder, MockTarget infrastructure
provides:
  - HandleSCSIWithStatus helper for simple SCSI status code injection
  - ERR-01 through ERR-06 conformance tests covering SCSI error handling
affects: [16-02-PLAN]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - HandleSCSIWithStatus uses SessionState.Update for correct CmdSN tracking
    - SNACK handler registration to drain async SNACK PDUs after Reject

key-files:
  created: []
  modified:
    - test/target.go
    - test/conformance/error_test.go

key-decisions:
  - "HandleSCSIWithStatus uses SessionState.Update instead of hardcoded CmdSN arithmetic for correctness"
  - "ERR-02 SNACK Reject test registers explicit SNACK handler to drain Status SNACK timer PDUs"
  - "ERR-03/ERR-04 use WriteBlocks to trigger write path for unsolicited data error scenarios"

patterns-established:
  - "HandleSCSIWithStatus pattern: convenience method for status-only error injection with proper CmdSN"
  - "SNACK drain pattern: register OpSNACKReq handler when testing Reject to prevent stale SNACK interference"

requirements-completed: [ERR-01, ERR-02, ERR-03, ERR-04, ERR-05, ERR-06]

# Metrics
duration: 7min
completed: 2026-04-05
---

# Phase 16 Plan 01: Error Injection and SCSI Error Handling Summary

**HandleSCSIWithStatus helper and 6 ERR conformance tests covering BUSY, RESERVATION CONFLICT, CHECK CONDITION sense parsing, and SNACK Reject task cancellation with retry**

## Performance

- **Duration:** 7 min
- **Started:** 2026-04-05T09:34:07Z
- **Completed:** 2026-04-05T09:41:27Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- Added HandleSCSIWithStatus convenience helper using SessionState.Update for correct CmdSN tracking
- Implemented 6 conformance tests (ERR-01 through ERR-06) covering all planned SCSI error conditions
- All tests pass with -race, full test suite unaffected

## Task Commits

Each task was committed atomically:

1. **Task 1: Add HandleSCSIWithStatus helper to MockTarget** - `7c87d97` (feat)
2. **Task 2: Implement ERR-01 through ERR-06 conformance tests** - `7f3b6c6` (test)

## Files Created/Modified
- `test/target.go` - Added HandleSCSIWithStatus method using SessionState.Update for CmdSN tracking
- `test/conformance/error_test.go` - Added 6 test functions: TestError_BUSY, TestError_ReservationConflict, TestError_UnexpectedUnsolicited, TestError_NotEnoughUnsolicited, TestError_CRCErrorSense, TestError_SNACKRejectNewCommand

## Decisions Made
- HandleSCSIWithStatus uses `mt.session.Update()` instead of hardcoded CmdSN arithmetic (plan requirement D-01)
- ERR-02 SNACK Reject test registers explicit OpSNACKReq handler to drain Status SNACK timer PDUs that fire after task cancellation (prevents 10s timeout from stale SNACK)
- ERR-03/ERR-04 use WriteBlocks (write path) to exercise unsolicited data error scenarios

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Added SNACK handler to drain async Status SNACK PDUs in ERR-02 test**
- **Found during:** Task 2 (ERR-02 test)
- **Issue:** After Reject cancels the task, the SNACK tail-loss timer (5s) fires and sends a Status SNACK. The mock target had no SNACK handler, causing the unhandled SNACK to sit in the TCP buffer. This prevented the second SCSI command from being processed correctly, leading to a 10s timeout.
- **Fix:** Registered an OpSNACKReq handler that silently consumes stale SNACKs. Also removed HasStatus from the gap DataIn (din2) to avoid StatSN confusion.
- **Files modified:** test/conformance/error_test.go
- **Verification:** Test passes in 0.2s instead of timing out
- **Committed in:** 7f3b6c6 (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug fix)
**Impact on plan:** Fix necessary for test correctness. No scope creep.

## Issues Encountered
None beyond the deviation documented above.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All 6 ERR conformance tests passing, ready for Phase 16 Plan 02 (SNACK-01/SNACK-02)
- HandleSCSIWithStatus helper available for future status-based error injection tests

---
*Phase: 16-error-injection-and-scsi-error-handling*
*Completed: 2026-04-05*
