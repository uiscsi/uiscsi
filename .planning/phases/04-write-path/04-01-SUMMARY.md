---
phase: 04-write-path
plan: 01
subsystem: session
tags: [iscsi, write-path, io-reader, immediate-data, scsi-command]

# Dependency graph
requires:
  - phase: 03-session-read-path-and-discovery
    provides: Session Submit, task struct, Data-In reassembly, CmdSN flow control
provides:
  - Command.Data io.Reader field for write payload
  - Submit auto-detection of writes from Data != nil
  - Immediate data reading bounded by min(FirstBurstLength, MaxRecvDataSegmentLength)
  - Task struct extended with isWrite, reader, bytesSent for R2T handling
affects: [04-write-path plan 02 R2T handling, 04-write-path plan 03 Data-Out generation]

# Tech tracking
tech-stack:
  added: []
  patterns: [io.Reader for write data instead of []byte, auto W-bit detection from non-nil Data]

key-files:
  created: []
  modified:
    - internal/session/types.go
    - internal/session/datain.go
    - internal/session/session.go
    - internal/session/session_test.go
    - internal/session/datain_test.go

key-decisions:
  - "io.Reader on Command for write data -- callers use bytes.NewReader for []byte, enables streaming large writes"
  - "Auto-set W-bit when cmd.Data != nil -- callers don't need to set both Data and Write"
  - "Immediate data bounded by min(FirstBurstLength, MaxRecvDataSegmentLength) per RFC 7143"

patterns-established:
  - "io.Reader ownership transfer: Submit reads immediate data, then transfers reader to task goroutine"
  - "bytesSent tracking on task struct for cumulative offset across immediate + unsolicited + R2T"

requirements-completed: [WRITE-03]

# Metrics
duration: 5min
completed: 2026-04-01
---

# Phase 4 Plan 01: Command Data Interface and Immediate Data Summary

**Command type uses io.Reader for write data with auto W-bit detection and immediate data reading from reader**

## Performance

- **Duration:** 5 min
- **Started:** 2026-04-01T10:01:37Z
- **Completed:** 2026-04-01T10:06:45Z
- **Tasks:** 3
- **Files modified:** 5

## Accomplishments
- Replaced ImmediateData []byte with Data io.Reader on Command struct for streaming write support
- Submit auto-detects write commands from non-nil Data, reads immediate data bounded by negotiated parameters
- Task struct extended with isWrite, reader, bytesSent fields for R2T handling in Plan 02
- All existing tests pass with new interface, new TestSessionSubmitWriteImmediateData verifies write path

## Task Commits

Each task was committed atomically:

1. **Task 1: Modify Command type and task struct for write support** - `1ea7159` (feat)
2. **Task 2: Update Submit for write detection and immediate data reading** - `179d8f7` (feat)
3. **Task 3: Update existing tests to use new Command.Data interface** - `87e41ae` (test)

## Files Created/Modified
- `internal/session/types.go` - Command struct: removed ImmediateData []byte, added Data io.Reader
- `internal/session/datain.go` - task struct: added isWrite, reader, bytesSent; updated newTask signature
- `internal/session/session.go` - Submit: auto W-bit, immediate data reading from io.Reader, reader ownership transfer
- `internal/session/session_test.go` - Added TestSessionSubmitWriteImmediateData
- `internal/session/datain_test.go` - Updated newTask calls for new 3-arg signature

## Decisions Made
- io.Reader on Command for write data -- callers use bytes.NewReader for []byte, enables streaming large writes
- Auto-set W-bit when cmd.Data != nil -- callers don't need to set both Data and Write fields
- Immediate data bounded by min(FirstBurstLength, MaxRecvDataSegmentLength) per RFC 7143 Section 12.11

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Command.Data io.Reader and task.reader/bytesSent fields ready for Plan 02 R2T handling
- Plan 02 can implement Data-Out PDU generation using task.reader and task.bytesSent offset

---
*Phase: 04-write-path*
*Completed: 2026-04-01*
