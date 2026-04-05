---
phase: 14-data-transfer-and-r2t-wire-validation
plan: 01
subsystem: testing
tags: [iscsi, mock-target, data-in, r2t, data-out, negotiation]

# Dependency graph
requires:
  - phase: 13-pdu-wire-capture-framework-mocktarget-extensions-and-command-sequencing
    provides: MockTarget with SessionState, TargetConn, PDUHandler framework
provides:
  - NegotiationConfig for controlling login parameter negotiation in tests
  - ReadPDU method on TargetConn for inline Data-Out receive
  - HandleSCSIReadMultiPDU for multi-PDU Data-In responses
  - SendR2TSequence for generating R2T chains with unique TTT/R2TSN
  - ReadDataOutPDUs for collecting Data-Out PDUs until Final bit
affects: [14-02, 14-03, 14-04, data-transfer-conformance-tests]

# Tech tracking
tech-stack:
  added: []
  patterns: [pointer-field config for optional negotiation overrides, multi-PDU chunked response helpers]

key-files:
  created: []
  modified: [test/target.go]

key-decisions:
  - "NegotiationConfig uses pointer fields (nil = echo initiator proposal) for backward compatibility"
  - "ReadPDU wraps ReadRawPDU with digest=false and maxRecvDSL=0 for test simplicity"
  - "SendR2TSequence is a standalone function (not method) for flexibility in custom handlers"

patterns-established:
  - "Pointer-field config pattern: nil means default, non-nil overrides"
  - "Multi-PDU response helper: chunk data, track DataSN/offset, set F/S bits on final"
  - "R2T sequence generation: baseTTT + r2tsn for unique TTT assignment"

requirements-completed: [DATA-01, DATA-02, DATA-03, DATA-04, DATA-05, DATA-06, DATA-07, DATA-08, DATA-09, DATA-10, DATA-11, DATA-12, DATA-13, DATA-14, R2T-01, R2T-02, R2T-03, R2T-04]

# Metrics
duration: 3min
completed: 2026-04-05
---

# Phase 14 Plan 01: Data-Out/R2T PDU Serialization Summary

**MockTarget extended with NegotiationConfig, multi-PDU Data-In, R2T sequence generation, and Data-Out receive for Phase 14 conformance tests**

## Performance

- **Duration:** 3 min
- **Started:** 2026-04-05T00:29:47Z
- **Completed:** 2026-04-05T00:32:38Z
- **Tasks:** 2
- **Files modified:** 1

## Accomplishments
- NegotiationConfig with 6 pointer fields controls login parameter overrides (ImmediateData, InitialR2T, FirstBurstLength, MaxBurstLength, MaxRecvDataSegmentLength, ErrorRecoveryLevel)
- HandleSCSIReadMultiPDU splits read data into correctly sequenced multi-PDU Data-In responses with DataSN, BufferOffset, F-bit, and S-bit
- SendR2TSequence generates R2T chains with unique TTT, incrementing R2TSN, and correct BufferOffset/DesiredDataTransferLength
- ReadDataOutPDUs collects Data-Out PDUs from initiator until Final bit, enabling write flow testing

## Task Commits

Each task was committed atomically:

1. **Task 1: Add NegotiationConfig, ReadPDU, and modify HandleLogin** - `fcfb9b1` (feat)
2. **Task 2: Add HandleSCSIReadMultiPDU and R2T sequence helper** - `45c22c1` (feat)

## Files Created/Modified
- `test/target.go` - Extended MockTarget with NegotiationConfig, ReadPDU, HandleSCSIReadMultiPDU, SendR2TSequence, ReadDataOutPDUs, BoolPtr, Uint32Ptr

## Decisions Made
- NegotiationConfig uses pointer fields so nil means "echo initiator proposal" preserving backward compatibility with all existing tests
- ReadPDU uses digest=false and maxRecvDSL=0 since test environments do not use digests
- SendR2TSequence is a standalone function rather than a method to allow use from both HandleSCSIFunc handlers and custom test logic

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness
- All MockTarget infrastructure for Phase 14 conformance tests is in place
- Plans 02-04 can use NegotiationConfig to set up different ImmediateData/InitialR2T/burst length scenarios
- SendR2TSequence and ReadDataOutPDUs enable write-path conformance tests

## Self-Check: PASSED

All files exist, all commits verified, all required functions present in test/target.go.

---
*Phase: 14-data-transfer-and-r2t-wire-validation*
*Completed: 2026-04-05*
