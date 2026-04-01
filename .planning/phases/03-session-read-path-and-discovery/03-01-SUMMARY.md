---
phase: 03-session-read-path-and-discovery
plan: 01
subsystem: session
tags: [iscsi, session, cmdsn, datain, scsi-command, rfc7143]

requires:
  - phase: 02-connection-and-login
    provides: "NegotiatedParams, transport.Conn, login.Login"
  - phase: 01-pdu-codec-and-transport
    provides: "PDU codec, Router, ReadPump/WritePump, serial arithmetic"
provides:
  - "Session struct with NewSession, Submit, Close"
  - "CmdSN/MaxCmdSN command windowing per RFC 7143 Section 3.2.2"
  - "Data-In reassembly with DataSN/BufferOffset validation"
  - "StatSN/ExpStatSN tracking from every response PDU"
  - "Router persistent registrations for multi-PDU commands"
  - "Command, Result, AsyncEvent, DiscoveryTarget, Portal types"
affects: [03-02, 03-03, 04-write-path, 05-error-recovery]

tech-stack:
  added: []
  patterns: [buffered-datain-reassembly, per-task-goroutine-dispatch, persistent-router-entries, context-aware-cmdwindow]

key-files:
  created:
    - internal/session/types.go
    - internal/session/cmdwindow.go
    - internal/session/cmdwindow_test.go
    - internal/session/datain.go
    - internal/session/datain_test.go
    - internal/session/session.go
    - internal/session/session_test.go
  modified:
    - internal/login/params.go
    - internal/login/login.go
    - internal/transport/router.go
    - internal/transport/conn.go

key-decisions:
  - "Buffered Data-In reassembly (bytes.Buffer) instead of io.Pipe to avoid deadlock with unbuffered pipe semantics"
  - "Per-task goroutine drains Router channel preventing slow reader from blocking other commands"
  - "Router refactored with routerEntry struct to support persistent registrations for multi-PDU commands"
  - "CmdSN window initialized to size 1 (cmdSN=expCmdSN=maxCmdSN) since login doesn't carry MaxCmdSN"
  - "NegotiatedParams extended with CmdSN/ExpStatSN for clean login-to-session handoff"

patterns-established:
  - "Per-task goroutine: each submitted SCSI command gets its own goroutine to process response PDUs"
  - "Buffered Data-In: Data-In PDUs accumulated in bytes.Buffer, delivered as bytes.Reader in Result"
  - "Persistent Router entries: RegisterPersistent creates entries that survive multiple Dispatch calls"
  - "Context-aware Cond.Wait: bridging sync.Cond with context.Context via goroutine + channel"

requirements-completed: [SESS-01, SESS-02, SESS-03, SESS-04, READ-01, READ-02, READ-03]

duration: 12min
completed: 2026-04-01
---

# Phase 03 Plan 01: Session Core Summary

**Session layer with CmdSN windowing, async SCSI command dispatch via Submit+Channel, and multi-PDU Data-In reassembly with DataSN/BufferOffset validation**

## Performance

- **Duration:** 12 min
- **Started:** 2026-04-01T07:03:10Z
- **Completed:** 2026-04-01T07:15:28Z
- **Tasks:** 2
- **Files modified:** 11

## Accomplishments
- Session struct wraps transport.Conn after login, auto-starts ReadPump/WritePump/dispatchLoop
- CmdSN/MaxCmdSN command window with sync.Cond gating, context cancellation, stale update rejection, and wrap-around support
- Submit returns channel-based Result with buffered io.Reader for read commands, nil for non-read
- Data-In reassembly validates DataSN sequence and BufferOffset, delivers error on gaps
- StatSN/ExpStatSN tracked from every response PDU (Data-In with S-bit, SCSIResponse)
- Router extended with persistent registrations for multi-PDU SCSI command correlation
- NegotiatedParams carries CmdSN/ExpStatSN from login to session layer

## Task Commits

Each task was committed atomically:

1. **Task 1: Types, CmdSN window, and NegotiatedParams handoff** - `b8d790c` (feat)
2. **Task 2: Session struct, Submit, Data-In reassembly, dispatcher** (TDD)
   - RED: `7630f1a` (test) - failing tests for datain and session
   - GREEN: `c708acf` (feat) - implementation passing all tests

## Files Created/Modified
- `internal/session/types.go` - Command, Result, AsyncEvent, DiscoveryTarget, Portal, SessionOption types
- `internal/session/cmdwindow.go` - CmdSN/MaxCmdSN window gating with sync.Cond per RFC 7143 Section 3.2.2
- `internal/session/cmdwindow_test.go` - 7 tests: acquire, blocking, cancel, close, stale, wrap-around, current
- `internal/session/datain.go` - Data-In reassembly with DataSN/BufferOffset validation
- `internal/session/datain_test.go` - 5 tests: single/multi DataIn, DSN gap, offset mismatch, non-read
- `internal/session/session.go` - Session struct, NewSession, Submit, dispatchLoop, Close
- `internal/session/session_test.go` - 6 tests: submit read, multi-PDU read, non-read, concurrent, params, StatSN
- `internal/login/params.go` - Added CmdSN/ExpStatSN fields and defaults
- `internal/login/login.go` - Populate CmdSN/ExpStatSN handoff before return
- `internal/transport/router.go` - Refactored with routerEntry, added RegisterPersistent, AllocateITT
- `internal/transport/conn.go` - Added NewConnFromNetConn, DigestHeader, DigestData

## Decisions Made
- **Buffered Data-In instead of io.Pipe:** io.Pipe has zero internal buffer, causing deadlock when Data-In writes block until reader reads but reader doesn't exist until Result is delivered. bytes.Buffer avoids this cleanly.
- **Per-task goroutine:** Each submitted command gets its own goroutine draining the Router channel, preventing one slow reader from blocking other task dispatches.
- **Router routerEntry refactor:** Changed `map[uint32]chan<- *RawPDU` to `map[uint32]*routerEntry` with persistent flag. Non-persistent entries (existing Register) auto-delete on Dispatch. Persistent entries survive for multi-PDU commands.
- **CmdSN window size 1 initial:** Login doesn't negotiate MaxCmdSN, so window starts at 1. First response PDU advances MaxCmdSN via update.
- **NewConnFromNetConn helper:** Added to transport.Conn for session layer and test construction without Dial.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] io.Pipe deadlock in Data-In reassembly**
- **Found during:** Task 2 (GREEN phase)
- **Issue:** Plan specified io.Pipe for streaming Data-In. io.Pipe has zero internal buffer, causing deadlock: pipe writer blocks in handleDataIn until reader reads, but reader reference is only delivered via Result channel which sends after the write.
- **Fix:** Replaced io.Pipe with bytes.Buffer for Data-In accumulation. Result.Data delivers bytes.NewReader wrapping the buffer. Functionally equivalent for callers (both implement io.Reader).
- **Files modified:** internal/session/datain.go
- **Verification:** All 18 tests pass including multi-PDU reassembly
- **Committed in:** c708acf

---

**Total deviations:** 1 auto-fixed (1 bug)
**Impact on plan:** Necessary correctness fix. io.Pipe streaming can be reconsidered in future optimization if memory-efficient streaming of large reads is needed.

## Issues Encountered
- Concurrent submit test initially deadlocked because CmdSN window of size 1 meant only 1 command could be in-flight. Fixed test to process commands sequentially from target side, with each response advancing the window.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Session layer complete with Submit, Data-In reassembly, and CmdSN windowing
- Ready for Plan 02 (NOP-Out keepalive, logout, async event handling)
- Ready for Plan 03 (SendTargets discovery)
- Write path (Data-Out, R2T) can build on the task/session infrastructure

## Self-Check: PASSED

All 11 files verified present. All 3 commits verified in git log.

---
*Phase: 03-session-read-path-and-discovery*
*Completed: 2026-04-01*
