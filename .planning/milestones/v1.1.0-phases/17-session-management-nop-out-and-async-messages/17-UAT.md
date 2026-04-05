---
status: complete
phase: 17-session-management-nop-out-and-async-messages
source: [17-01-SUMMARY.md, 17-02-SUMMARY.md, 17-03-SUMMARY.md]
started: 2026-04-05T12:30:00Z
updated: 2026-04-05T12:31:00Z
---

## Current Test

[testing complete]

## Tests

### 1. NOP-Out conformance tests pass with -race
expected: TestNOPOut_PingResponse (SESS-03), TestNOPOut_PingRequest (SESS-04), TestNOPOut_ExpStatSNConfirmation (SESS-05) all PASS under -race.
result: pass

### 2. Session lifecycle tests pass with -race
expected: TestSession_LogoutAfterAsyncEvent1 (SESS-01) and TestSession_CleanLogout (SESS-06) both PASS under -race.
result: pass

### 3. Async message tests pass with -race
expected: TestAsync_LogoutRequest (ASYNC-01), TestAsync_ConnectionDrop (ASYNC-02), TestAsync_SessionDrop (ASYNC-03), TestAsync_NegotiationRequest (ASYNC-04) all PASS under -race.
result: pass

### 4. SendAsyncMsg exists on MockTarget
expected: `test/target.go` has `func (mt *MockTarget) SendAsyncMsg` with generic API per D-01.
result: pass

### 5. SendExpStatSNConfirmation exported
expected: Public `SendExpStatSNConfirmation() error` method exists on Session, callable from external test package.
result: pass

### 6. LUN echo fix applied
expected: `internal/session/keepalive.go` handleUnsolicitedNOPIn echoes LUN field from NOP-In in NOP-Out response.
result: pass

### 7. Full test suite unaffected
expected: `go test ./... -race -count=1 -timeout 120s` passes — no regressions.
result: pass

## Summary

total: 7
passed: 7
issues: 0
pending: 0
skipped: 0

## Gaps

[none]
