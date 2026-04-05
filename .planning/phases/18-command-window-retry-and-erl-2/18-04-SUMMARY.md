---
phase: 18-command-window-retry-and-erl-2
plan: 04
subsystem: testing
tags: [iscsi, reject, retry, erl-1, same-connection, conformance, rfc7143]

# Dependency graph
requires:
  - phase: 18-command-window-retry-and-erl-2 plan 02
    provides: TestRetry_RejectCallerReissue pattern, pducapture Reject capture
provides:
  - "CMDSEQ-07: same-connection retry with original ITT, CDB, CmdSN at ERL>=1 (RFC 7143 Section 6.2.1)"
  - "retrySameConnection method on Session for ERL>=1 Reject recovery"
  - "TestRetry_SameConnectionRetry conformance test proving wire-level field identity"
affects: [error-recovery, session, conformance]

# Tech tracking
tech-stack:
  added: []
  patterns: [same-connection-retry-erl1, erl-aware-reject-handler]

key-files:
  created: []
  modified:
    - internal/session/datain.go
    - internal/session/session.go
    - test/conformance/retry_test.go
    - test/conformance/error_test.go

key-decisions:
  - "Reject handler is ERL-aware: ERL>=1 retries same-connection, ERL=0 cancels task"
  - "retrySameConnection reuses original ITT, CDB, CmdSN per RFC 7143 Section 6.2.1"
  - "Updated TestError_SNACKRejectNewCommand to expect same-connection retry at ERL>=1"

patterns-established:
  - "ERL-aware Reject handling: check tk.erl before deciding cancel vs retry"
  - "Same-connection retry: reuse ITT/CmdSN, reset DataSN/offset, re-send command"

requirements-completed: [CMDSEQ-07]

# Metrics
duration: 7min
completed: 2026-04-05
---

# Phase 18 Plan 04: Same-Connection Retry Summary

**Same-connection retry on Reject at ERL>=1 using original ITT, CDB, CmdSN per RFC 7143 Section 6.2.1**

## Performance

- **Duration:** 7 min
- **Started:** 2026-04-05T16:26:22Z
- **Completed:** 2026-04-05T16:33:11Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- Implemented retrySameConnection method that re-sends SCSI commands with original ITT, CDB, and CmdSN
- Made both taskLoop and handleUnsolicited Reject handlers ERL-aware (ERL>=1 retries, ERL=0 cancels)
- Added TestRetry_SameConnectionRetry proving all three fields identical on wire after retry
- Updated TestRetry_RejectCallerReissue to test ERL=0 path explicitly
- Full project test suite passes with -race (35+ tests, zero regressions)

## Task Commits

Each task was committed atomically:

1. **Task 1: Add cmdSN field to task and implement same-connection retry on Reject at ERL>=1** - `672a409` (feat)
2. **Task 2: Add TestRetry_SameConnectionRetry conformance test and update existing test** - `660c70f` (test)

## Files Created/Modified
- `internal/session/datain.go` - Added cmdSN field to task struct for storing original CmdSN
- `internal/session/session.go` - Added retrySameConnection method, ERL-aware Reject handlers in taskLoop and handleUnsolicited
- `test/conformance/retry_test.go` - Added TestRetry_SameConnectionRetry, updated TestRetry_RejectCallerReissue for ERL=0
- `test/conformance/error_test.go` - Updated TestError_SNACKRejectNewCommand for ERL>=1 same-connection retry

## Decisions Made
- Reject handler checks `tk.erl >= 1` to decide retry vs cancel, matching RFC 7143 Section 6.2.1 requirements
- retrySameConnection sends via `s.writeCh` with `select/default` to avoid blocking on full channel
- TestRetry_RejectCallerReissue changed to ERL=0 since ERL=1 now retries transparently

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Updated TestError_SNACKRejectNewCommand for ERL>=1 behavior change**
- **Found during:** Task 2 (conformance test verification)
- **Issue:** TestError_SNACKRejectNewCommand expected first ReadBlocks to fail after Reject at ERL=1, but same-connection retry now makes it succeed transparently
- **Fix:** Updated test to expect ReadBlocks success and verify same-connection retry (same ITT/CmdSN on retry)
- **Files modified:** test/conformance/error_test.go
- **Verification:** Full conformance suite passes with -race
- **Committed in:** 660c70f (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug fix in existing test)
**Impact on plan:** Test update necessary for correctness after behavior change. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- CMDSEQ-07 gap is closed: same-connection retry with original ITT/CDB/CmdSN works at ERL>=1
- Full conformance suite green with -race flag
- Ready for verifier validation

---
*Phase: 18-command-window-retry-and-erl-2*
*Completed: 2026-04-05*
