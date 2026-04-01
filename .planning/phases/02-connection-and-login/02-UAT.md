---
status: complete
phase: 02-connection-and-login
source: [02-01-SUMMARY.md, 02-02-SUMMARY.md, 02-03-SUMMARY.md]
started: 2026-04-01T00:00:00Z
updated: 2026-04-01T00:00:00Z
---

## Current Test

[testing complete]

## Tests

### 1. Text Key-Value Codec Round-Trip
expected: EncodeTextKV/DecodeTextKV round-trips null-delimited key=value pairs with byte-perfect fidelity and deterministic ordering
result: pass
verified_by: `go test ./internal/login/ -run TestTextCodec -v -race` (7 subtests pass)

### 2. Negotiation Engine Covers All 14 Mandatory Keys
expected: KeyRegistry contains all 14 RFC 7143 Section 13 mandatory keys with correct negotiation types (BooleanAnd, BooleanOr, NumericalMin, NumericalMax, ListSelect, Declarative)
result: pass
verified_by: `go test ./internal/login/ -run TestKeyRegistryCompleteness -v -race` (pass)

### 3. Parameterized Negotiation Matrix (TEST-04)
expected: Parameterized tests cover all 6 negotiation types with combinatorial subtests (BoolAnd 4, BoolOr 4, NumMin 4, NumMax 3, ListSelect 4, Declarative 2)
result: pass
verified_by: `go test ./internal/login/ -run "TestNegotiation|TestNegotiate|TestFirstBurst" -v -race` (10 test functions, 21+ subtests pass)

### 4. NegotiatedParams Typed Struct
expected: NegotiatedParams provides compile-time safe typed fields (bool/uint32) instead of map[string]string, with RFC default values
result: pass
verified_by: `go test ./internal/login/ -run TestDefaults -v -race` (pass) + `go test ./internal/login/ -run TestNegotiateFullParams -v -race` (pass)

### 5. LoginError with errors.As()
expected: LoginError with StatusClass/StatusDetail works with errors.As() for structured error handling, 11 status constants match RFC 7143 Section 11.13
result: pass
verified_by: `go test ./internal/login/ -run "TestLoginError|TestStatusConstants" -v -race` (3 tests pass)

### 6. CHAP Response Computation (LOGIN-04)
expected: CHAP response computes MD5(id_byte || secret || challenge) matching RFC 1994
result: pass
verified_by: `go test ./internal/login/ -run TestChapResponse -v -race` (3 subtests pass)

### 7. CHAP Binary Encoding/Decoding
expected: Binary value encoding handles 0x hex prefix emission and accepts both 0x hex and 0b base64 prefix formats
result: pass
verified_by: `go test ./internal/login/ -run "TestEncodeCHAP|TestDecodeCHAP" -v -race` (8 subtests pass)

### 8. One-Way CHAP Exchange
expected: One-way CHAP exchange produces correct CHAP_N + CHAP_R from target challenge
result: pass
verified_by: `go test ./internal/login/ -run TestCHAPExchangeOneWay -v -race` (pass)

### 9. Mutual CHAP Exchange (LOGIN-05)
expected: Mutual CHAP generates initiator CHAP_I + CHAP_C and verifies target response with constant-time comparison
result: pass
verified_by: `go test ./internal/login/ -run TestCHAPExchangeMutual -v -race` (pass)

### 10. Login with AuthMethod=None (LOGIN-01, LOGIN-03)
expected: Login succeeds with AuthMethod=None against mock target, transitions through SecurityNeg -> OperationalNeg -> FullFeaturePhase, returns NegotiatedParams with TSIH
result: pass
verified_by: `go test ./internal/login/ -run TestLoginAuthNone -v -race` (pass)

### 11. Login with CHAP Authentication (LOGIN-04)
expected: Login succeeds with CHAP auth against mock target, correct MD5 response computed and verified
result: pass
verified_by: `go test ./internal/login/ -run TestLoginCHAP -v -race` (pass)

### 12. Login with Wrong CHAP Password
expected: Login fails with LoginError StatusClass=2, StatusDetail=1 (auth failure) when wrong password provided
result: pass
verified_by: `go test ./internal/login/ -run TestLoginCHAPWrongPassword -v -race` (pass)

### 13. Login with Mutual CHAP (LOGIN-05)
expected: Login succeeds with mutual CHAP, initiator verifies target's response
result: pass
verified_by: `go test ./internal/login/ -run TestLoginMutualCHAP -v -race` (pass)

### 14. Mutual CHAP Target Auth Failure
expected: Login fails when target sends wrong mutual CHAP response
result: pass
verified_by: `go test ./internal/login/ -run TestLoginMutualCHAPTargetAuthFail -v -race` (pass)

### 15. Digest Negotiation (INTEG-01, INTEG-02)
expected: Login negotiates HeaderDigest and DataDigest via ListSelect, NegotiatedParams reflects negotiated values
result: pass
verified_by: `go test ./internal/login/ -run TestLoginDigestNegotiation -v -race` (pass)

### 16. Both Digests CRC32C (INTEG-03)
expected: When both digests negotiated as CRC32C, transport.Conn.SetDigests(true, true) called after login
result: pass
verified_by: `go test ./internal/login/ -run TestLoginDigestBothCRC32C -v -race` (pass)

### 17. Custom Operational Parameters (LOGIN-06)
expected: Login negotiates MaxBurstLength, FirstBurstLength to min(initiator, target) per NumericalMin type
result: pass
verified_by: `go test ./internal/login/ -run TestLoginCustomOperationalParams -v -race` (pass)

### 18. Login Target Error Handling
expected: Target returning StatusClass=3 produces LoginError accessible via errors.As()
result: pass
verified_by: `go test ./internal/login/ -run TestLoginTargetError -v -race` (pass)

### 19. Login Context Cancellation
expected: Login respects context.Context cancellation, returns error promptly when context is cancelled
result: pass
verified_by: `go test ./internal/login/ -run TestLoginContextCancellation -v -race` (pass)

### 20. Full Suite Under Race Detector
expected: All 37 tests pass with -race flag, no data races detected
result: pass
verified_by: `go test ./internal/login/ -race -count=1` (ok, 1.267s) + `go vet ./internal/login/` (clean)

## Summary

total: 20
passed: 20
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

[none]
