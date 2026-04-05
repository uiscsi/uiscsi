---
status: complete
phase: 16-error-injection-and-scsi-error-handling
source: [16-01-SUMMARY.md, 16-02-SUMMARY.md]
started: 2026-04-05T11:45:00Z
updated: 2026-04-05T11:46:30Z
---

## Current Test

[testing complete]

## Tests

### 1. ERR tests pass with -race
expected: All 6 ERR conformance tests pass under `go test ./test/conformance/ -run 'TestError_BUSY|TestError_ReservationConflict|TestError_UnexpectedUnsolicited|TestError_NotEnoughUnsolicited|TestError_CRCErrorSense|TestError_SNACKRejectNewCommand' -race -count=1 -v -timeout 60s` — each reports PASS, no race conditions detected.
result: pass

### 2. SNACK tests pass with -race
expected: Both SNACK wire conformance tests pass under `go test ./test/conformance/ -run 'TestSNACK_' -race -count=1 -v -timeout 60s` — TestSNACK_DataSNGap and TestSNACK_DataACKWireFields both PASS with no race conditions.
result: pass

### 3. HandleSCSIWithStatus uses SessionState.Update
expected: `test/target.go` HandleSCSIWithStatus method calls `mt.session.Update(cmd.CmdSN, cmd.Header.Immediate)` for CmdSN tracking — NOT hardcoded arithmetic like the legacy HandleSCSIError.
result: pass

### 4. errors.As pattern for SCSI error assertions
expected: All ERR tests in `test/conformance/error_test.go` use `errors.As(&scsiErr)` to extract `*SCSIError` — no string matching on error messages. Status, SenseKey, ASC, ASCQ verified via struct fields.
result: pass

### 5. Full test suite unaffected
expected: `go test ./... -race -count=1 -timeout 120s` passes — no regressions in existing tests from Phase 16 changes.
result: pass

## Summary

total: 5
passed: 5
issues: 0
pending: 0
skipped: 0

## Gaps

[none]
