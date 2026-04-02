---
phase: 07-public-api-observability-and-release
plan: 01
subsystem: api
tags: [go, iscsi, public-api, functional-options, error-hierarchy, streaming-io]

# Dependency graph
requires:
  - phase: 06.1-observability-debugging
    provides: PDU hooks, metrics hooks, structured logging, enriched errors
  - phase: 05-scsi-command-layer
    provides: 19 SCSI CDB builders and response parsers
  - phase: 03-session-management
    provides: Session, Submit, TMF, SendTargets, Logout
provides:
  - Complete public API surface in root package uiscsi
  - Dial/Discover orchestration wrapping internal transport/login/session
  - Session type with 20+ typed SCSI command methods
  - Execute() raw CDB pass-through for arbitrary commands
  - StreamRead/StreamWrite for zero-copy I/O
  - Typed error hierarchy (SCSIError, TransportError, AuthError) with errors.As
  - Functional options for all configuration (login, session, observability)
affects: [07-02-integration-tests, 07-03-examples-release]

# Tech tracking
tech-stack:
  added: []
  patterns: [public-wrapper-types, functional-options-delegation, error-type-conversion, internal-to-public-adapter]

key-files:
  created: [uiscsi.go, session.go, stream.go, types.go, errors.go, options.go, errors_test.go, uiscsi_test.go]
  modified: []

key-decisions:
  - "All public types are value types in root package -- no internal type leakage"
  - "WithPDUHook adapter concatenates BHS+DataSegment into []byte to avoid exposing transport.RawPDU"
  - "wrapAuthError classifies login.LoginError by StatusClass for auth vs transport errors"
  - "ReadBlocks/WriteBlocks use Read16/Write16 exclusively for full 64-bit LBA support"
  - "RequestSense reuses submitAndCheck instead of unexported checkResult"

patterns-established:
  - "Public wrapper pattern: root package types wrap internal types with converter functions"
  - "Error wrapping: wrapSCSIError/wrapTransportError/wrapAuthError at API boundary"
  - "Functional options delegation: public Option -> internal LoginOption/SessionOption"
  - "submitAndCheck helper centralizes SCSI status checking for all typed methods"

requirements-completed: [API-01, API-02, API-03, API-04, API-05, OBS-01, OBS-02, OBS-03]

# Metrics
duration: 6min
completed: 2026-04-02
---

# Phase 7 Plan 1: Public API Surface Summary

**Complete public API surface with Dial/Discover, 20+ Session methods, typed error hierarchy, streaming I/O, and observability options**

## Performance

- **Duration:** 6 min
- **Started:** 2026-04-02T08:38:23Z
- **Completed:** 2026-04-02T08:44:07Z
- **Tasks:** 2
- **Files modified:** 8

## Accomplishments
- Built complete public API surface: Dial(), Discover(), Session with ReadBlocks/WriteBlocks/Inquiry/ReadCapacity/Execute and 15+ more SCSI commands
- Typed error hierarchy (SCSIError, TransportError, AuthError) with full errors.As/Unwrap support and converter functions from internal error types
- 14 functional options delegating cleanly to internal login/session options including observability hooks (PDUHook, MetricsHook, Logger, AsyncHandler)
- StreamRead/StreamWrite for zero-copy I/O returning io.Reader directly from PDU reassembly

## Task Commits

Each task was committed atomically:

1. **Task 1: Public types, error hierarchy, and functional options** - `7024c2c` (feat)
2. **Task 2: Dial/Discover orchestration and Session methods** - `6af9b23` (feat)

## Files Created/Modified
- `uiscsi.go` - Package doc, Dial(), Discover() orchestration
- `session.go` - Session type with 20+ SCSI command methods, TMF, Execute()
- `stream.go` - StreamRead/StreamWrite streaming I/O variants
- `types.go` - Public wrapper types (Target, Portal, Result, Capacity, etc.) and converter functions
- `errors.go` - SCSIError, TransportError, AuthError with wrappers
- `options.go` - 14 functional options delegating to internal options
- `errors_test.go` - Internal tests for error types and converter functions
- `uiscsi_test.go` - External tests for Dial failure, option compilation, error formatting

## Decisions Made
- All public types are value types in root package -- no internal type leakage in exported signatures
- WithPDUHook adapter concatenates BHS+DataSegment into single []byte to avoid exposing internal transport.RawPDU
- wrapAuthError classifies login.LoginError by StatusClass to distinguish auth errors from transport errors
- ReadBlocks/WriteBlocks use Read16/Write16 exclusively (not Read10/Write10) for full 64-bit LBA support
- RequestSense uses submitAndCheck (public helper) instead of trying to call unexported scsi.checkResult

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] RequestSense cannot use unexported scsi.checkResult**
- **Found during:** Task 2 (Session methods)
- **Issue:** Plan called for scsi.CheckResult but the function is unexported (checkResult)
- **Fix:** Used submitAndCheck helper which performs the same status/sense checking
- **Files modified:** session.go
- **Verification:** go build ./... succeeds, go test passes
- **Committed in:** 6af9b23 (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 blocking)
**Impact on plan:** Minor adaptation to unexported function, no functional difference.

## Issues Encountered
None

## Known Stubs
None -- all methods are fully wired to internal implementations.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Public API surface is complete and compiles cleanly
- Ready for Plan 02 (integration tests against gotgt) and Plan 03 (examples/release)
- All 8 requirements (API-01 through API-05, OBS-01 through OBS-03) are satisfied

---
*Phase: 07-public-api-observability-and-release*
*Completed: 2026-04-02*
