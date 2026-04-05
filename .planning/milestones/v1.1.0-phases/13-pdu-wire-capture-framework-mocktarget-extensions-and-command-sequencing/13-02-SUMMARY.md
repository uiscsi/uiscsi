---
phase: 13-pdu-wire-capture-framework-mocktarget-extensions-and-command-sequencing
plan: 02
subsystem: testing
tags: [conformance-tests, command-sequencing, cmdsn, immediate-delivery, pdu-capture, iscsi]

# Dependency graph
requires:
  - phase: 13-pdu-wire-capture-framework-mocktarget-extensions-and-command-sequencing
    plan: 01
    provides: PDU capture Recorder, HandleSCSIFunc, SessionState
provides:
  - CMDSEQ-01 conformance test proving CmdSN sequential increment for non-immediate SCSI commands
  - CMDSEQ-02 conformance test proving NOP-Out Immediate flag and no CmdSN advance
  - CMDSEQ-03 conformance test proving TMF Immediate flag and no CmdSN advance
  - Established pattern for wire-level FFP conformance tests using Recorder + HandleSCSIFunc + SessionState
affects: [all-future-ffp-conformance-tests, command-window-tests]

# Tech tracking
tech-stack:
  added: []
  patterns: [solicited-nop-in-for-nop-out-trigger, session-state-aware-nop-handler, cmdseq-conformance-pattern]

key-files:
  created:
    - test/conformance/cmdseq_test.go
  modified: []

key-decisions:
  - "Target sends solicited NOP-In (TTT != 0xFFFFFFFF) to trigger initiator NOP-Out response for CMDSEQ-02"
  - "Custom NOP-Out handler with SessionState.Update(immediate=true) instead of default HandleNOPOut which hardcodes CmdSN+1"
  - "WithKeepaliveInterval(30s) to prevent timer-based NOP-Out interference in all three tests"

patterns-established:
  - "Solicited NOP-In from target (callCount==0 in HandleSCSIFunc) to trigger deterministic NOP-Out capture"
  - "SessionState-aware handlers for all PDU types in CmdSN conformance tests (not just SCSI commands)"
  - "t.Errorf for field assertions (D-09), t.Fatalf only for precondition failures (len checks)"

requirements-completed: [CMDSEQ-01, CMDSEQ-02, CMDSEQ-03]

# Metrics
duration: 3min
completed: 2026-04-05
---

# Phase 13 Plan 02: CmdSN Command Sequencing Conformance Tests Summary

**Three wire-level CmdSN conformance tests proving sequential increment for SCSI commands and Immediate delivery (no CmdSN advance) for NOP-Out and TMF**

## Performance

- **Duration:** 3 min
- **Started:** 2026-04-04T22:28:24Z
- **Completed:** 2026-04-04T22:31:30Z
- **Tasks:** 2
- **Files modified:** 1

## Accomplishments
- CMDSEQ-01: Verifies CmdSN increments by exactly 1 across 5 sequential non-immediate SCSI commands on the wire
- CMDSEQ-02: Verifies NOP-Out carries Immediate=true and does not consume a CmdSN slot (SCSI commands before/after have delta=1)
- CMDSEQ-03: Verifies TMF (LUN Reset) carries Immediate=true and does not consume a CmdSN slot
- All 25 conformance tests pass under go test -race with zero regressions

## Task Commits

Each task was committed atomically:

1. **Task 1: Write CMDSEQ-01 CmdSN sequential increment test** - `775f431` (test)
2. **Task 2: Write CMDSEQ-02 and CMDSEQ-03 immediate delivery tests** - `a2a14fc` (test)

## Files Created/Modified
- `test/conformance/cmdseq_test.go` - Three CmdSN conformance tests: sequential increment, NOP-Out immediate, TMF immediate

## Decisions Made
- Used solicited NOP-In from target (TTT=0x00000001) in HandleSCSIFunc (callCount==0) to trigger deterministic NOP-Out from initiator, instead of relying on keepalive timer which is non-deterministic
- Created custom NOP-Out handler using SessionState.Update(cmdSN, immediate=true) to get correct ExpCmdSN -- the default HandleNOPOut hardcodes CmdSN+1 which incorrectly advances ExpCmdSN for immediate commands, causing command window exhaustion
- All tests use WithKeepaliveInterval(30*time.Second) to prevent timer-based NOP-Out from interfering with deterministic assertions

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed NOP-Out handler ExpCmdSN for immediate commands**
- **Found during:** Task 2 (CMDSEQ-02 test)
- **Issue:** Default HandleNOPOut uses `req.CmdSN + 1` for ExpCmdSN, which incorrectly advances the command window for immediate NOP-Out. This caused the second TestUnitReady to block on "window full" until context timeout.
- **Fix:** Replaced HandleNOPOut with a custom handler that calls `tgt.Session().Update(req.CmdSN, req.Header.Immediate)` -- since NOP-Out is Immediate, Update does not advance ExpCmdSN.
- **Files modified:** test/conformance/cmdseq_test.go (test-local handler, not modifying production HandleNOPOut)
- **Verification:** CMDSEQ-02 passes under go test -race
- **Committed in:** a2a14fc (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Necessary fix to make CMDSEQ-02 work correctly with SessionState. The default HandleNOPOut is still valid for non-SessionState tests.

## Issues Encountered
None beyond the deviation above.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- CmdSN conformance test pattern established for all future FFP tests
- Solicited NOP-In technique available for any test needing deterministic NOP-Out capture
- SessionState-aware handler pattern proven for all immediate and non-immediate PDU types

## Known Stubs
None.

## Self-Check: PASSED
- test/conformance/cmdseq_test.go: FOUND
- Commit 775f431: FOUND
- Commit a2a14fc: FOUND

---
*Phase: 13-pdu-wire-capture-framework-mocktarget-extensions-and-command-sequencing*
*Completed: 2026-04-05*
