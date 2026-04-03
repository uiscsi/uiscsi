---
status: complete
phase: 09-lio-e2e-tests
source: [09-VERIFICATION.md]
started: 2026-04-02T17:30:00Z
updated: 2026-04-03T03:40:00Z
---

## Current Test

[testing complete]

## Tests

### 1. Basic Connectivity
expected: Discover returns target IQN; Dial succeeds; Inquiry returns VendorID="LIO-ORG"; ReadCapacity returns BlockSize consistent with 64MB LUN; TestUnitReady succeeds; Close returns nil
result: pass
note: TestBasicConnectivity PASS — VendorID="LIO-ORG", BlockSize=512, LBA=131071

### 2. Data Integrity
expected: bytes.Equal passes for both LBA 0 and LBA 100 write-then-read cycles
result: pass
note: TestDataIntegrity PASS — both LBA 0 and LBA 100 byte-for-byte match

### 3. CHAP Authentication
expected: One-way CHAP succeeds; bad password dial returns non-nil error
result: pass
note: TestCHAP PASS, TestCHAPBadPassword PASS — bad password correctly rejected

### 4. Mutual CHAP
expected: Bidirectional CHAP completes without error; Inquiry after auth succeeds
result: pass
note: TestCHAPMutual PASS — VendorID="LIO-ORG" after mutual CHAP

### 5. CRC32C Digests
expected: Dial with CRC32C header+data digests succeeds; write+read returns identical data
result: pass
note: TestDigests PASS — CRC32C header+data negotiated, data integrity verified

### 6. Multi-LUN
expected: ReportLuns returns LUNs 0, 1, 2; ReadCapacity for each returns correct sizes for 32/64/128MB
result: pass
note: TestMultiLUN PASS — 3 LUNs at 32/64/128MB confirmed

### 7. TMF LUN Reset
expected: LUNReset returns response=0 (Function Complete); Inquiry succeeds afterward
result: pass
note: TestTMF_LUNReset PASS — response=0, session functional after reset

### 8. Error Recovery Connection Drop
expected: ss -K kills TCP socket; retry loop succeeds within 10 attempts; Inquiry returns valid data after reconnect
result: pass
note: TestErrorRecovery_ConnectionDrop PASS — reconnect on attempt 1, VendorID="LIO-ORG"

## Summary

total: 8
passed: 8
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps
