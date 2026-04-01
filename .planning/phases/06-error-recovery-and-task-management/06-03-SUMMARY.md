---
phase: 06-error-recovery-and-task-management
plan: 03
subsystem: session
tags: [snack, erl1, erl2, connection-replacement, task-reassignment, iscsi, rfc7143]

requires:
  - phase: 06-error-recovery-and-task-management
    provides: TMF methods (sendTMF, AbortTask), SNACK/TMF constants, FaultConn, sessionConfig recovery options
provides:
  - ERL 1 SNACK-based retransmission with DataSN gap detection and tail loss timeout
  - ERL 2 connection replacement with Logout reasonCode=2 and TMF TASK REASSIGN
  - WithTSIH login option for session reinstatement
  - WithReconnectInfo session option for reconnect context
  - Race-safe startPumps pattern capturing conn/channels locally
affects: [06-04, error-recovery, connection-management]

tech-stack:
  added: []
  patterns: [SNACK gap detection with pending PDU buffer, per-task timeout for tail loss, startPumps local capture for race-safe replacement]

key-files:
  created:
    - internal/session/snack.go
    - internal/session/snack_test.go
    - internal/session/connreplace.go
    - internal/session/connreplace_test.go
  modified:
    - internal/session/datain.go
    - internal/session/session.go
    - internal/session/types.go
    - internal/login/login.go

key-decisions:
  - "SNACK uses task's own ITT per RFC 7143 Section 11.16 (Pitfall 4)"
  - "Per-task SNACK timeout fires Status SNACK for tail loss detection (D-06)"
  - "startPumps captures conn/channels locally to prevent race during connection replacement"
  - "Logout failure on ERL 2 replacement is non-fatal (target may have already cleaned up)"
  - "WithTSIH added as blocking dependency for ERL 2 (Rule 3 auto-fix)"

patterns-established:
  - "SNACK pattern: gap detected -> buffer OOO PDU, send SNACK, drain on fill"
  - "Per-task timeout: AfterFunc resets on each Data-In, fires Status SNACK for tail loss"
  - "Connection replacement: cancel+wait -> dial -> login(ISID+TSIH) -> replace -> Logout(2) -> TASK REASSIGN"
  - "startPumps: capture conn/writeCh/unsolCh/done locally before goroutine start"

requirements-completed: [ERL-02, ERL-03, TEST-05]

duration: 9min
completed: 2026-04-01
---

# Phase 06 Plan 03: ERL 1/2 SNACK and Connection Replacement Summary

**SNACK-based PDU retransmission for within-connection recovery plus ERL 2 connection replacement with Logout reasonCode=2 and TMF TASK REASSIGN**

## Performance

- **Duration:** 9 min
- **Started:** 2026-04-01T16:17:56Z
- **Completed:** 2026-04-01T16:27:05Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- ERL 1 SNACK implementation: DataSN gaps trigger SNACK Request with correct BegRun/RunLength, out-of-order PDUs buffered and drained after gap fill, per-task SNACK timeout fires Status SNACK for tail loss detection
- ERL 2 connection replacement: failed connection dropped, new TCP established with same ISID+TSIH, Logout with reasonCode=2 on new connection for old CID cleanup, TASK REASSIGN for each in-flight task
- Race-safe pump goroutine management via startPumps pattern that captures conn/channels locally
- 13 total tests (7 SNACK, 6 ERL 2) all passing with -race

## Task Commits

Each task was committed atomically:

1. **Task 1: ERL 1 SNACK implementation** - `54fe5b2` (feat)
2. **Task 2: ERL 2 connection replacement** - `fe2b7f2` (feat)

## Files Created/Modified
- `internal/session/snack.go` - SNACK sending, pending PDU buffer, per-task timeout
- `internal/session/snack_test.go` - 7 subtests: gap detection, gap fill, ERL 0 fatal, ITT correctness, multiple gaps, timeout tail loss, timeout reset
- `internal/session/connreplace.go` - replaceConnection with Logout reasonCode=2 and TMF TASK REASSIGN
- `internal/session/connreplace_test.go` - 6 tests: basic replacement, logout_reasoncode2, logout failure nonfatal, reassign failure, multiple tasks, data integrity
- `internal/session/datain.go` - Added ERL/SNACK fields to task struct, handleDataIn branches on ERL
- `internal/session/session.go` - Added reconnect fields, startPumps for race-safe goroutine management
- `internal/session/types.go` - WithReconnectInfo option, sessionConfig reconnect fields
- `internal/login/login.go` - WithTSIH option for session reinstatement

## Decisions Made
- SNACK uses task's own ITT per RFC 7143 Section 11.16 (Pitfall 4) -- not a new ITT
- Per-task SNACK timeout (default 5s) fires Status SNACK for tail loss detection per D-06
- startPumps captures conn/writeCh/unsolCh/done locally before goroutine start to prevent race during connection replacement
- Logout failure on ERL 2 replacement is non-fatal (target may have already cleaned up old connection state)
- WithTSIH added to login package as blocking dependency for ERL 2 reinstatement

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Added WithTSIH login option for session reinstatement**
- **Found during:** Task 2 (ERL 2 connection replacement)
- **Issue:** login.WithTSIH does not exist -- needed for session reinstatement login with non-zero TSIH
- **Fix:** Added tsih field to loginConfig, WithTSIH option, and wired tsih into loginState
- **Files modified:** internal/login/login.go
- **Verification:** All login tests pass, ERL 2 tests use WithTSIH successfully
- **Committed in:** fe2b7f2 (Task 2 commit)

**2. [Rule 3 - Blocking] Added WithReconnectInfo session option and reconnect fields**
- **Found during:** Task 2 (ERL 2 connection replacement)
- **Issue:** Session struct lacks targetAddr, loginOpts, isid, tsih fields needed for reconnection
- **Fix:** Added fields to Session struct, WithReconnectInfo option, sessionConfig fields
- **Files modified:** internal/session/session.go, internal/session/types.go
- **Verification:** ERL 2 tests dial and login successfully during replacement
- **Committed in:** fe2b7f2 (Task 2 commit)

**3. [Rule 1 - Bug] Race-safe pump goroutine management**
- **Found during:** Task 2 (ERL 2 connection replacement)
- **Issue:** writePumpLoop/readPumpLoop/dispatchLoop accessed s.conn/s.writeCh/s.unsolCh without mutex, racing with replaceConnection
- **Fix:** Refactored pump starts into startPumps method that captures conn/channels locally before goroutine creation
- **Files modified:** internal/session/session.go
- **Verification:** All tests pass with -race (no data race warnings)
- **Committed in:** fe2b7f2 (Task 2 commit)

---

**Total deviations:** 3 auto-fixed (1 bug, 2 blocking)
**Impact on plan:** All auto-fixes necessary for correctness and compilation. No scope creep.

## Issues Encountered
None beyond the auto-fixed deviations above.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- ERL 0 (Plan 02), ERL 1 (SNACK), and ERL 2 (connection replacement) complete the full error recovery hierarchy per RFC 7143 Section 7
- WithTSIH and WithReconnectInfo are available for Plan 02 (ERL 0 reconnect) if running in parallel
- startPumps pattern establishes safe connection replacement for any future reconnection code

## Self-Check: PASSED

- All 4 created files exist on disk
- Both commits (54fe5b2, fe2b7f2) found in git log
- All tests pass with -race

---
*Phase: 06-error-recovery-and-task-management*
*Completed: 2026-04-01*
