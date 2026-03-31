---
status: complete
phase: 01-pdu-codec-and-transport
source: [01-01-SUMMARY.md, 01-02-SUMMARY.md, 01-03-SUMMARY.md]
started: 2026-03-31T21:00:00Z
updated: 2026-03-31T21:10:00Z
---

## Current Test

[testing complete]

## Tests

### 1. Full Test Suite Passes Under Race Detector
expected: Run `go test -race ./internal/...` — all tests pass with no race conditions detected. Expected: 60+ tests across serial, digest, pdu, and transport packages, all PASS.
result: pass

### 2. Serial Arithmetic Wrap-Around
expected: Run `go test -v -run TestLessThan ./internal/serial/` — tests pass including wrap-around cases where s1 > s2 numerically but s1 is "less than" s2 in serial arithmetic (e.g., 0xFFFFFFFF < 0x00000001).
result: pass

### 3. CRC32C Digest Matches RFC Test Vectors
expected: Run `go test -v -run TestHeaderDigest ./internal/digest/` — CRC32C computation matches all 4 RFC test vectors. DataDigest includes zero-padding bytes in CRC computation.
result: pass

### 4. PDU Round-Trip for All 18 Opcodes
expected: Run `go test -v -run TestRoundTrip ./internal/pdu/` — each of the 18 iSCSI opcode types (8 initiator, 10 target) marshals to BHS bytes and unmarshals back to identical struct values.
result: pass

### 5. PDU Framing Over TCP Pipe
expected: Run `go test -v -run TestReadWriteRawPDU ./internal/transport/` — PDUs written through WriteRawPDU on one end of net.Pipe() are read back identically by ReadRawPDU on the other end, including back-to-back PDUs, AHS, and data segments with padding.
result: pass

### 6. ITT Router Skips Reserved 0xFFFFFFFF
expected: Run `go test -v -run TestReservedITT ./internal/transport/` — the ITT allocator never returns 0xFFFFFFFF (reserved per RFC 7143), wrapping from 0xFFFFFFFE to 0x00000000.
result: pass

### 7. Concurrent Pump Round-Trip
expected: Run `go test -v -race -run TestPump ./internal/transport/` — write pump serializes concurrent PDU sends without interleaving, read pump dispatches responses to correct ITT waiters, unsolicited PDUs go to separate channel. No races detected.
result: pass

## Summary

total: 7
passed: 7
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

[none yet]
