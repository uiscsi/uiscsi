---
phase: 09-lio-e2e-tests
plan: 01
subsystem: testing
tags: [e2e, lio, configfs, iscsi-target, kernel]

# Dependency graph
requires:
  - phase: 07-public-api-observability-and-release
    provides: uiscsi public API (Dial, Discover, Session methods)
provides:
  - test/lio/ package for LIO configfs target setup/teardown
  - test/e2e/ with TestMain and TestBasicConnectivity
  - Orphan sweep for crashed test cleanup
affects: [09-02 (remaining E2E test scenarios)]

# Tech tracking
tech-stack:
  added: []
  patterns: [configfs direct manipulation via os package, e2e build tag gating, ephemeral port allocation]

key-files:
  created: [test/lio/lio.go, test/lio/sweep.go, test/e2e/e2e_test.go]
  modified: []

key-decisions:
  - "Cleanup func returned to caller (not t.Cleanup) for explicit teardown control"
  - "removeSymlinksIn helper for safe configfs directory cleanup without os.RemoveAll"
  - "generate_node_acls=1 for non-CHAP tests, explicit ACLs with authentication=1 for CHAP"

patterns-established:
  - "E2E test pattern: RequireRoot + RequireModules + lio.Setup + defer cleanup + exercise API"
  - "Configfs write pattern: os.WriteFile with []byte(value) without trailing newline"
  - "Orphan sweep pattern: SweepOrphans in TestMain before m.Run"

requirements-completed: [E2E-01, E2E-02, E2E-03, E2E-10]

# Metrics
duration: 3min
completed: 2026-04-02
---

# Phase 9 Plan 1: LIO E2E Test Foundation Summary

**LIO configfs helper package with Setup/Teardown/SweepOrphans and basic connectivity E2E test against real kernel iSCSI target**

## Performance

- **Duration:** 3 min
- **Started:** 2026-04-02T15:10:12Z
- **Completed:** 2026-04-02T15:13:08Z
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- Built test/lio/ package that creates/tears down real LIO iSCSI targets via direct configfs manipulation
- Implemented TestBasicConnectivity exercising Discover + Dial + Inquiry + ReadCapacity + TestUnitReady + Close
- Removed dead gotgt integration test stubs (6 t.Skip-only tests)
- All E2E code gated behind //go:build e2e tag -- existing test suite unaffected

## Task Commits

Each task was committed atomically:

1. **Task 1: LIO configfs helper package** - `3c284f7` (feat)
2. **Task 2: Basic connectivity E2E test + delete gotgt stubs** - `5ddac9d` (feat)

## Files Created/Modified
- `test/lio/lio.go` - LIO configfs target Setup/Teardown, skip helpers (RequireRoot, RequireModules, RequireConfigfs)
- `test/lio/sweep.go` - SweepOrphans for TestMain orphan cleanup
- `test/e2e/e2e_test.go` - TestMain with sweep, TestBasicConnectivity
- `test/integration/gotgt_test.go` - Deleted (dead code per D-10)

## Decisions Made
- Cleanup func uses setupState struct to track all created resources for strict reverse-order teardown
- removeSymlinksIn reads directory entries and checks ModeSymlink rather than assuming symlink names
- Backstore names use crypto/rand hex suffix for parallel test safety
- Non-CHAP tests use generate_node_acls=1 + demo_mode_write_protect=0 for simplicity

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- LIO helper package ready for remaining 6 E2E test scenarios in plan 09-02
- CHAP, digest, multi-LUN, TMF, data integrity, and error recovery tests can reuse lio.Setup with different Config options
- Existing test suite passes clean (go test ./...)

---
*Phase: 09-lio-e2e-tests*
*Completed: 2026-04-02*
