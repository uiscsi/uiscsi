---
status: passed
phase: 18-command-window-retry-and-erl-2
source: [18-01-SUMMARY.md, 18-02-SUMMARY.md, 18-03-SUMMARY.md, 18-04-SUMMARY.md]
started: 2026-04-05T18:50:00Z
updated: 2026-04-05T19:07:00Z
---

## Current Test

[all tests complete]

## Tests

### 1. Zero window blocks new commands
expected: `go test ./test/conformance/ -run TestCmdWindow_ZeroWindow -race -count=1 -v` passes. Initiator blocks on zero window, resumes when NOP-In opens it.
result: passed

### 2. Large window allows concurrent commands
expected: `go test ./test/conformance/ -run TestCmdWindow_LargeWindow -race -count=1 -v` passes. 8 concurrent commands succeed with unique contiguous CmdSNs.
result: passed

### 3. Window-of-one serializes commands
expected: `go test ./test/conformance/ -run TestCmdWindow_WindowOfOne -race -count=1 -v` passes. Commands serialize with window size 1.
result: passed

### 4. MaxCmdSN in SCSI Response closes window
expected: `go test ./test/conformance/ -run TestCmdWindow_MaxCmdSNInResponse -race -count=1 -v` passes. SCSI Response with negative delta closes window, NOP-In reopens.
result: passed

### 5. Same-connection retry preserves original fields
expected: `go test ./test/conformance/ -run TestRetry_SameConnectionRetry -race -count=1 -v` passes. Wire capture shows ITT[1]==ITT[0] and CmdSN[1]==CmdSN[0] after Reject at ERL>=1.
result: passed — ITT: 0x00000000==0x00000000, CmdSN: 1==1, CDB identical

### 6. Reject triggers caller reissue at ERL=1
expected: `go test ./test/conformance/ -run TestRetry_RejectCallerReissue -race -count=1 -v` passes. New ITT, new CmdSN, same CDB after caller reissues.
result: passed — CmdSN 1->2, ITT 0x00->0x01, CDB identical

### 7. ExpStatSN gap triggers Status SNACK
expected: `go test ./test/conformance/ -run TestRetry_ExpStatSNGap -race -count=1 -v` passes. Status SNACK (Type=1) fired within timeout at ERL=1.
result: passed — SNACK Type=1 BegRun=0 RunLength=0 ExpStatSN=9

### 8. ERL 2 connection reassignment
expected: `go test ./test/conformance/ -run TestERL2_ConnectionReassignment -race -count=1 -v` passes. Logout with reasonCode=2 captured on wire.
result: passed — ERL 2 connection replacement complete, tasks_reassigned=1

### 9. ERL 2 task reassign
expected: `go test ./test/conformance/ -run TestERL2_TaskReassign -race -count=1 -v` passes. TMF TASK REASSIGN (Function=14) with correct ReferencedTaskTag on new connection.
result: passed — ERL 2 connection replacement complete, tasks_reassigned=1

### 10. No regressions across full test suite
expected: `go test ./... -race -count=1 -timeout 120s` all packages pass. No failures in internal/session, test/conformance, or any other package.
result: passed — 11 packages, 0 failures

## Summary

total: 10
passed: 10
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps
