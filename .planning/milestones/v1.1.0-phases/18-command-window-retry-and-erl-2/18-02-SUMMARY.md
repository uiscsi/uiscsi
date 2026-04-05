---
phase: 18-command-window-retry-and-erl-2
plan: 02
subsystem: testing
tags: [iscsi, snack, reject, erl-1, conformance, pducapture]

# Dependency graph
requires:
  - phase: 13-pdu-capture-and-mock-extensions
    provides: pducapture.Recorder and MockTarget HandleSCSIFunc
  - phase: 18-command-window-retry-and-erl-2 plan 01
    provides: command window conformance test patterns
provides:
  - "CMDSEQ-07: Reject + caller reissue conformance test proving new ITT/CmdSN after Reject at ERL=1"
  - "CMDSEQ-08: ExpStatSN gap + SNACK timer tail loss conformance test proving Status SNACK fires at ERL=1"
  - "Public WithSNACKTimeout option for configuring SNACK timer duration"
  - "Bug fix: task.snackTimeout wired from session config (was zero, timer never started)"
affects: [error-recovery, snack, session]

# Tech tracking
tech-stack:
  added: []
  patterns: [tail-loss-snack-timer-test, reject-reissue-conformance-pattern]

key-files:
  created:
    - test/conformance/retry_test.go
  modified:
    - internal/session/session.go
    - options.go

key-decisions:
  - "Reject at ERL=1 cancels task; caller re-issues with new ITT/CmdSN -- test documents this is not same-connection retry per FFP #4.1"
  - "Tail loss scenario triggers SNACK timer: send partial Data-In, never send final, timer fires Status SNACK"
  - "Wired task.snackTimeout from session config -- was zero (dead code), now active at ERL >= 1"
  - "Exposed WithSNACKTimeout in public API for test configurability (500ms in tests vs 5s default)"

patterns-established:
  - "Reject + reissue test pattern: HandleSCSIFunc callCount==0 sends Reject, callCount>=1 responds normally"
  - "Tail loss test pattern: send partial Data-In without status to start SNACK timer, verify Status SNACK via pducapture"

requirements-completed: [CMDSEQ-07, CMDSEQ-08]

# Metrics
duration: 9min
completed: 2026-04-05
---

# Phase 18 Plan 02: Command Retry and ExpStatSN Gap Summary

**Reject-reissue and SNACK timer tail loss conformance tests proving ERL=1 command retry and status recovery behavior**

## Performance

- **Duration:** 9 min
- **Started:** 2026-04-05T13:10:51Z
- **Completed:** 2026-04-05T13:19:56Z
- **Tasks:** 2
- **Files modified:** 3

## Accomplishments
- TestRetry_RejectCallerReissue (CMDSEQ-07): proves Reject at ERL=1 cancels task, caller re-issues with new ITT/CmdSN but identical CDB
- TestRetry_ExpStatSNGap (CMDSEQ-08): proves Status SNACK (Type=1) fires on tail loss after StatSN gap at ERL=1
- Fixed bug where task.snackTimeout was never wired from session config, making SNACK timer dead code in production Submit path
- Exposed WithSNACKTimeout in public API for configurable SNACK timer duration

## Task Commits

Each task was committed atomically:

1. **Task 1: Reject + caller reissue test (CMDSEQ-07)** - `a33e8e7` (test)
2. **Task 2: ExpStatSN gap recovery test (CMDSEQ-08)** - `d262d2b` (test)

## Files Created/Modified
- `test/conformance/retry_test.go` - TestRetry_RejectCallerReissue and TestRetry_ExpStatSNGap conformance tests
- `internal/session/session.go` - Wire task.snackTimeout from session config in Submit
- `options.go` - Add public WithSNACKTimeout option

## Decisions Made
- Reject at ERL=1 cancels the task; the caller re-issues a new command. This is NOT same-connection retry per FFP #4.1 (documented with TODO in test). The production code in recovery.go retryTasks always allocates new ITT/CmdSN.
- For tail loss testing, send partial Data-In (one segment without status) to start the SNACK timer, then withhold the final response. Timer fires after configured timeout and sends Status SNACK.
- WithSNACKTimeout exposed publicly to allow tests to use short timeouts (500ms) instead of the 5s default, keeping test execution fast.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Wired task.snackTimeout from session config**
- **Found during:** Task 2 (ExpStatSN gap recovery test)
- **Issue:** task.snackTimeout was never assigned from session config in Submit(). The field defaulted to zero, and the guard `if t.erl >= 1 && t.snackTimeout > 0` was always false, making the SNACK timer dead code for tasks created through the public API.
- **Fix:** Added `tk.snackTimeout = s.cfg.snackTimeout` in Submit after the ERL fields assignment.
- **Files modified:** internal/session/session.go
- **Verification:** TestRetry_ExpStatSNGap now successfully captures Status SNACK on wire. All existing tests pass.
- **Committed in:** d262d2b (Task 2 commit)

**2. [Rule 3 - Blocking] Exposed WithSNACKTimeout in public API**
- **Found during:** Task 2 (ExpStatSN gap recovery test)
- **Issue:** WithSNACKTimeout existed only as internal session option. The conformance test (package conformance_test) could not set a short SNACK timeout for fast test execution.
- **Fix:** Added public WithSNACKTimeout(d time.Duration) Option in options.go, wrapping the internal session.WithSNACKTimeout.
- **Files modified:** options.go
- **Verification:** Test uses WithSNACKTimeout(500ms) successfully.
- **Committed in:** d262d2b (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (1 bug, 1 blocking)
**Impact on plan:** Both auto-fixes were essential for the test to work. The snackTimeout bug was real production code missing a wire-up. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Retry and SNACK timer conformance tests complete
- Ready for Plan 03 (ERL 2 connection reassignment tests)

---
*Phase: 18-command-window-retry-and-erl-2*
*Completed: 2026-04-05*

## Self-Check: PASSED
- All files exist: retry_test.go, session.go, options.go, 18-02-SUMMARY.md
- All commits found: a33e8e7, d262d2b
