---
phase: 15-scsi-command-write-mode-wire-tests
plan: 01
subsystem: testing
tags: [conformance, write-path, helpers, mock-target]

requires:
  - phase: 14-data-out-r2t-pdu-wire-conformance
    provides: MockTarget infrastructure, PDU capture, DataOut conformance tests
provides:
  - Shared write-test helpers for conformance test setup
  - Reusable negotiation override conversion
  - Common R2T/Response send helpers
affects: [15-02, future conformance test plans]

tech-stack:
  added: []
  patterns: [writeTestSetup struct pattern for conformance test DRY]

key-files:
  created: [test/conformance/helpers_test.go]
  modified: []

key-decisions:
  - "Helpers are additive -- existing dataout_test.go is not refactored to use them (out of scope per plan)"
  - "negotiationOverrides only converts boolean fields (ImmediateData, InitialR2T); numeric fields stay in manual overrides"

patterns-established:
  - "writeTestSetup: struct encapsulating MockTarget + Recorder with constructor and dialWithOverrides method"
  - "negotiationOverrides: NegotiationConfig bools to string map conversion for WithOperationalOverrides"

requirements-completed: []

duration: 1min
completed: 2026-04-05
---

# Phase 15 Plan 01: Shared Write-Test Helpers Summary

**Extracted 5 reusable write-test helpers into helpers_test.go to eliminate setup boilerplate for SCSI command wire conformance tests**

## What Was Done

### Task 1: Create shared write-test helpers (per D-03)

Created `test/conformance/helpers_test.go` with 5 helper functions:

1. **newWriteTestSetup** -- Creates MockTarget + Recorder, configures negotiation, registers login/logout/NOP-Out handlers, registers cleanup
2. **dialWithOverrides** -- Dials MockTarget with PDU hook, 30s keepalive, operational overrides; registers session cleanup
3. **negotiationOverrides** -- Converts NegotiationConfig boolean fields to string map for WithOperationalOverrides
4. **sendR2TAndConsume** -- Sends R2T for given offset/length, reads Data-Out until F-bit, returns ExpCmdSN/MaxCmdSN
5. **sendSCSIResponse** -- Sends standard success SCSI Response with correct ITT, StatSN, ExpCmdSN/MaxCmdSN

**Commit:** 74367f4

## Verification

- `go vet ./test/conformance/` -- passed (no issues)
- `go test ./test/conformance/ -run TestDataOut -count=1 -timeout 30s -race` -- all 10 DataOut tests passed in 1.028s

## Deviations from Plan

None -- plan executed exactly as written.

## Commits

| # | Hash | Message | Files |
|---|------|---------|-------|
| 1 | 74367f4 | feat(15-01): add shared write-test helpers for conformance tests | test/conformance/helpers_test.go |

## Self-Check: PASSED
