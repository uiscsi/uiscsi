---
phase: 02-connection-and-login
verified: 2026-03-31T00:00:00Z
status: passed
score: 15/15 must-haves verified
re_verification: false
---

# Phase 02: Connection and Login Verification Report

**Phase Goal:** A Go application can establish an authenticated iSCSI connection with full operational parameter negotiation, including digest settings
**Verified:** 2026-03-31
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Text key=value pairs round-trip through encode/decode with byte-perfect fidelity | VERIFIED | `TestTextCodecRoundTrip`, `TestTextCodecByteExact` pass; `EncodeTextKV`/`DecodeTextKV` in textcodec.go |
| 2 | Negotiation engine resolves all five negotiation types correctly | VERIFIED | `TestNegotiationBooleanAnd` (4 subtests), `TestNegotiationBooleanOr` (4 subtests), `TestNegotiationNumericalMin`, `TestNegotiationNumericalMax`, `TestNegotiationListSelect`, `TestNegotiationDeclarative` all pass |
| 3 | NegotiatedParams struct has typed fields for all 14 mandatory RFC 7143 Section 13 keys | VERIFIED | `params.go` has all 14 fields plus `TargetName` and `TSIH`; `TestDefaults` passes |
| 4 | LoginError exposes StatusClass and StatusDetail and works with errors.As() | VERIFIED | `errors.go` has `LoginError` struct with both fields; `TestLoginErrorAs` passes |
| 5 | Parameterized tests cover the full negotiation matrix per TEST-04 | VERIFIED | `negotiation_test.go` has 10 test functions covering all 6 negotiation types; `TestKeyRegistryCompleteness` validates all 14 keys |
| 6 | CHAP response computation produces correct MD5 hash for known test vectors | VERIFIED | `chap.go` implements `chapResponse(id, secret, challenge)` as `MD5(id||secret||challenge)`; `TestChapResponse` passes |
| 7 | CHAP binary values encode with 0x prefix hex and decode both 0x hex and 0b base64 | VERIFIED | `encodeCHAPBinary`/`decodeCHAPBinary` in chap.go; `TestEncodeCHAPBinary`, `TestDecodeCHAPBinary` pass |
| 8 | One-way CHAP state machine produces correct CHAP_N + CHAP_R from target challenge | VERIFIED | `chapState.processChallenge` in chap.go; `TestCHAPExchangeOneWay` passes |
| 9 | Mutual CHAP generates initiator challenge and verifies target response | VERIFIED | `chapState.verifyMutualResponse` in chap.go; `TestCHAPExchangeMutual` passes |
| 10 | Login with AuthMethod=None against a mock target succeeds | VERIFIED | `TestLoginAuthNone` passes; mock target wired to `transport.ReadRawPDU`/`WriteRawPDU` |
| 11 | Login with CHAP succeeds with correct credentials and fails with wrong credentials | VERIFIED | `TestLoginCHAP` and `TestLoginCHAPWrongPassword` pass; wrong credentials produce `LoginError{StatusClass:2, StatusDetail:1}` |
| 12 | Login with mutual CHAP succeeds and verifies target identity | VERIFIED | `TestLoginMutualCHAP` and `TestLoginMutualCHAPTargetAuthFail` pass |
| 13 | After successful login, NegotiatedParams has correct values from target responses | VERIFIED | `TestLoginCustomOperationalParams` verifies `MaxBurstLength=131072`, `FirstBurstLength=32768` |
| 14 | Digest negotiation activates CRC32C settings on transport after login | VERIFIED | `login.go` line 147-148: `tc.SetDigests(ls.params.HeaderDigest, ls.params.DataDigest)` and `tc.SetMaxRecvDSL(...)` called post-login; `TestLoginDigestNegotiation`, `TestLoginDigestBothCRC32C` pass |
| 15 | Login failure returns LoginError with correct StatusClass and StatusDetail | VERIFIED | `TestLoginTargetError` passes; `doSecurityNegotiation`/`doOperationalNegotiation` both return `&LoginError{...}` on non-zero StatusClass |

**Score:** 15/15 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/login/textcodec.go` | Text key=value null-byte codec | VERIFIED | Exports `EncodeTextKV`, `DecodeTextKV`, `KeyValue`; 57 lines, substantive |
| `internal/login/negotiation.go` | Declarative key registry and negotiation engine | VERIFIED | Exports `NegotiationType`, `KeyDef`, `keyRegistry`, `resolveKey`, `applyNegotiatedKeys`; 204 lines |
| `internal/login/params.go` | NegotiatedParams struct and defaults | VERIFIED | Exports `NegotiatedParams` (16 typed fields), `Defaults()`; 44 lines |
| `internal/login/errors.go` | LoginError type and status constants | VERIFIED | Exports `LoginError`, all 11 status constants, `statusMessage`; 63 lines |
| `internal/login/chap.go` | CHAP authentication logic | VERIFIED | Exports `chapResponse`, `encodeCHAPBinary`, `decodeCHAPBinary`, `chapState`, `newCHAPState`, `processChallenge`, `verifyMutualResponse`; 159 lines |
| `internal/login/login.go` | Login function, functional options, login state machine | VERIFIED | Exports `Login`, `LoginOption`, `WithTarget`, `WithCHAP`, `WithMutualCHAP`, `WithHeaderDigest`, `WithDataDigest`; 455 lines |
| `internal/login/login_test.go` | Login integration tests with mock iSCSI target | VERIFIED | 10 test functions, `runMockTarget` helper, `errors.As` usage, `transport.ReadRawPDU` in mock; >200 lines |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `internal/login/negotiation.go` | `internal/login/params.go` | `applyNegotiatedKeys` populates `NegotiatedParams` | WIRED | `applyNegotiatedKeys(params *NegotiatedParams, keys []KeyValue)` at line 156 |
| `internal/login/negotiation.go` | `internal/login/textcodec.go` | Uses `DecodeTextKV` type `KeyValue` | WIRED | `KeyValue` type used throughout; `DecodeTextKV` called in login.go via negotiation flow |
| `internal/login/login.go` | `internal/transport/conn.go` | `Login` accepts `*transport.Conn`, calls `SetDigests`/`SetMaxRecvDSL` | WIRED | Lines 107, 147-148; `tc.SetDigests(...)` and `tc.SetMaxRecvDSL(...)` after login |
| `internal/login/login.go` | `internal/transport/framer.go` | Synchronous `ReadRawPDU`/`WriteRawPDU` (not pumps) | WIRED | Lines 393, 398; `transport.WriteRawPDU(ls.conn, raw)` and `transport.ReadRawPDU(ls.conn, false, false)` |
| `internal/login/login.go` | `internal/login/negotiation.go` | Uses `buildInitiatorKeys` and `applyNegotiatedKeys` | WIRED | Lines 293, 321, 344; all three functions called in `doOperationalNegotiation` |
| `internal/login/login.go` | `internal/login/chap.go` | Uses `chapState` for CHAP authentication | WIRED | Lines 139, 244, 271; `newCHAPState`, `processChallenge`, `verifyMutualResponse` all called |
| `internal/login/login.go` | `internal/pdu` | Builds `LoginReq`, parses `LoginResp` | WIRED | Lines 357-405; `pdu.LoginReq{...}`, `pdu.EncodePDU(req)`, `pdu.LoginResp{}`, `resp.UnmarshalBHS(...)` |
| `internal/login/chap.go` | `crypto/md5` | MD5(id_byte || secret || challenge) per RFC 1994 | WIRED | `md5.New()` at line 19; hash written in correct byte order |
| `internal/login/chap.go` | `encoding/hex` | Hex encoding/decoding of CHAP_C and CHAP_R values | WIRED | `hex.EncodeToString` at line 31, `hex.DecodeString` at line 41 |

### Data-Flow Trace (Level 4)

This phase produces no user-visible rendering components. All artifacts are library functions with clear data flows: text codec produces `[]KeyValue` from wire bytes; negotiation engine reads `keyRegistry` and populates `NegotiatedParams` fields; CHAP state machine reads target keys and writes response keys; login state machine reads `NegotiatedParams` and calls transport setters. All data flows are exercised by the test suite against a mock target over loopback TCP. No hollow props or disconnected data paths found.

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| All login tests pass under race detector | `go test ./internal/login/ -race -count=1` | `ok github.com/rkujawa/uiscsi/internal/login 1.262s` | PASS |
| Full test suite passes | `go test ./... -race -count=1` | All 5 packages pass | PASS |
| Negotiation matrix fully covered | `go test -run "TestNegotiation|TestNegotiate|TestFirstBurst|TestKeyRegistry" -v` | 10 test functions, all subtests pass | PASS |
| go vet clean | `go vet ./internal/login/` | No output (clean) | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| LOGIN-01 | 02-03-PLAN.md | Full login phase state machine | SATISFIED | `loginState.run()` drives stages 0->1->3 in login.go; `TestLoginAuthNone`, `TestLoginCHAP` verify end-to-end flow |
| LOGIN-02 | 02-01-PLAN.md | Text key-value negotiation engine for all RFC 7143 Section 13 mandatory keys | SATISFIED | `keyRegistry` in negotiation.go lists all 14 keys; `TestKeyRegistryCompleteness` verifies by name |
| LOGIN-03 | 02-03-PLAN.md | AuthMethod=None authentication | SATISFIED | `doSecurityNegotiation` handles `chapUser == ""` path; `TestLoginAuthNone` passes |
| LOGIN-04 | 02-02-PLAN.md | CHAP authentication (one-way) | SATISFIED | `chapState.processChallenge` computes `MD5(id||secret||challenge)`; `TestLoginCHAP` and `TestLoginCHAPWrongPassword` pass |
| LOGIN-05 | 02-02-PLAN.md | Mutual CHAP authentication | SATISFIED | `chapState.verifyMutualResponse` with `subtle.ConstantTimeCompare`; `TestLoginMutualCHAP` and `TestLoginMutualCHAPTargetAuthFail` pass |
| LOGIN-06 | 02-01-PLAN.md | Operational parameter negotiation (all 14 keys) | SATISFIED | `buildInitiatorKeys` proposes all 14 keys; `doOperationalNegotiation` resolves via `resolveKey` and `applyNegotiatedKeys`; `TestLoginCustomOperationalParams` and `TestNegotiateFullParams` verify |
| INTEG-01 | 02-03-PLAN.md | Header digest negotiation and CRC32C | SATISFIED | `tc.SetDigests(ls.params.HeaderDigest, ls.params.DataDigest)` in Login(); `TestLoginDigestNegotiation` verifies `params.HeaderDigest == true` after `WithHeaderDigest("CRC32C", "None")` |
| INTEG-02 | 02-03-PLAN.md | Data digest negotiation and CRC32C | SATISFIED | Same `SetDigests` call; `TestLoginDigestBothCRC32C` verifies `params.DataDigest == true` |
| INTEG-03 | 02-03-PLAN.md | Digest generation on outgoing PDUs when negotiated | SATISFIED | `tc.SetDigests` activates digest computation in transport layer after login completes |
| TEST-04 | 02-01-PLAN.md | Parameterized tests for negotiation parameter matrix | SATISFIED | `negotiation_test.go` has 10 test functions; BooleanAnd/BooleanOr have 4 subtests each; NumericalMin/Max have boundary cases; ListSelect covers no-overlap error path |

**Note on REQUIREMENTS.md status fields:** LOGIN-02, LOGIN-06, and TEST-04 are marked `[ ]` (pending) in REQUIREMENTS.md, but the implementation is complete and all tests pass. This is a stale documentation state — the code satisfies all three requirements. REQUIREMENTS.md should be updated to mark these as `[x]`.

### Anti-Patterns Found

None. Scanned all 10 files in `internal/login/` for `TODO`, `FIXME`, `XXX`, `HACK`, `PLACEHOLDER`, `return null`, `return {}`, empty handlers. No issues found. `go vet` reports clean.

### Human Verification Required

None required. All behaviors are testable with the mock target harness. The integration tests exercise the full state machine against loopback TCP, covering:
- AuthMethod=None
- One-way CHAP with correct and wrong credentials
- Mutual CHAP with correct and wrong target responses
- Header and data digest negotiation
- Custom operational parameter negotiation
- Target error responses
- Context cancellation

### Gaps Summary

No gaps. All 15 observable truths are verified. All 10 required files exist with substantive implementations. All key links are wired. No anti-patterns. Full test suite passes under `-race`.

The only documentation issue is that REQUIREMENTS.md still marks LOGIN-02, LOGIN-06, and TEST-04 as pending — these should be updated to `[x]` since the implementation satisfies them.

---

_Verified: 2026-03-31_
_Verifier: Claude (gsd-verifier)_
