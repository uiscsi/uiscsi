---
phase: 13-pdu-wire-capture-framework-mocktarget-extensions-and-command-sequencing
plan: 01
subsystem: testing
tags: [pdu-capture, mock-target, command-sequencing, iscsi, test-infrastructure]

# Dependency graph
requires:
  - phase: 07-public-api-conformance-tests-examples-and-documentation
    provides: WithPDUHook API, MockTarget with handler dispatch, conformance test patterns
provides:
  - PDU capture Recorder with Hook/All/Filter/Sent/Received for wire-level assertions
  - MockTarget HandleSCSIFunc for per-command routing with call counter
  - SessionState for stateful ExpCmdSN/MaxCmdSN tracking with configurable delta
affects: [13-02, all-future-ffp-conformance-tests, command-window-tests]

# Tech tracking
tech-stack:
  added: []
  patterns: [pducapture-recorder-hook-pattern, session-state-update-pattern, atomic-call-counter-in-handler]

key-files:
  created:
    - test/pducapture/capture.go
    - test/pducapture/capture_test.go
  modified:
    - test/target.go
    - test/target_test.go

key-decisions:
  - "Recorder uses pdu.DecodeBHS to decode captured bytes into typed PDU structs"
  - "SessionState.Update handles immediate vs non-immediate CmdSN advancement per RFC 7143"
  - "HandleSCSIFunc uses atomic.Int32 for goroutine-safe call counter"
  - "HandleLogin seeds SessionState so ExpCmdSN is correct for first FFP command"

patterns-established:
  - "pducapture.Recorder.Hook() returns WithPDUHook-compatible closure for test capture"
  - "SessionState.Update(cmdSN, immediate) returns (expCmdSN, maxCmdSN) for response PDUs"
  - "HandleSCSIFunc(func(tc, cmd, callCount)) for flexible per-command test routing"

requirements-completed: [CMDSEQ-01, CMDSEQ-02, CMDSEQ-03]

# Metrics
duration: 4min
completed: 2026-04-05
---

# Phase 13 Plan 01: PDU Capture Framework and MockTarget Extensions Summary

**PDU capture Recorder with typed decode via DecodeBHS, MockTarget HandleSCSIFunc with atomic call counter, and SessionState for stateful CmdSN/MaxCmdSN tracking**

## Performance

- **Duration:** 4 min
- **Started:** 2026-04-04T22:12:27Z
- **Completed:** 2026-04-04T22:17:22Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- PDU capture framework in test/pducapture/ with Recorder, CapturedPDU, Hook/All/Filter/Sent/Received
- MockTarget HandleSCSIFunc for per-command routing with 0-based atomic call counter
- SessionState with Update() handling immediate vs non-immediate CmdSN advancement and configurable MaxCmdSN delta
- HandleLogin seeds SessionState so FFP tests get correct ExpCmdSN from the start

## Task Commits

Each task was committed atomically:

1. **Task 1: Create PDU capture framework** - `f7814a9` (feat)
2. **Task 2: Extend MockTarget with HandleSCSIFunc and SessionState** - `911869f` (feat)

## Files Created/Modified
- `test/pducapture/capture.go` - Recorder, CapturedPDU, Hook/All/Filter/Sent/Received methods
- `test/pducapture/capture_test.go` - 5 unit tests for capture framework
- `test/target.go` - SessionState struct, HandleSCSIFunc, Session() accessor, HandleLogin seeding
- `test/target_test.go` - 3 new tests for call counter, Update semantics, delta configuration

## Decisions Made
- Recorder uses pdu.DecodeBHS to decode captured bytes into typed PDU structs for direct field access in assertions
- SessionState.Update handles immediate vs non-immediate CmdSN advancement per RFC 7143 Section 3.2.2.1
- HandleSCSIFunc uses atomic.Int32 for the call counter since handlers may run from multiple goroutines
- HandleLogin seeds SessionState with req.CmdSN so ExpCmdSN is correct for first full-feature phase command
- Existing HandleSCSIRead/HandleSCSIWrite/HandleSCSIError handlers left unchanged (still hardcode CmdSN+1/CmdSN+10)

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- PDU capture framework ready for use in FFP conformance tests (Plan 02)
- HandleSCSIFunc and SessionState ready for command window and sequencing tests
- All existing conformance tests pass (22 tests verified, no regression from HandleLogin change)

---
*Phase: 13-pdu-wire-capture-framework-mocktarget-extensions-and-command-sequencing*
*Completed: 2026-04-05*
