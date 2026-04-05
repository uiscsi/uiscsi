---
phase: 16-error-injection-and-scsi-error-handling
plan: 02
subsystem: testing
tags: [iscsi, snack, erl, conformance, wire-test, rfc7143]

# Dependency graph
requires:
  - phase: 14-data-transfer-and-r2t-wire-validation
    provides: DataIn A-bit handling, pducapture framework, MockTarget ERL=1 support
provides:
  - SNACK-01 Data/R2T SNACK wire conformance test (Type=0, BegRun, RunLength)
  - SNACK-02 DataACK SNACK wire field depth test (Type=2, BegRun, RunLength, TTT, ITT)
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "DataSN gap simulation via skipped PDU in handler sequence"
    - "BufferOffset on status-only PDU must match expected next offset"

key-files:
  created:
    - test/conformance/snack_test.go
  modified: []

key-decisions:
  - "Status-only final DataIn PDU requires BufferOffset matching cumulative data offset"
  - "SNACK RunLength for single-PDU gap is 1 (gap = din.DataSN - nextDataSN)"

patterns-established:
  - "DataSN gap test pattern: send PDU 0, skip PDU 1, send PDU 2, sleep 100ms, retransmit PDU 1, send final with status"
  - "SNACK field assertion pattern: filter rec.Sent(OpSNACKReq), iterate for matching Type, assert BegRun/RunLength/TTT"

requirements-completed: [SNACK-01, SNACK-02]

# Metrics
duration: 5min
completed: 2026-04-05
---

# Phase 16 Plan 02: SNACK Wire Conformance Tests Summary

**Data/R2T SNACK on DataSN gap (Type=0, BegRun=1, RunLength=1) and DataACK SNACK on A-bit (Type=2, BegRun=2, RunLength=0, TTT=0x00000042) verified at wire level**

## Performance

- **Duration:** 5 min
- **Started:** 2026-04-05T09:34:06Z
- **Completed:** 2026-04-05T09:39:41Z
- **Tasks:** 2
- **Files modified:** 1

## Accomplishments
- SNACK-01: Verified initiator sends Data/R2T SNACK with Type=0, BegRun=1, RunLength=1 when DataSN gap detected
- SNACK-02: Verified DataACK SNACK with Type=2, BegRun=2, RunLength=0, TTT=0x00000042 -- deeper wire field assertions than Phase 14 DATA-07
- Both tests pass under -race with ERL=1 configured on target and initiator

## Task Commits

Each task was committed atomically:

1. **Task 1+2: SNACK wire conformance tests** - `c8c72eb` (test)

## Files Created/Modified
- `test/conformance/snack_test.go` - SNACK-01 DataSN gap test and SNACK-02 DataACK wire field depth test

## Decisions Made
- Status-only final DataIn PDU (DataSN=3) requires BufferOffset=1536 to match the initiator's expected next offset after 3x512 bytes of data
- Combined both tasks into single commit since they share one file and were written together

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] BufferOffset on status-only final DataIn PDU**
- **Found during:** Task 1 (TestSNACK_DataSNGap)
- **Issue:** Final DataSN=3 PDU with HasStatus=true but no data had default BufferOffset=0, causing initiator BufferOffset mismatch error (expected 1536)
- **Fix:** Set BufferOffset=1536 on the status-only final PDU to match cumulative data offset
- **Files modified:** test/conformance/snack_test.go
- **Verification:** Test passes after fix
- **Committed in:** c8c72eb

---

**Total deviations:** 1 auto-fixed (1 bug in test setup)
**Impact on plan:** Necessary for correct test behavior. No scope creep.

## Issues Encountered
- Pre-existing build errors in error_test.go (from plan 16-01) required linter auto-fix of WriteBlocks call signatures before conformance package would compile. These are not caused by this plan's changes.
- TestError_SNACKRejectNewCommand (from plan 16-01) fails with context deadline exceeded -- pre-existing, unrelated to SNACK tests.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- SNACK wire conformance coverage complete for Data/R2T and DataACK types
- Phase 16 plans complete (16-01 error injection + 16-02 SNACK wire tests)

---
*Phase: 16-error-injection-and-scsi-error-handling*
*Completed: 2026-04-05*
