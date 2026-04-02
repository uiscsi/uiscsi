---
phase: 07-public-api-observability-and-release
plan: 02
subsystem: testing
tags: [mock-target, conformance, iol, integration, gotgt, synctest]

requires:
  - phase: 07-01
    provides: "Public API surface (Dial, Session, Execute, TMF, errors, options)"
provides:
  - "MockTarget in-process iSCSI target for test infrastructure"
  - "IOL-inspired conformance test suite covering login, full-feature, error, TMF"
  - "Gotgt integration test skeleton with build tag"
affects: [07-03, future-phases]

tech-stack:
  added: []
  patterns: [mock-target-handler-registration, conformance-test-categories, tiered-test-approach]

key-files:
  created:
    - test/target.go
    - test/target_test.go
    - test/conformance/login_test.go
    - test/conformance/fullfeature_test.go
    - test/conformance/error_test.go
    - test/conformance/task_test.go
    - test/integration/gotgt_test.go
  modified: []

key-decisions:
  - "MockTarget uses opcode-keyed handler dispatch for maximum test flexibility"
  - "Conformance tests use public API only (package conformance_test) for true E2E validation"
  - "Integration tests behind //go:build integration tag per D-07 tiered approach"

patterns-established:
  - "Handler registration: tgt.HandleLogin(), tgt.HandleSCSIRead(lun, data) for composable test targets"
  - "Conformance test structure: setupTarget helper + context.WithTimeout + table-driven subtests"
  - "Build helpers: BuildInquiryData, BuildReadCapacity16Data, BuildReportLunsData for typed SCSI responses"

requirements-completed: [TEST-01, TEST-02]

duration: 7min
completed: 2026-04-02
---

# Phase 07 Plan 02: Test Infrastructure and Conformance Suite Summary

**Mock iSCSI target with handler-based PDU dispatch and 20 IOL-inspired conformance tests covering login, SCSI read/write/inquiry, error recovery, and task management**

## Performance

- **Duration:** 7 min
- **Started:** 2026-04-02T08:47:07Z
- **Completed:** 2026-04-02T08:54:07Z
- **Tasks:** 3
- **Files modified:** 7

## Accomplishments
- MockTarget accepts TCP connections and dispatches PDUs to registered handlers with built-in login/logout/SCSI/TMF/NOP support
- 20 conformance tests exercise the full public API (Dial, ReadBlocks, WriteBlocks, Inquiry, ReadCapacity, TestUnitReady, ReportLuns, Execute, StreamRead, AbortTask, LUNReset, TargetWarmReset) against in-process mock
- Gotgt integration skeleton with 6 test stubs behind //go:build integration tag

## Task Commits

Each task was committed atomically:

1. **Task 1: Mock iSCSI target with handler registration** - `f67b869` (feat)
2. **Task 2: IOL-inspired conformance test suite** - `52b823a` (feat)
3. **Task 3: Gotgt integration test skeleton** - `ae11d9d` (feat)

## Files Created/Modified
- `test/target.go` - MockTarget with PDUHandler dispatch, HandleLogin/HandleSCSIRead/HandleSCSIWrite/HandleLogout/HandleNOPOut/HandleTMF/HandleSCSIError/HandleDiscovery, BuildRawPDU, BuildInquiryData, BuildReadCapacity16Data, BuildReportLunsData
- `test/target_test.go` - 4 self-tests: AcceptConnection, LoginExchange, HandleSCSIRead, Close
- `test/conformance/login_test.go` - 5 login tests: AuthNone, WithTarget, InvalidAddress, ContextCancel, MultipleSessions
- `test/conformance/fullfeature_test.go` - 11 full-feature tests: SingleBlock/MultiBlock read, write, Inquiry, ReadCapacity, TestUnitReady, ReportLuns, RawCDB, RawRead, StreamRead, ContextTimeout
- `test/conformance/error_test.go` - 3 error tests: SCSICheckCondition with sense, TransportDrop, TypedErrorChain
- `test/conformance/task_test.go` - 3 TMF tests: AbortTask, LUNReset, TargetWarmReset
- `test/integration/gotgt_test.go` - 6 skipped stubs: Dial, ReadWrite, Inquiry, ReadCapacity, Discovery, CHAP

## Decisions Made
- MockTarget uses opcode-keyed handler dispatch (map[OpCode]PDUHandler) for maximum composability -- tests register only the handlers they need
- Conformance tests are in `package conformance_test` (external test package) to ensure they only use the public API
- HandleSCSIRead handles multiple CDB opcodes (READ, INQUIRY, READ CAPACITY, REPORT LUNS) through a single handler by inspecting CDB[0] -- simpler than per-opcode handlers for mock target
- Integration test skeleton uses t.Skip with descriptive messages rather than empty test bodies for clarity

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing Critical] Added HandleSCSIError and HandleDiscovery convenience methods**
- **Found during:** Task 1 (Mock target implementation)
- **Issue:** Plan mentioned conformance tests needing CHECK CONDITION and Discovery testing, but mock target didn't have error/discovery handlers
- **Fix:** Added HandleSCSIError(status, senseData) and HandleDiscovery(targets) methods to MockTarget
- **Files modified:** test/target.go
- **Verification:** Used in error_test.go (TestError_SCSICheckCondition)
- **Committed in:** f67b869 (Task 1 commit)

**2. [Rule 2 - Missing Critical] Added BuildInquiryData/BuildReadCapacity16Data/BuildReportLunsData helpers**
- **Found during:** Task 1 (Mock target implementation)
- **Issue:** Conformance tests need properly formatted SCSI response data but plan didn't specify builders
- **Fix:** Added typed builder functions that produce correct binary format for INQUIRY, READ CAPACITY(16), and REPORT LUNS responses
- **Files modified:** test/target.go
- **Verification:** Used in fullfeature_test.go tests
- **Committed in:** f67b869 (Task 1 commit)

---

**Total deviations:** 2 auto-fixed (2 missing critical)
**Impact on plan:** Both additions necessary for conformance tests to function. No scope creep.

## Issues Encountered
None

## Known Stubs
None -- all mock target functionality is wired and tested. Integration test stubs are intentional placeholders per D-07 tiered approach, gated behind build tag.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Test infrastructure complete for Plan 03 (documentation, examples, release prep)
- MockTarget available for any future test consumers via `github.com/rkujawa/uiscsi/test`
- Gotgt integration stubs ready to wire when dependency is added

## Self-Check: PASSED
- All 7 created files exist on disk
- All 3 task commit hashes found in git log (f67b869, 52b823a, ae11d9d)

---
*Phase: 07-public-api-observability-and-release*
*Completed: 2026-04-02*
