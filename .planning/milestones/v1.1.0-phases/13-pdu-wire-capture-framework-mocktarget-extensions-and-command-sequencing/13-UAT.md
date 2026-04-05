---
status: complete
phase: 13-pdu-wire-capture-framework-mocktarget-extensions-and-command-sequencing
source: [13-01-SUMMARY.md, 13-02-SUMMARY.md]
started: 2026-04-05T00:00:00Z
updated: 2026-04-05T00:10:00Z
---

## Current Test

[testing complete]

## Tests

### 1. PDU capture framework unit tests pass under -race
expected: `go test -race ./test/pducapture/ -v` exits 0 with all 5 tests passing (decode, short data, invalid opcode, filter, defensive copy)
result: pass

### 2. MockTarget extensions tests pass under -race
expected: `go test -race ./test/ -v` exits 0 with all tests passing, including new HandleSCSIFunc call counter, SessionState Update semantics, and SetMaxCmdSNDelta tests
result: pass

### 3. CMDSEQ-01: CmdSN sequential increment verified on wire
expected: `go test -race ./test/conformance/ -run TestCmdSN_SequentialIncrement -v` exits 0. Test sends 5 TestUnitReady commands, captures PDUs via WithPDUHook, verifies CmdSN increments by exactly 1 between each non-immediate SCSI command.
result: pass

### 4. CMDSEQ-02: NOP-Out Immediate flag, no CmdSN advance
expected: `go test -race ./test/conformance/ -run TestCmdSN_ImmediateDelivery_NonTMF -v` exits 0. NOP-Out captured on wire has Immediate=true. SCSI commands before and after NOP-Out have CmdSN delta=1 (NOP-Out did not consume a slot).
result: pass

### 5. CMDSEQ-03: TMF Immediate flag, no CmdSN advance
expected: `go test -race ./test/conformance/ -run TestCmdSN_ImmediateDelivery_TMF -v` exits 0. TMF (LUN Reset) captured on wire has Immediate=true. SCSI commands before and after TMF have CmdSN delta=1.
result: pass

### 6. Full conformance suite regression check
expected: `go test -race ./test/conformance/ -v` exits 0 with all 25 conformance tests passing (22 existing + 3 new CmdSN tests). No regressions from HandleLogin SessionState seeding.
result: pass

### 7. go vet clean on new packages
expected: `go vet ./test/pducapture/ ./test/` exits 0 with no warnings.
result: pass

## Summary

total: 7
passed: 7
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

[none]
