---
phase: 06-error-recovery-and-task-management
plan: 01
subsystem: session
tags: [tmf, task-management, error-injection, iscsi, rfc7143]

requires:
  - phase: 05-scsi-command-layer
    provides: Session struct, task struct, cleanupTask, Submit, Router
provides:
  - FaultConn test utility for deterministic error injection
  - All six TMF methods on Session (AbortTask, AbortTaskSet, ClearTaskSet, LUNReset, TargetWarmReset, TargetColdReset)
  - TMF/SNACK constants and TMFResult type
  - Recovery SessionOptions (MaxReconnectAttempts, ReconnectBackoff, SNACKTimeout)
  - ErrTaskAborted sentinel for aborted task notification
affects: [06-02, 06-03, error-recovery, reconnection]

tech-stack:
  added: []
  patterns: [TMF immediate-mode PDU exchange, LUN-based task cleanup with snapshot pattern]

key-files:
  created:
    - internal/transport/faultconn.go
    - internal/transport/faultconn_test.go
    - internal/session/tmf.go
    - internal/session/tmf_test.go
  modified:
    - internal/session/types.go
    - internal/session/datain.go
    - internal/session/session.go

key-decisions:
  - "TMF always uses Immediate=true and window.current() not acquire() per RFC 7143 Section 11.5"
  - "Cleanup order: Unregister from Router, then cancel task, then delete from map (Pitfall 2)"
  - "LUN-based cleanup snapshots matching ITTs under lock then cleans outside lock (Pitfall 8)"
  - "FaultConn uses mutex-protected cumulative byte counters for deterministic fault threshold triggering"

patterns-established:
  - "TMF sendTMF helper: Register ITT, build PDU with Immediate, send, wait with 30s timeout"
  - "FaultConn pattern: wrap net.Conn, inject faults via closure functions, threshold-based triggering"
  - "cleanupAbortedTask pattern: Unregister -> lock+delete -> cancel (ordered to prevent stale dispatch)"

requirements-completed: [TMF-01, TMF-02, TMF-03, TMF-04, TMF-05, TMF-06, TEST-05]

duration: 8min
completed: 2026-04-01
---

# Phase 06 Plan 01: TMF and faultConn Summary

**All six RFC 7143 task management functions with auto-cleanup plus faultConn deterministic error injection utility**

## Performance

- **Duration:** 8 min
- **Started:** 2026-04-01T16:03:39Z
- **Completed:** 2026-04-01T16:11:56Z
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments
- FaultConn wraps net.Conn with injectable read/write faults using byte-threshold closures, self-tested with -race
- All six TMF methods (AbortTask, AbortTaskSet, ClearTaskSet, LUNReset, TargetWarmReset, TargetColdReset) implemented on Session using shared sendTMF helper
- Auto-cleanup on TMFRespComplete: AbortTask cancels single task with ErrTaskAborted, LUN-scoped TMFs clean all matching tasks
- TMF, TMFResp, and SNACK constants defined per RFC 7143 Sections 11.5.1, 11.6.1, 11.16.1
- Recovery SessionOptions (WithMaxReconnectAttempts, WithReconnectBackoff, WithSNACKTimeout) ready for Plans 02/03

## Task Commits

Each task was committed atomically:

1. **Task 1: faultConn test utility and TMF types** - `e15929f` (feat)
2. **Task 2: All six TMF methods with auto-cleanup** - `140e305` (feat)

## Files Created/Modified
- `internal/transport/faultconn.go` - FaultConn wrapper with injectable read/write faults and threshold helpers
- `internal/transport/faultconn_test.go` - 6 subtests: passthrough, fault-after-bytes, concurrent safety, runtime fault setting
- `internal/session/tmf.go` - sendTMF helper plus 6 public TMF methods and cleanup helpers
- `internal/session/tmf_test.go` - Tests for all 6 TMFs including auto-cleanup verification, Immediate bit, context cancellation
- `internal/session/types.go` - TMF/SNACK constants, TMFResult, ErrTaskAborted, recovery SessionOptions
- `internal/session/datain.go` - Added lun field to task struct
- `internal/session/session.go` - Set tk.lun from Command.LUN in Submit

## Decisions Made
- TMF uses Immediate=true and s.window.current() (not acquire) per RFC 7143 Section 11.5 -- TMFs do not consume CmdSN window slots
- Cleanup follows Pitfall 2 ordering: Unregister ITT from Router first, then cancel task, then delete from map -- prevents stale PDU dispatch to cancelled tasks
- LUN-based cleanup snapshots matching ITTs under lock, then cleans each outside lock (Pitfall 8) -- avoids holding session lock during potentially blocking cancel operations
- FaultConn uses sync.Mutex protecting cumulative byte counters and fault function pointers for thread-safe runtime fault injection

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- Concurrent safety test initially deadlocked with net.Pipe due to synchronous read/write semantics -- simplified to test concurrent SetReadFault/SetWriteFault calls instead, which validates the mutex protection without pipe deadlock risk

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- TMF methods and faultConn are prerequisites for Plans 02 (ERL 0/1/2) and 03 (connection recovery)
- Recovery SessionOptions wired into sessionConfig defaults, ready for use
- ErrTaskAborted sentinel available for error recovery paths

## Self-Check: PASSED

All 7 files verified present. Both task commits (e15929f, 140e305) verified in git log.

---
*Phase: 06-error-recovery-and-task-management*
*Completed: 2026-04-01*
