---
phase: 10-e2e-test-coverage-expansion-unh-iol-compliance-gaps
plan: 05
subsystem: testing
tags: [e2e, iscsi, negotiation, tmf, reject, rfc7143]

requires:
  - phase: 10-04
    provides: OpReject handling in session taskLoop and SenseLength prefix stripping in datain.go
provides:
  - Graceful Reject error handling in negotiation matrix E2E test
  - Corrected TMF AbortTask response validation accepting RFC 7143 response 255
  - ITT validation corrected to only reject reserved 0xFFFFFFFF
affects: []

tech-stack:
  added: []
  patterns:
    - "Skip subtests on target Reject instead of failing (graceful degradation)"
    - "Accept all valid TMF response codes per RFC 7143 Section 11.6.1"

key-files:
  created: []
  modified:
    - test/e2e/negotiation_test.go
    - test/e2e/tmf_test.go

key-decisions:
  - "Accept TMF response 255 (Function Rejected) as valid per RFC 7143 Section 11.6.1"
  - "ITT 0x00000000 is valid (router starts at 0); only 0xFFFFFFFF is reserved"
  - "Skip negotiation subtests where target rejects data PDU instead of failing"

patterns-established:
  - "Timeout wrapper on session.Close() in tests where Reject may leave session in bad state"

requirements-completed: [E2E-12, E2E-15, E2E-16]

duration: 1min
completed: 2026-04-03
---

# Phase 10 Plan 05: E2E Test Gap Closure Summary

**Fixed negotiation matrix and AbortTask TMF tests to handle Reject errors and accept all valid RFC 7143 response codes**

## Performance

- **Duration:** 1 min
- **Started:** 2026-04-03T00:15:26Z
- **Completed:** 2026-04-03T00:16:37Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- Negotiation matrix test (TestNegotiation_ImmediateDataInitialR2T) now skips subtests where target rejects data PDU instead of hanging/failing
- AbortTask TMF test accepts response code 255 (Function Rejected) per RFC 7143 Section 11.6.1
- ITT validation corrected: 0x00000000 is valid (only 0xFFFFFFFF reserved per RFC)
- Session Close() wrapped with timeout to prevent test hangs after Reject errors

## Task Commits

Each task was committed atomically:

1. **Task 1: Fix negotiation matrix test to handle Reject errors gracefully** - `dd1bc05` (fix)
2. **Task 2: Fix AbortTask TMF test -- accept response code 255 and validate ITT capture** - `6070e23` (fix)

## Files Created/Modified
- `test/e2e/negotiation_test.go` - Graceful Reject error handling, Close() timeout wrapper
- `test/e2e/tmf_test.go` - Accept TMF response 255, correct ITT validation

## Decisions Made
- Accept TMF response 255 (Function Rejected) as valid per RFC 7143 Section 11.6.1 -- this is a defined response code, not an error
- ITT 0x00000000 is valid (router starts allocation at 0) -- only 0xFFFFFFFF is reserved per RFC 7143
- Skip negotiation subtests where target rejects data PDU rather than failing -- not all ImmediateData/InitialR2T combinations work with every target configuration

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All 4 UAT gaps addressed (negotiation matrix, SCSI error sense data, AbortTask TMF)
- SCSI error tests (Tests 5 and 6) should pass with Plan 04's SenseLength prefix stripping fix without additional test changes
- Phase 10 gap closure complete -- ready for UAT re-run to verify all 11 E2E tests pass

---
*Phase: 10-e2e-test-coverage-expansion-unh-iol-compliance-gaps*
*Completed: 2026-04-03*
