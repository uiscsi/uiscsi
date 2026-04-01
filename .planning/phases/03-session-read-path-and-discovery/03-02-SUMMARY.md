---
phase: 03-session-read-path-and-discovery
plan: 02
subsystem: session
tags: [iscsi, nop-out, nop-in, keepalive, async-message, logout, rfc7143]

# Dependency graph
requires:
  - phase: 03-01
    provides: "Session layer with SCSI command dispatch, CmdSN flow control, Data-In reassembly"
provides:
  - "NOP-Out/NOP-In keepalive with configurable interval and timeout"
  - "Unsolicited NOP-In echo (target-initiated ping response)"
  - "AsyncMsg dispatch to user callback with event code handling"
  - "Target-requested logout auto-handling with Time2Wait"
  - "Graceful Logout PDU exchange with LogoutReq/LogoutResp"
  - "LogoutConnection with reason code 2 for connection recovery"
  - "Close() integration with graceful logout before force-close"
affects: [error-recovery, connection-management, task-management]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Keepalive goroutine with ticker + timeout pattern"
    - "Unsolicited PDU dispatch by opcode to dedicated handlers"
    - "Drain in-flight tasks before logout via polling loop"
    - "loggedIn state flag for Close() logout gating"

key-files:
  created:
    - internal/session/keepalive.go
    - internal/session/keepalive_test.go
    - internal/session/async.go
    - internal/session/logout.go
    - internal/session/logout_test.go
  modified:
    - internal/session/session.go

key-decisions:
  - "Refactored handleUnsolicited into opcode-based dispatch to dedicated handlers in separate files"
  - "Logout() drains in-flight tasks before CmdSN acquire to avoid window-closed error"
  - "Close() attempts graceful logout with 5s timeout before force-closing"
  - "loggedIn bool state tracks whether session is in full-feature phase"

patterns-established:
  - "Opcode-based unsolicited PDU dispatch: handleUnsolicited delegates to handleUnsolicitedNOPIn and handleAsyncMsg by opcode"
  - "Keepalive pattern: ticker goroutine with Router.Register for single-response correlation"

requirements-completed: [SESS-05, EVT-01, EVT-02, EVT-03]

# Metrics
duration: 7min
completed: 2026-04-01
---

# Phase 3 Plan 2: Keepalive, Async Events, and Logout Summary

**NOP-Out/NOP-In keepalive with timeout detection, async message dispatch to user callbacks, and graceful Logout PDU exchange per RFC 7143**

## Performance

- **Duration:** 7 min
- **Started:** 2026-04-01T07:19:34Z
- **Completed:** 2026-04-01T07:26:41Z
- **Tasks:** 2
- **Files modified:** 6

## Accomplishments
- Keepalive detects dead connections via periodic NOP-Out pings with configurable interval/timeout (SESS-05)
- Target NOP-In pings echoed with NOP-Out response including TTT and data (SESS-05)
- AsyncMsg PDUs dispatched to user callback with event code handling including auto-logout on EventCode 1 (EVT-01)
- Graceful logout drains in-flight tasks, exchanges Logout/LogoutResp PDUs, supports reason code 2 for recovery (EVT-02, EVT-03)
- Close() attempts graceful logout before force-closing transport

## Task Commits

Each task was committed atomically:

1. **Task 1: NOP-Out/NOP-In keepalive and async message handling** - `02d3264` (feat)
2. **Task 2: Logout PDU exchange and session Close integration** - `1735202` (feat)

## Files Created/Modified
- `internal/session/keepalive.go` - Keepalive loop, NOP-Out ping, unsolicited NOP-In echo
- `internal/session/keepalive_test.go` - 4 tests: ping, timeout, echo, async callback
- `internal/session/async.go` - AsyncMsg dispatch, target-requested logout handling
- `internal/session/logout.go` - Logout/LogoutResp exchange, Logout(), LogoutConnection()
- `internal/session/logout_test.go` - 5 tests: graceful, timeout, drain, reason code 2, Close
- `internal/session/session.go` - Keepalive goroutine start, opcode dispatch, Close with logout

## Decisions Made
- Refactored inline handleUnsolicited into opcode-based dispatch delegating to keepalive.go and async.go handlers
- Logout() keeps command window open until after LogoutReq CmdSN is acquired (avoid errWindowClosed)
- Close() uses loggedIn bool flag to avoid attempting logout on already-errored sessions
- Target-requested logout (AsyncEvent code 1) handled in goroutine with DefaultTime2Wait delay

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed Logout() window-closed race**
- **Found during:** Task 2 (Logout tests)
- **Issue:** Logout() closed the command window before calling logout(), which needs window.acquire() for CmdSN
- **Fix:** Moved window.close() to after the logout PDU exchange succeeds
- **Files modified:** internal/session/logout.go
- **Verification:** TestLogoutGraceful and TestLogoutDrainsInFlight pass
- **Committed in:** 1735202 (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Essential fix for correctness. No scope creep.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Session layer now has complete keepalive, async, and logout support
- Ready for SendTargets discovery (03-03) or error recovery work
- All tests pass with -race, go vet clean

## Self-Check: PASSED

All 6 created/modified files verified present. Both task commits (02d3264, 1735202) verified in git log.

---
*Phase: 03-session-read-path-and-discovery*
*Completed: 2026-04-01*
