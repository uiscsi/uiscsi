---
phase: 19-task-management-and-text-negotiation
plan: 02
subsystem: testing
tags: [iscsi, text-request, conformance, rfc7143, renegotiation, sendtargets]

# Dependency graph
requires:
  - phase: 17-async-renegotiation-and-conformance
    provides: renegotiate() and SendTargets with C-bit continuation
provides:
  - Text Request wire-level conformance tests (TEXT-01 through TEXT-06)
affects: []

# Tech tracking
tech-stack:
  added: []
  patterns: [target-side TTT capture for Discover continuation validation]

key-files:
  created:
    - test/conformance/text_test.go
  modified:
    - internal/session/async.go

key-decisions:
  - "TTT continuation test uses uiscsi.Discover (public API) with target-side request capture since SendTargets is internal-only"
  - "Race fix: renegotiate() and applyRenegotiatedParams() now hold s.mu for s.params access"

patterns-established:
  - "pollTextResps helper ensures renegotiation completion before triggering next exchange"

requirements-completed: [TEXT-01, TEXT-02, TEXT-03, TEXT-04, TEXT-05, TEXT-06]

# Metrics
duration: 8min
completed: 2026-04-05
---

# Phase 19 Plan 02: Text Request Wire Conformance Summary

**6 Text Request conformance tests validating opcode, F-bit, ITT uniqueness, TTT handling (initial and continuation), CmdSN/ExpStatSN, and negotiation reset per RFC 7143**

## Performance

- **Duration:** 8 min
- **Started:** 2026-04-05T19:10:22Z
- **Completed:** 2026-04-05T19:18:51Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- 6 conformance tests covering TEXT-01 through TEXT-06 requirements
- All tests pass under -race with no flakiness
- Full conformance suite shows no regressions (58.9s total)
- Fixed pre-existing data race in renegotiate/applyRenegotiatedParams

## Task Commits

Each task was committed atomically:

1. **Task 1: Create Text Request wire conformance tests** - `7198da0` (feat)
2. **Task 2: Verify full test suite passes with race detector** - verification only, no code changes

## Files Created/Modified
- `test/conformance/text_test.go` - 6 Text Request conformance tests with helper functions
- `internal/session/async.go` - Race fix: s.mu protection for s.params in renegotiate/applyRenegotiatedParams

## Decisions Made
- Used `uiscsi.Discover()` for TEXT-04 (TTT continuation) since `SendTargets()` is internal-only; validation done via target-side request capture with mutex-protected slice
- Added `pollTextResps` helper alongside `pollTextReqs` to ensure renegotiation fully completes before triggering the next one, preventing concurrent param access
- `triggerRenegotiationViaAsync` helper with `callCount == triggerOnCall` pattern fires AsyncMsg code 4 on exactly the right SCSI command

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed data race in renegotiate/applyRenegotiatedParams**
- **Found during:** Task 1 (TestText_ITTUniqueness)
- **Issue:** Concurrent renegotiation goroutines race on s.params fields (MaxRecvDataSegmentLength, MaxBurstLength, FirstBurstLength) - one writes in applyRenegotiatedParams while another reads in renegotiate
- **Fix:** Read params under s.mu lock in renegotiate(); hold s.mu in applyRenegotiatedParams()
- **Files modified:** internal/session/async.go
- **Verification:** All tests pass with -race, including 3 sequential renegotiations in TEXT-02
- **Committed in:** 7198da0 (Task 1 commit)

---

**Total deviations:** 1 auto-fixed (1 bug fix)
**Impact on plan:** Race fix essential for correctness under concurrent renegotiation. No scope creep.

## Issues Encountered
None beyond the race fix documented above.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All TEXT requirements complete
- Text wire conformance validated alongside full conformance suite

---
*Phase: 19-task-management-and-text-negotiation*
*Completed: 2026-04-05*
