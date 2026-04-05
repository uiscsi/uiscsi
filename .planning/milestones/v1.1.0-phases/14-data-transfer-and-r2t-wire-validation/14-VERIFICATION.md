---
phase: 14-data-transfer-and-r2t-wire-validation
verified: 2026-04-05T01:24:15Z
status: passed
score: 17/18 must-haves verified
re_verification: false
gaps:
  - id: R2T-03
    description: "DataSequenceInOrder=No cannot be negotiated (initiator hardcodes Yes with BooleanOr semantics). TestR2T_MultipleR2T covers multi-R2T isolation under Yes only. Tracked as open gap for future initiator enhancement."
    severity: low
    resolution: deferred
human_verification:
  - test: "R2T-03 partial coverage: verify that the DataSequenceInOrder=No gap is acceptable for this phase and that R2T-03 is considered satisfied under DataSequenceInOrder=Yes constraint"
    expected: "Team decision on whether TestR2T_MultipleR2T (which documents the limitation via t.Log) constitutes sufficient R2T-03 coverage given the initiator hardcodes DataSequenceInOrder=Yes"
    why_human: "The plan explicitly acknowledges this limitation. No later milestone phase addresses DataSequenceInOrder=No support. Whether partial coverage passes the R2T-03 requirement is a policy decision, not a code fact."
---

# Phase 14: Data Transfer and R2T Wire Validation Verification Report

**Phase Goal:** All Data-Out and Data-In PDU fields are verified at the wire level -- DataSN, F-bit, Buffer Offset, TTT echo, burst lengths, and R2T fulfillment ordering
**Verified:** 2026-04-05T01:24:15Z
**Status:** passed (R2T-03 partial — tracked as open gap)
**Re-verification:** No -- initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Tests verify Data-Out DataSN starts at 0 and increments correctly per R2T sequence, with F-bit set on final PDU of each sequence | VERIFIED | TestDataOut_DataSN asserts DataSN 0,1,2,3 across 4 PDUs; TestDataOut_FBitSolicited asserts Final=false on first two, Final=true on last |
| 2 | Tests verify unsolicited data respects FirstBurstLength and solicited data respects MaxBurstLength/MaxRecvDataSegmentLength, with correct mode behavior for all ImmediateData/InitialR2T combinations | VERIFIED | TestDataOut_UnsolicitedFirstBurst, TestDataOut_NoUnsolicited, TestDataOut_FirstBurstLimit, TestDataOut_FBitUnsolicited, TestDataOut_MaxRecvDSL -- all 4 ImmediateData/InitialR2T combinations covered |
| 3 | Tests verify Data-Out echoes Target Transfer Tag from R2T and Buffer Offset increases correctly across PDUs | VERIFIED | TestDataOut_TTTEcho asserts TTT=0xDEADBEEF echoed; TestDataOut_BufferOffset asserts offsets 0,512,1024,1536 |
| 4 | Tests verify Data-In with S+F status acceptance, A-bit SNACK DataACK trigger, and zero-length DataSegmentLength handling | VERIFIED | TestDataIn_StatusInFinal, TestDataIn_ABitDataACK (with ERL=1 bilateral config, asserts SNACK Type=2 BegRun=2), TestDataIn_ZeroLength all pass |
| 5 | Tests verify R2T fulfillment with correct single-PDU and multi-PDU responses, including out-of-order and parallel command scenarios | PARTIAL | TestR2T_SinglePDU, TestR2T_MultiPDU, TestR2T_ParallelCommand pass fully. TestR2T_MultipleR2T covers multi-R2T under DataSequenceInOrder=Yes only -- out-of-order (DataSequenceInOrder=No) cannot be tested because initiator hardcodes DataSequenceInOrder=Yes with BooleanOr negotiation semantics. Gap documented via t.Log in test. No later phase addresses this. |
| 6 | MockTarget can control login negotiation parameters (ImmediateData, InitialR2T, FirstBurstLength, MaxBurstLength, MaxRecvDataSegmentLength, ErrorRecoveryLevel) | VERIFIED | NegotiationConfig struct with 6 pointer fields exists in test/target.go:130; HandleLogin case 1 (line 431-484) checks each field before echoing |
| 7 | MockTarget can receive and accumulate Data-Out PDUs from initiator during write sequences | VERIFIED | ReadDataOutPDUs function exists at test/target.go:879, used in all write-path tests |
| 8 | MockTarget can send multi-PDU Data-In responses with correct DataSN, BufferOffset, F-bit, and S-bit | VERIFIED | HandleSCSIReadMultiPDU exists at test/target.go:788, iterates chunks tracking DataSN/offset, sets HasStatus+StatSN on final |
| 9 | MockTarget can generate R2T sequences with correct TTT, R2TSN, BufferOffset, and DesiredDataTransferLength | VERIFIED | SendR2TSequence function at test/target.go:837 |
| 10 | Existing tests still pass after MockTarget extensions | VERIFIED | go test ./test/conformance/ -count=1 -timeout 60s -race passes (13.3s) |
| 11 | A-bit DataACK SNACK is sent when Data-In has Acknowledge=true at ERL>=1 | VERIFIED | internal/session/datain.go:104 checks din.Acknowledge && t.erl >= 1; internal/session/session.go:149 populates task.erl from s.params.ErrorRecoveryLevel |
| 12 | Test verifies DataSN resets to 0 for each R2T sequence | VERIFIED | TestDataOut_DataSNPerR2T groups by TTT, asserts DataSN 0,1 for first burst and 0,1 for second burst |
| 13 | Test verifies EDTL matches actual transfer | VERIFIED | TestDataIn_EDTL asserts sum of all DataSegmentLen values == 1024 and Read() returns exactly 1024 bytes |
| 14 | Test verifies no unsolicited data in R2T-only mode | VERIFIED | TestDataOut_NoUnsolicited asserts SCSI Command DataSegmentLen==0 and no TTT=0xFFFFFFFF Data-Out PDUs |
| 15 | Test verifies TTT echo from R2T in Data-Out | VERIFIED | TestDataOut_TTTEcho asserts TTT=0xDEADBEEF on every captured Data-Out |
| 16 | Test verifies multi-PDU R2T response fields | VERIFIED | TestR2T_MultiPDU asserts 4 PDUs, DataSN 0-3, BufferOffset 0/256/512/768, Final only on last |
| 17 | Test verifies parallel command R2T per-command isolation | VERIFIED | TestR2T_ParallelCommand asserts Data-Out ITTs match command ITTs for each distinct TTT group |
| 18 | Test verifies R2T fulfillment ordering when DataSequenceInOrder=No | PARTIAL | Impossible with current initiator (hardcodes DataSequenceInOrder=Yes, BooleanOr semantics). TestR2T_MultipleR2T documents this via t.Log and tests sequential ordering under DataSequenceInOrder=Yes. |

**Score:** 17/18 truths verified (1 partial: R2T-03 DataSequenceInOrder=No)

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `test/target.go` | NegotiationConfig, SetNegotiationConfig, ReadPDU, HandleSCSIReadMultiPDU, SendR2TSequence, ReadDataOutPDUs, BoolPtr, Uint32Ptr | VERIFIED | 977 lines, all 8 required symbols present |
| `test/conformance/dataout_test.go` | 10 Data-Out wire conformance tests | VERIFIED | 1055 lines, 10 test functions covering DATA-01,02,03,04,05,08,10,11,12,13 |
| `test/conformance/datain_test.go` | 4 Data-In wire conformance tests | VERIFIED | 422 lines, 4 test functions covering DATA-06,07,09,14 |
| `test/conformance/r2t_test.go` | 4 R2T fulfillment conformance tests | VERIFIED (partial) | 553 lines, 4 test functions covering R2T-01,02,03(partial),04 |
| `internal/session/datain.go` | A-bit DataACK SNACK path | VERIFIED | Line 104: `if din.Acknowledge && t.erl >= 1 && t.getWriteCh != nil` sends SNACKTypeDataACK |
| `internal/session/session.go` | task.erl populated from session params | VERIFIED | Line 149: `tk.erl = uint32(s.params.ErrorRecoveryLevel)` |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| test/target.go NegotiationConfig | HandleLogin operational phase | config fields override echoed values | VERIFIED | Lines 434-484 read negCfg and check each field before echoing |
| test/target.go ReadPDU | transport.ReadRawPDU | TargetConn method wrapping ReadRawPDU | VERIFIED | Line 68: `func (tc *TargetConn) ReadPDU()` calls `transport.ReadRawPDU` |
| test/conformance/dataout_test.go | test/target.go | SetNegotiationConfig + HandleSCSIFunc + ReadDataOutPDUs | VERIFIED | Every dataout test uses SetNegotiationConfig and bilateral WithOperationalOverrides |
| test/conformance/dataout_test.go | test/pducapture/capture.go | rec.Sent(pdu.OpDataOut) for field assertions | VERIFIED | All 10 tests use rec.Sent for captured PDU field assertions |
| test/conformance/datain_test.go | test/target.go | MockTarget HandleSCSIFunc for multi-PDU Data-In | VERIFIED | TestDataIn_ABitDataACK uses SetNegotiationConfig(ERL=1) + HandleSCSIFunc |
| test/conformance/r2t_test.go | test/target.go | SendR2TSequence and ReadDataOutPDUs | VERIFIED | r2t_test.go uses ReadDataOutPDUs in all 4 tests; custom R2T sending (not SendR2TSequence -- tests send R2Ts inline) |

### Data-Flow Trace (Level 4)

These are test files -- they produce assertions about protocol behavior rather than rendering dynamic data. Data flows from mock target through initiator to captured PDUs verified by assertions.

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| `test/conformance/dataout_test.go` | `douts := rec.Sent(pdu.OpDataOut)` | pducapture.Recorder capturing real wire PDUs | Yes -- initiator sends actual PDUs via net.Pipe | FLOWING |
| `test/conformance/datain_test.go` | `snacks := rec.Sent(pdu.OpSNACKReq)` | pducapture.Recorder capturing real wire PDUs | Yes -- initiator sends SNACK after A-bit received | FLOWING |
| `test/conformance/r2t_test.go` | `outs := rec.Sent(pdu.OpDataOut)` | pducapture.Recorder capturing real wire PDUs | Yes -- full TCP connection via net.Pipe | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| All 18 phase 14 conformance tests pass | `go test ./test/conformance/ -run "TestDataOut\|TestDataIn\|TestR2T" -count=1 -timeout 60s -race` | 18/18 PASS (0.00s each) | PASS |
| Session package tests pass after datain.go fix | `go test ./internal/session/... -count=1 -timeout 30s -race` | ok (18.2s) | PASS |
| Full conformance suite still passes | `go test ./test/conformance/ -count=1 -timeout 60s -race` | ok (13.3s) | PASS |
| Build succeeds | `go build ./test/` | no output (success) | PASS |
| go vet passes | `go vet ./...` | no output (success) | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| DATA-01 | 14-02 | Data-Out DataSN starts at 0 and increments per R2T sequence | SATISFIED | TestDataOut_DataSN asserts DataSN 0,1,2,3 across 4 PDUs |
| DATA-02 | 14-02 | Unsolicited data respects FirstBurstLength with solicited R2T follow-up | SATISFIED | TestDataOut_UnsolicitedFirstBurst verifies sum of immediate+unsolicited <= FBL=1024 |
| DATA-03 | 14-02 | No unsolicited data in R2T-only mode | SATISFIED | TestDataOut_NoUnsolicited verifies DataSegmentLen=0 in SCSI Command and no TTT=0xFFFFFFFF PDUs |
| DATA-04 | 14-02 | Unsolicited Data-Out PDUs respect FirstBurstLength | SATISFIED | TestDataOut_FirstBurstLimit verifies total == FBL=768 |
| DATA-05 | 14-02 | Data-Out echoes Target Transfer Tag from R2T | SATISFIED | TestDataOut_TTTEcho asserts TTT=0xDEADBEEF on all Data-Out |
| DATA-06 | 14-03 | Status accepted in final Data-In PDU with S+F bits | SATISFIED | TestDataIn_StatusInFinal verifies S+F on last PDU only |
| DATA-07 | 14-03 | Data-In A-bit triggers SNACK DataACK at ERL>=1 | SATISFIED | TestDataIn_ABitDataACK asserts SNACK Type=2, BegRun=2 at ERL=1 |
| DATA-08 | 14-02 | Data-Out respects target MaxRecvDataSegmentLength | SATISFIED | TestDataOut_MaxRecvDSL asserts each segment <= 256 |
| DATA-09 | 14-03 | Initiator accepts Data-In with DataSegmentLength=0 | SATISFIED | TestDataIn_ZeroLength verifies no error, returns 512 bytes |
| DATA-10 | 14-02 | F bit set on last unsolicited Data-Out PDU | SATISFIED | TestDataOut_FBitUnsolicited verifies Final=true only on last TTT=0xFFFFFFFF PDU |
| DATA-11 | 14-02 | F bit set on last solicited Data-Out PDU | SATISFIED | TestDataOut_FBitSolicited verifies Final=false on first two, Final=true on last |
| DATA-12 | 14-02 | DataSN per R2T sequence in Data-Out | SATISFIED | TestDataOut_DataSNPerR2T verifies per-burst reset (0,1 then 0,1) |
| DATA-13 | 14-02 | Buffer Offset increases correctly in Data-Out | SATISFIED | TestDataOut_BufferOffset verifies offsets 0,512,1024,1536 |
| DATA-14 | 14-03 | Expected Data Transfer Length matches actual transfer | SATISFIED | TestDataIn_EDTL verifies sum of DataSegmentLen == EDTL == 1024 |
| R2T-01 | 14-04 | Single Data-Out response to R2T with correct TTT, offset, length | SATISFIED | TestR2T_SinglePDU asserts 1 PDU, TTT=0x100, BufferOffset=0, DataSN=0, Final=true, DataSegmentLen=512 |
| R2T-02 | 14-04 | Multi-PDU response to R2T with F bit and continuous offsets | SATISFIED | TestR2T_MultiPDU asserts 4 PDUs, DataSN 0-3, BufferOffset 0/256/512/768, F-bit on last only |
| R2T-03 | 14-04 | R2T fulfillment order when DataSequenceInOrder=No | PARTIAL | TestR2T_MultipleR2T covers multi-R2T under DataSequenceInOrder=Yes only. Initiator hardcodes DataSequenceInOrder=Yes (BooleanOr semantics) making DataSequenceInOrder=No untestable. Gap documented via t.Log. |
| R2T-04 | 14-04 | Parallel command R2T fulfillment ordering | SATISFIED | TestR2T_ParallelCommand validates per-command ITT/TTT isolation with sequential writes (deadlock prevention) |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| None found | - | - | - | - |

No TODO/FIXME/PLACEHOLDER comments, no empty implementations, no hardcoded empty data returned to callers.

### Human Verification Required

#### 1. R2T-03 Partial Coverage Assessment

**Test:** Review TestR2T_MultipleR2T in test/conformance/r2t_test.go and the documented limitation
**Expected:** Team to decide whether partial R2T-03 coverage (DataSequenceInOrder=Yes only) satisfies the requirement, or whether R2T-03 should remain open and a future phase should add DataSequenceInOrder=No support to the initiator
**Why human:** The plan explicitly acknowledged this limitation (plan 04 acceptance criteria: "MultipleR2T includes t.Log documenting DataSequenceInOrder=No gap"). The test passes and documents the constraint. Whether this is "good enough" for milestone v1.1 or should be tracked as a gap requiring initiator changes is a product/team decision, not a code fact. No later milestone phase addresses DataSequenceInOrder=No initiator support.

### Gaps Summary

One partial coverage situation requiring human decision:

**R2T-03 DataSequenceInOrder=No**: RFC 7143 FFP #12.3 specifies R2T fulfillment order testing when DataSequenceInOrder=No. The current initiator hardcodes DataSequenceInOrder=Yes in login proposals (internal/login/login.go), and BooleanOr negotiation semantics mean the result is always Yes even if the target proposes No. TestR2T_MultipleR2T therefore tests multi-R2T isolation under DataSequenceInOrder=Yes only, which is a valid but partial coverage. The test passes, the gap is clearly documented via t.Log. No later milestone phase (15-19) covers adding DataSequenceInOrder=No proposal support to the initiator.

All other 17 truths are fully verified with passing tests and field-level assertions.

---

_Verified: 2026-04-05T01:24:15Z_
_Verifier: Claude (gsd-verifier)_
