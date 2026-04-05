---
phase: 18-command-window-retry-and-erl-2
plan: 01
subsystem: testing
tags: [iscsi, cmdwindow, flow-control, rfc7143, conformance]

# Dependency graph
requires:
  - phase: 13-pdu-capture-and-mocktarget-extensions
    provides: PDU capture framework and SessionState for CmdSN tracking
provides:
  - Command window conformance tests (zero, large, size-1, MaxCmdSN close)
  - Fixed cmdWindow.acquire zero window detection
  - Fixed cmdWindow.update to allow MaxCmdSN decrease for flow control
affects: [18-command-window-retry-and-erl-2]

# Tech tracking
tech-stack:
  added: []
  patterns: [goroutine+timer blocking verification pattern for zero window tests]

key-files:
  created: [test/conformance/cmdwindow_test.go]
  modified: [internal/session/cmdwindow.go]

key-decisions:
  - "Fixed cmdWindow zero window detection: added windowOpen() helper that checks MaxCmdSN < ExpCmdSN (serial) before InWindow, per RFC 7143 Section 3.2.2.1"
  - "Fixed cmdWindow.update to allow MaxCmdSN to decrease: target flow control requires ability to close window by lowering MaxCmdSN"
  - "LargeWindow test checks CmdSN uniqueness and contiguous range instead of wire ordering (concurrent goroutines produce non-deterministic wire order)"

patterns-established:
  - "Zero window blocking test: goroutine+timer pattern with 300ms blocking check and 5s completion check"
  - "Window-of-one serialization: verify response[i] sequence < command[i+1] sequence via Recorder.All()"

requirements-completed: [CMDSEQ-04, CMDSEQ-05, CMDSEQ-06, CMDSEQ-09]

# Metrics
duration: 7min
completed: 2026-04-05
---

# Phase 18 Plan 01: Command Window Conformance Tests Summary

**4 command window conformance tests (zero/large/size-1/MaxCmdSN close) with cmdWindow bug fixes for RFC 7143 flow control compliance**

## Performance

- **Duration:** 7 min
- **Started:** 2026-04-05T13:14:44Z
- **Completed:** 2026-04-05T13:22:23Z
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments
- TestCmdWindow_ZeroWindow proves initiator blocks on zero window and resumes via NOP-In
- TestCmdWindow_LargeWindow proves 8 concurrent commands through window of 256
- TestCmdWindow_WindowOfOne proves only 1 command at a time with serialized ordering
- TestCmdWindow_MaxCmdSNInResponse proves SCSI Response can close window via MaxCmdSN
- Fixed two RFC 7143 compliance bugs in cmdWindow (zero window detection, MaxCmdSN decrease)

## Task Commits

Each task was committed atomically:

1. **Task 1: Command window zero and large tests** - `a087da3` (feat)
2. **Task 2: Command window size-1 and MaxCmdSN close tests** - `83022f2` (feat)

## Files Created/Modified
- `test/conformance/cmdwindow_test.go` - 4 command window conformance tests (CMDSEQ-04/05/06/09)
- `internal/session/cmdwindow.go` - Fixed zero window detection and MaxCmdSN decrease support

## Decisions Made
- Added windowOpen() helper to cmdWindow that checks MaxCmdSN < ExpCmdSN (serial arithmetic) to detect zero window before calling InWindow. The existing InWindow function treated the range as wrapping, incorrectly allowing CmdSN == lo even when window size was 0.
- Removed monotonic guard from MaxCmdSN in cmdWindow.update. Per RFC 7143, the target can decrease MaxCmdSN to close the command window. The only constraint is the validity check (MaxCmdSN >= ExpCmdSN - 1).
- LargeWindow test verifies CmdSN uniqueness and contiguous range rather than strict wire ordering, since concurrent goroutines produce non-deterministic dispatch order.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Fixed cmdWindow.acquire zero window detection**
- **Found during:** Task 1 (TestCmdWindow_ZeroWindow implementation)
- **Issue:** InWindow(cmdSN, expCmdSN, maxCmdSN) returned true when sn==lo even when MaxCmdSN < ExpCmdSN (zero window), because it treats the range as wrapping. The zero window test could not prove blocking without this fix.
- **Fix:** Added windowOpen() helper that checks serial.LessThan(maxCmdSN, expCmdSN) before InWindow. Used in both fast and slow paths of acquire.
- **Files modified:** internal/session/cmdwindow.go
- **Verification:** All existing cmdwindow unit tests and conformance tests pass; new zero window test correctly blocks.
- **Committed in:** a087da3 (Task 1 commit)

**2. [Rule 3 - Blocking] Fixed cmdWindow.update MaxCmdSN monotonic guard**
- **Found during:** Task 1 (TestCmdWindow_ZeroWindow implementation)
- **Issue:** update() only accepted MaxCmdSN if serial.GreaterThan or equal, preventing the target from closing the window by lowering MaxCmdSN. Both ZeroWindow and MaxCmdSNInResponse tests required this fix.
- **Fix:** Changed MaxCmdSN update to accept any valid value (passes RFC validity check), not just monotonically increasing values. ExpCmdSN remains monotonic.
- **Files modified:** internal/session/cmdwindow.go
- **Verification:** All existing tests pass; MaxCmdSNInResponse test correctly demonstrates window close via SCSI Response.
- **Committed in:** a087da3 (Task 1 commit)

---

**Total deviations:** 2 auto-fixed (2 blocking)
**Impact on plan:** Both fixes are RFC 7143 compliance corrections required for the tests to prove the intended behavior. No scope creep -- the command window flow control is the exact subject of this plan.

## Issues Encountered
None beyond the deviation fixes above.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Command window conformance tests complete, ready for Plan 02 (command retry and ExpStatSN gap tests) and Plan 03 (ERL 2 wire-level tests)
- cmdWindow now correctly implements RFC 7143 zero window and flow control

---
## Self-Check: PASSED

*Phase: 18-command-window-retry-and-erl-2*
*Completed: 2026-04-05*
