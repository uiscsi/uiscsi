---
status: complete
phase: 14-data-transfer-and-r2t-wire-validation
source: [14-01-SUMMARY.md, 14-02-SUMMARY.md, 14-03-SUMMARY.md, 14-04-SUMMARY.md]
started: 2026-04-05T02:30:00Z
updated: 2026-04-05T03:35:00Z
---

## Current Test

[testing complete]

## Tests

### 1. All 18 conformance tests pass with race detector
expected: `go test ./test/conformance/ -run "TestDataOut|TestDataIn|TestR2T" -count=1 -timeout 60s -race` exits 0 with all tests passing.
result: pass

### 2. Data-Out tests verify wire-level fields under bilateral negotiation
expected: `go test ./test/conformance/ -run TestDataOut -v -count=1 -timeout 30s` shows 10 passing tests with verbose output confirming DataSN, TTT, BufferOffset, F-bit, and MaxRecvDSL assertions.
result: pass

### 3. Data-In A-bit DataACK production fix works
expected: `go test ./test/conformance/ -run TestDataIn_ABitDataACK -v -count=1 -timeout 30s` passes, proving the initiator now sends SNACK Type=DataACK when target sets A-bit on Data-In at ERL>=1.
result: pass

### 4. R2T fulfillment tests verify per-burst isolation
expected: `go test ./test/conformance/ -run TestR2T -v -count=1 -timeout 30s` shows 4 passing tests including parallel command ITT/TTT isolation.
result: pass

### 5. MockTarget NegotiationConfig controls login parameters
expected: `go test ./test/conformance/ -run TestDataOut_NoUnsolicited -v -count=1 -timeout 15s` passes — this test sets ImmediateData=No/InitialR2T=Yes via NegotiationConfig and verifies zero unsolicited Data-Out PDUs.
result: pass

### 6. Full project regression check
expected: `go test ./... -count=1 -timeout 180s` exits 0 with no failures across all packages.
result: pass

### 7. No race conditions under stress
expected: `go test ./test/conformance/ -run "TestDataOut|TestDataIn|TestR2T" -count=5 -timeout 120s -race` exits 0 — running 5 iterations catches intermittent races.
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
