# RFC 7143 Compliance Matrix

Coverage audit for key MUST requirements from RFC 7143 (iSCSI: Internet Small Computer
Systems Interface (iSCSI) Protocol (Consolidated)).

This is a living document — updated as compliance work progresses. Each row traces a MUST
requirement to the implementation and test coverage that satisfies it.

**Status legend:**
- **Covered** — requirement is implemented and tested
- **Partial** — partially implemented; gap noted
- **N/A** — not applicable to an initiator-only implementation

---

## PDU Encoding/Decoding (Section 11)

| Section | MUST Requirement | Status | Implementation | Test Reference |
|---------|------------------|--------|----------------|----------------|
| 11.2.1 | BHS is exactly 48 bytes | Covered | `pdu.BHSLength = 48`; `ReadRawPDU` reads exactly 48 bytes with `io.ReadFull` | `pdu_test.go TestPDURoundTrip` |
| 11.2.1 | DataSegmentLength is 24-bit (bytes 5–7) | Covered | `encodeDataSegmentLength` stores 3 bytes; `decodeDataSegmentLength` reads 3 bytes; `ProtocolError{OversizedSegment}` on overflow | `bhs_test.go`, `pdu/errors_test.go` |
| 11.2.1 | Padding to 4-byte boundary | Covered | `PadLen(n)` in `pdu/padding.go`; `WriteRawPDU` appends zero padding; `ReadRawPDU` skips padding | `pdu_test.go TestEncodePDUPadding` |
| 11.2.1 | Reserved bits MUST be zero on send | Covered | MarshalBHS functions zero-initialize BHS before writing fields | `pdu_test.go TestPDURoundTrip` (all 18 opcodes) |
| 11.2.1 | TotalAHSLength in 4-byte words | Covered | `Header.TotalAHSLength` is in 4-byte words; stored at BHS byte 4 | `pdu_test.go TestDataSegmentLengthDoesNotCorruptTotalAHSLength` |
| 11.3.1 | Unknown opcodes MUST return Reject PDU (target) / error (initiator) | Covered (initiator) | `DecodeBHS` returns `ProtocolError{BadOpcode}` for unknown opcodes | `pdu_test.go TestDecodeBHSUnknownOpcode`, `FuzzDecodeBHS` |
| 11.4 | AHS chain must terminate | Covered | `UnmarshalAHS` iterates with length check; returns error on truncation | `ahs_test.go`, `FuzzUnmarshalAHS` |

## NOP-Out / NOP-In (Section 11.18 / 11.19)

| Section | MUST Requirement | Status | Implementation | Test Reference |
|---------|------------------|--------|----------------|----------------|
| 11.18 | NOP-Out response to NOP-In MUST set ITT=0xFFFFFFFF | Covered | `handleUnsolicitedNOPIn` sets `InitiatorTaskTag: 0xFFFFFFFF` | `async_test.go TestNOPInResponsePDUFields` |
| 11.18 | NOP-Out response MUST echo TTT from NOP-In | Covered | `handleUnsolicitedNOPIn`: `TargetTransferTag: nopin.TargetTransferTag` | `async_test.go TestNOPInResponsePDUFields`, `keepalive_test.go TestUnsolicitedNOPInResponse` |
| 11.19 | NOP-In with TTT != 0xFFFFFFFF MUST NOT be dropped | Covered | `ReadPump` sends NOP-In to unsolicitedCh with blocking select (not non-blocking) | `async_test.go TestNOPInResponsePDUFields` |
| 11.19 | Informational NOP-In (TTT=0xFFFFFFFF) requires no reply | Covered | `handleUnsolicitedNOPIn` only sends NOP-Out when `TTT != 0xFFFFFFFF` | `async_test.go TestNOPInInformational` |
| 11.18 | NOP-Out CmdSN must be current session CmdSN | Covered | `handleUnsolicitedNOPIn`: `CmdSN: s.window.current()` | `async_test.go TestNOPInResponsePDUFields` |
| 11.18 | NOP-Out ExpStatSN must be current ExpStatSN | Covered | `handleUnsolicitedNOPIn`: `ExpStatSN: s.getExpStatSN()` | `async_test.go TestNOPInResponsePDUFields` |

## SCSI Command / Response (Section 11.6 / 11.7)

| Section | MUST Requirement | Status | Implementation | Test Reference |
|---------|------------------|--------|----------------|----------------|
| 11.6.1 | SCSI Command CDB in BHS (16 bytes) or AHS (extended) | Covered | `SCSICommand.CDB [16]byte` in BHS; AHS for extended CDB | `pdu_test.go TestPDURoundTrip SCSICommand`, `ahs_test.go` |
| 11.6.1 | Read/Write bits mutually exclusive | Partial | Struct fields `Read`, `Write` are separate booleans; no runtime enforcement | `pdu_test.go TestSCSICommandWriteFlag` |
| 11.7.1 | SCSI Response must carry StatSN | Covered | `SCSIResponse.StatSN` field set by target; processed by `updateStatSN` | `pdu_test.go TestPDURoundTrip SCSIResponse` |
| 11.7.1 | Overflow and Underflow residual flags | Covered | `SCSIResponse.Overflow`, `Underflow`, `BidiOverflow`, `BidiUnderflow` round-trip | `pdu_test.go TestPDURoundTrip SCSIResponse_AllResidualFlags` |
| 11.7.1 | Sense data returned in Data Segment | Covered | `SCSIResponse.Data` carries sense; `ParseSense` handles 0x70/0x71/0x72/0x73 | `scsi/fuzz_test.go FuzzSenseData`, `scsi/fuzz_test.go FuzzParseSense` |

## Login Request / Response (Section 11.11 / 11.12)

| Section | MUST Requirement | Status | Implementation | Test Reference |
|---------|------------------|--------|----------------|----------------|
| 11.11.1 | Login Request MUST carry ISID | Covered | `LoginReq.ISID [6]byte` set during session initiation | `pdu_test.go TestPDURoundTrip LoginReq` |
| 11.11.1 | Transit bit: initiator requests phase transition | Covered | `LoginReq.Transit` bool; CSG/NSG encoding per spec | `pdu_test.go TestLoginReqCSGNSGBitPacking` |
| 11.11.1 | CSG and NSG are 2-bit fields (0=Security, 1=Operational, 3=FullFeature) | Covered | Encoded in byte 1 bits 3:2 and 1:0 | `pdu_test.go TestLoginReqCSGNSGBitPacking` |
| 11.12.1 | Login Response carries StatusClass/StatusDetail | Covered | `LoginResp.StatusClass`, `StatusDetail`; `LoginError` wraps them | `login/errors.go`, `login/login_test.go` |
| 11.12.1 | Login Response TSIH must match initiator ISID | Covered | TSIH stored and echoed in subsequent Login Requests | `pdu_test.go TestPDURoundTrip LoginResp` |
| 11.11.4 | Text parameters during login negotiated via null-delimited key=value | Covered | `EncodeTextKV` / `DecodeTextKV` null-byte delimited | `login/fuzz_test.go FuzzDecodeTextKV`, `FuzzLoginTextCodec` |

## Login Status Codes (Section 11.13)

| Section | MUST Requirement | Status | Implementation | Test Reference |
|---------|------------------|--------|----------------|----------------|
| 11.13 | StatusClass=0 means success | Covered | `negotiation.go` checks `StatusClass == 0` | `login/login_test.go` |
| 11.13 | StatusClass=1 means redirect | Covered | `LoginError{Reason: ReasonRedirect}` | `login/errors.go` |
| 11.13 | StatusClass=2 means initiator error | Covered | `LoginError{Reason: ...}` for class=2 | `login/errors.go`, `login/login_test.go` |
| 11.13 | StatusClass=3 means target error | Covered | Handled identically to class=2 with distinct reason | `login/errors.go` |

## Logout (Section 11.14 / 11.15)

| Section | MUST Requirement | Status | Implementation | Test Reference |
|---------|------------------|--------|----------------|----------------|
| 11.14.1 | Logout Request MUST specify reason code | Covered | `LogoutReq.ReasonCode`; 0x00=close session, 0x01=close connection | `pdu_test.go TestPDURoundTrip LogoutReq` |
| 11.15.1 | Logout Response Time2Wait / Time2Retain | Covered | `LogoutResp.Time2Wait`, `Time2Retain` parsed and round-trip verified | `pdu_test.go TestPDURoundTrip LogoutResp` |
| 11.14.1 | Logout must complete in-flight commands before closing | Covered | `sess.Drain()` blocks until all tasks complete before sending LogoutReq | `session/logout_test.go TestLogoutDrainsInFlight` |

## Reject PDU (Section 11.17)

| Section | MUST Requirement | Status | Implementation | Test Reference |
|---------|------------------|--------|----------------|----------------|
| 11.17.1 | Reject PDU must carry the rejected PDU BHS in data segment | Covered | `Reject.Data` holds the 48-byte rejected BHS | `pdu_test.go TestRejectWithDataSegment` |
| 11.17.1 | Reject Reason field (1 byte) | Covered | `Reject.Reason` round-trips correctly | `pdu_test.go TestPDURoundTrip Reject` |

## CmdSN Numbering (Section 3.2.2.1 / 12.1)

| Section | MUST Requirement | Status | Implementation | Test Reference |
|---------|------------------|--------|----------------|----------------|
| 3.2.2.1 | CmdSN uses RFC 1982 serial number arithmetic | Covered | `internal/serial` package implements RFC 1982 comparison | `session/cmdwindow_test.go` |
| 3.2.2.1 | MaxCmdSN window MUST be respected | Covered | `cmdWindow.acquire` blocks until window opens | `session/cmdwindow_test.go TestCmdWindowBlocking` |
| 3.2.2.1 | ExpCmdSN in responses advances the window | Covered | `s.window.update(expCmdSN, maxCmdSN)` on every response | `session/session_test.go` |

## Digest Negotiation (Section 12.2)

| Section | MUST Requirement | Status | Implementation | Test Reference |
|---------|------------------|--------|----------------|----------------|
| 12.2.1 | Header digest applies to BHS + AHS | Covered | `WriteRawPDU` / `ReadRawPDU` compute CRC32C over BHS when header digest enabled | `transport/framer.go`, `session/session_test.go TestHeaderDigestMismatch` |
| 12.2.1 | Data digest applies to data segment (no padding) | Covered | Data digest computed before padding in `WriteRawPDU` | `transport/framer.go` |
| 12.2.1 | Digest is CRC32C (not CRC32/IEEE) | Covered | `hash/crc32.Castagnoli` throughout; `internal/digest/crc32c.go` | `digest/crc32c_test.go` |

## CHAP Authentication (Section 13)

| Section | MUST Requirement | Status | Implementation | Test Reference |
|---------|------------------|--------|----------------|----------------|
| 13.1 | CHAP challenge MUST be at least 16 bytes | Covered | `validateChallenge` rejects `len < 16` with `ReasonShortChallenge` | `login/chap_test.go`, `FuzzCHAPChallenge` |
| 13.1 | CHAP challenge MUST have non-zero entropy | Covered | `validateChallenge` rejects all-zero challenge with `ReasonLowEntropy` | `login/chap_test.go TestValidateChallenge_AllZero` |
| 13.1 | CHAP response = MD5(id \|\| secret \|\| challenge) | Covered | `chapResponse` implements RFC 1994 §4.1 exactly | `login/chap_test.go TestCHAPResponse` |
| 13.1 | Mutual CHAP: target response verified constant-time | Covered | `verifyMutualResponse` uses `subtle.ConstantTimeCompare` | `login/chap_test.go TestMutualCHAP` |
| 13.2 | CHAP algorithm must be MD5 (CHAP_A=5) | Covered | `processChallenge` rejects any `CHAP_A` != "5" | `login/chap_test.go TestUnsupportedCHAPAlgorithm` |

---

## Coverage Gaps / Deferred

| Section | Requirement | Status | Notes |
|---------|-------------|--------|-------|
| 11.6.1 | Read/Write bits mutually exclusive enforcement | Partial | No runtime check prevents both being set simultaneously; currently relies on caller discipline |
| 11.10 | SNACK Request full recovery logic | Partial | SNACKReq PDU encodes/decodes correctly; session-level SNACK retransmit not exercised in unit tests |
| 3.4 | ERL 2 connection replacement full spec | Partial | Framework in `connreplace.go`; edge cases not exhaustively tested |

---

*Last updated: Phase 04 Plan 03 — Fuzz targets and RFC audit*
