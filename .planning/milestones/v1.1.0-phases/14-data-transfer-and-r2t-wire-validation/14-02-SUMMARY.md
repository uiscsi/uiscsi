---
phase: 14-data-transfer-and-r2t-wire-validation
plan: 02
subsystem: test/conformance
tags: [data-out, wire-validation, conformance, write-path]
dependency_graph:
  requires: [14-01]
  provides: [dataout-conformance-tests]
  affects: []
tech_stack:
  added: []
  patterns: [bilateral-negotiation, pdu-capture-assertions, unsolicited-vs-solicited-filtering]
key_files:
  created:
    - test/conformance/dataout_test.go
  modified: []
decisions:
  - "Group by TTT to distinguish R2T bursts in DataSNPerR2T test"
  - "Filter unsolicited Data-Out by TTT=0xFFFFFFFF per RFC 7143 Section 11.7.1"
  - "Sum immediate data (SCSI Command DataSegmentLen) + unsolicited Data-Out for FirstBurstLength checks (Pitfall 5)"
metrics:
  duration: 4min
  completed: 2026-04-05
  tasks: 2
  files: 1
---

# Phase 14 Plan 02: Data-Out Wire Conformance Tests Summary

10 Data-Out wire conformance tests covering DataSN sequencing, TTT echo, MaxRecvDSL enforcement, F-bit correctness, FirstBurstLength compliance, and unsolicited/solicited path separation per RFC 7143.

## Tasks Completed

### Task 1: Data-Out solicited path tests (DATA-01, DATA-05, DATA-08, DATA-11, DATA-12, DATA-13)
**Commit:** 053dd6d

Created test/conformance/dataout_test.go with 6 solicited Data-Out tests:
- **TestDataOut_DataSN** (DATA-01): Verifies DataSN increments 0,1,2,3 across 4 PDUs at MaxRecvDSL=512 for 2048-byte write.
- **TestDataOut_TTTEcho** (DATA-05): Verifies Data-Out echoes TTT=0xDEADBEEF from R2T.
- **TestDataOut_MaxRecvDSL** (DATA-08): Verifies each Data-Out segment <= 256 bytes.
- **TestDataOut_FBitSolicited** (DATA-11): Verifies only last of 3 PDUs has Final=true.
- **TestDataOut_DataSNPerR2T** (DATA-12): Verifies DataSN resets to 0 for second R2T burst (grouped by TTT).
- **TestDataOut_BufferOffset** (DATA-13): Verifies offsets 0, 512, 1024, 1536.

All tests use bilateral negotiation (SetNegotiationConfig + WithOperationalOverrides) and pducapture assertions.

### Task 2: Data-Out unsolicited path tests (DATA-02, DATA-03, DATA-04, DATA-10)
**Commit:** abf0964

Added 4 unsolicited Data-Out tests:
- **TestDataOut_UnsolicitedFirstBurst** (DATA-02): Verifies immediate + unsolicited Data-Out <= FirstBurstLength=1024, with solicited R2T follow-up.
- **TestDataOut_NoUnsolicited** (DATA-03): Verifies SCSI Command DataSegmentLen=0 and no TTT=0xFFFFFFFF PDUs in R2T-only mode.
- **TestDataOut_FirstBurstLimit** (DATA-04): Verifies total unsolicited exactly equals FBL=768 (256 immediate + 2x256 Data-Out).
- **TestDataOut_FBitUnsolicited** (DATA-10): Verifies only last unsolicited Data-Out (TTT=0xFFFFFFFF) has Final=true.

All 10 tests pass with -race. Full conformance suite green.

## Deviations from Plan

None - plan executed exactly as written.

## Verification Results

```
go test ./test/conformance/ -run TestDataOut -count=1 -timeout 30s -race
ok  github.com/rkujawa/uiscsi/test/conformance  1.019s

go test ./test/conformance/ -count=1 -timeout 30s -race
ok  github.com/rkujawa/uiscsi/test/conformance  10.218s
```

## Self-Check: PASSED

- FOUND: test/conformance/dataout_test.go
- FOUND: commit 053dd6d (Task 1)
- FOUND: commit abf0964 (Task 2)
