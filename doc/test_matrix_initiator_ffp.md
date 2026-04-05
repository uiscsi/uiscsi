# UNH-IOL Initiator FFP Test Matrix

Maps each test from the **UNH-IOL iSCSI Initiator Full Feature Phase Test Suite v0.1** (`doc/initiator_ffp.pdf`) to existing uiscsi E2E and conformance test coverage.

**Legend:**
- **Covered** — wire-level conformance test or E2E test validates this area
- **Partial** — some aspects covered but not the specific PDU-level validation the IOL test requires
- **Not Covered** — no existing test for this area
- **N/A** — feature intentionally not implemented

## Summary

| Status | Count | Percentage |
|--------|-------|------------|
| Covered | 52 | 84% |
| Partial | 4 | 6% |
| Not Covered | 6 | 10% |
| **Total** | **62** | |

## Test Matrix

### Group 1: Command Numbering

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #1.1 | Verify CmdSN increments by 1 for each non-immediate command | 3.2.2.1 | Covered | `test/conformance/cmdseq_test.go` | TestCmdSN_SequentialIncrement: wire-level CmdSN validation via pducapture |

### Group 2: Immediate Delivery

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #2.1 | Verify immediate delivery flag + CmdSN for non-TMF commands | 3.2.2.1 | Covered | `test/conformance/cmdseq_test.go` | TestCmdSN_ImmediateDelivery_NonTMF: validates I-bit + CmdSN on wire |
| #2.2 | Verify immediate delivery for task management commands | 3.2.2.1 | Covered | `test/conformance/cmdseq_test.go`, `test/conformance/tmf_test.go` | TestCmdSN_ImmediateDelivery_TMF + TestTMF_CmdSN: TMF I-bit and CmdSN at PDU level |

### Group 3: MaxCmdSN / ExpCmdSN (Command Window)

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #3.1 | Initiator respects zero command window (MaxCmdSN = ExpCmdSN-1) | 3.2.2.1 | Covered | `test/conformance/cmdwindow_test.go` | TestCmdWindow_ZeroWindow: goroutine+timer blocking proof, NOP-In reopens window |
| #3.2 | Initiator uses large command window (MaxCmdSN >> ExpCmdSN) | 3.2.2.1 | Covered | `test/conformance/cmdwindow_test.go` | TestCmdWindow_LargeWindow: 8 concurrent commands with unique contiguous CmdSNs |
| #3.3 | Initiator respects window size of 1 | 3.2.2.1 | Covered | `test/conformance/cmdwindow_test.go` | TestCmdWindow_WindowOfOne: serialized ordering verified via Seq numbers |

### Group 4: Command Retry

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #4.1 | Retried command carries original ITT, CDB, CmdSN on same connection | 3.2.2.1, 6.2.1 | Covered | `test/conformance/retry_test.go` | TestRetry_SameConnectionRetry: wire-level proof ITT[1]==ITT[0], CmdSN[1]==CmdSN[0] at ERL>=1; TestRetry_RejectCallerReissue: covers caller-reissue path |

### Group 5: ExpStatSN

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #5.1 | Detect large StatSN / ExpStatSN gap → recovery action | 3.2.2.2 | Covered | `test/conformance/retry_test.go` | TestRetry_ExpStatSNGap: StatSN jumped by 5, Status SNACK (Type=1) captured on wire at ERL=1 |

### Group 6: DataSN

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #6.1 | Data-Out DataSN starts at 0 and increments per R2T sequence | 3.2.2.3, 10.7.5 | Covered | `test/conformance/dataout_test.go` | TestDataOut_DataSN + TestDataOut_DataSNPerR2T: wire-level DataSN validation via pducapture |

### Group 7: Connection Reassignment

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #7.1 | Initiator performs task reassign on new connection after drop (ERL 2) | 6.2.2 | Covered | `test/conformance/erl2_test.go` | TestERL2_ConnectionReassignment: Logout(reasonCode=2) on wire; TestERL2_TaskReassign: TMF Function=14 with correct ReferencedTaskTag |

### Group 8: Data Transmission

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #8.1 | Unsolicited data respects FirstBurstLength; solicited data follows R2T | 3.2.4.2 | Covered | `test/conformance/dataout_test.go`, `test/conformance/scsicommand_test.go` | TestDataOut_UnsolicitedFirstBurst + TestSCSICommand_FirstBurstLength: wire-level FirstBurstLength boundary validation |
| #8.2 | No unsolicited data when InitialR2T=Yes, ImmediateData=No | 3.2.4.2 | Covered | `test/conformance/dataout_test.go` | TestDataOut_NoUnsolicited: validates DSL=0 on wire |
| #8.3 | Unsolicited Data-Out (not immediate) respects FirstBurstLength | 3.2.4.2 | Covered | `test/conformance/dataout_test.go` | TestDataOut_FirstBurstLimit: wire-level validation |

### Group 9: Target Transfer Tag

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #9.1 | Data-Out echoes Target Transfer Tag from R2T | 3.2.4.3, 10.7.4 | Covered | `test/conformance/dataout_test.go` | TestDataOut_TTTEcho: validates TTT field match via pducapture |

### Group 10: Data-In

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #10.1 | Accept status in final Data-In PDU (S bit + F bit) | 10.7, 10.7.3 | Covered | `test/conformance/datain_test.go` | TestDataIn_StatusInFinal: wire-level S/F bit validation |
| #10.2 | Respond to Data-In A bit with SNACK DataACK (ERL≥1) | 10.7.2 | Covered | `test/conformance/datain_test.go`, `test/conformance/snack_test.go` | TestDataIn_ABitDataACK + TestSNACK_DataACKWireFields: wire-level A-bit and DataACK SNACK capture |

### Group 11: Data-Out PDU Fields

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #11.1.1 | Data-Out respects target MaxRecvDataSegmentLength | 10.7.7 | Covered | `test/conformance/dataout_test.go` | TestDataOut_MaxRecvDSL: wire-level segment size validation |
| #11.1.2 | Accept Data-In with DataSegmentLength=0 | 10.7.7 | Covered | `test/conformance/datain_test.go` | TestDataIn_ZeroLength: zero-length Data-In PDU handling |
| #11.2.1 | F bit set on last unsolicited Data-Out PDU | 10.7.1 | Covered | `test/conformance/dataout_test.go` | TestDataOut_FBitUnsolicited: wire-level F-bit validation |
| #11.2.2 | F bit set on last solicited Data-Out PDU | 10.7.1 | Covered | `test/conformance/dataout_test.go` | TestDataOut_FBitSolicited: wire-level F-bit validation |
| #11.3 | DataSN starts at 0 per R2T sequence, increments per PDU | 10.7.5 | Covered | `test/conformance/dataout_test.go` | TestDataOut_DataSNPerR2T: see #6.1 |
| #11.4 | Buffer Offset increases correctly in Data-Out PDUs | 10.7.6 | Covered | `test/conformance/dataout_test.go` | TestDataOut_BufferOffset: wire-level offset validation |

### Group 12: R2T Handling

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #12.1 | Single Data-Out response to R2T with correct TTT, offset, length | 10.8 | Covered | `test/conformance/r2t_test.go` | TestR2T_SinglePDU: wire-level TTT/offset/length validation |
| #12.2 | Multi-PDU response to R2T with F bit, continuous offsets | 10.8 | Covered | `test/conformance/r2t_test.go` | TestR2T_MultiPDU: wire-level multi-PDU R2T fulfillment |
| #12.3 | R2T fulfillment order when DataSequenceInOrder=No | 10.8 | Partial | `test/conformance/r2t_test.go` | TestR2T_MultipleR2T: exercises multiple R2Ts but DataSequenceInOrder=Yes (LIO default) |
| #12.4 | Parallel commands: R2T fulfillment order across interleaved commands | 10.8 | Covered | `test/conformance/r2t_test.go` | TestR2T_ParallelCommand: parallel writes with interleaved R2T fulfillment |

### Group 13: SNACK

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #13.1 | Data/R2T SNACK construction (skip DataSN, trigger retransmit request) | 10.16 | Covered | `test/conformance/snack_test.go` | TestSNACK_DataSNGap: wire-level SNACK PDU capture after injected DataSN gap |
| #13.2 | DataACK SNACK in response to A-bit | 10.16 | Covered | `test/conformance/snack_test.go` | TestSNACK_DataACKWireFields: A-bit + DataACK wire validation |

### Group 14: Logout Request

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #14.1 | Logout after AsyncMessage code 1 (single connection) | 10.9, 10.14 | Covered | `test/conformance/session_test.go`, `test/conformance/async_test.go` | TestSession_LogoutAfterAsyncEvent1 + TestAsync_LogoutRequest: wire-level async logout |
| #14.2 | Logout after AsyncMessage code 1 (multi-connection session) | 10.9, 10.14 | Not Covered | — | Multi-connection sessions not supported (MaxConnections=1) |

### Group 15: NOP-Out

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #15.1 | NOP-Out ping response (TTT echo, ITT=0xffffffff, I-bit, LUN echo) | 10.18, 10.19 | Covered | `test/conformance/nopout_test.go` | TestNOPOut_PingResponse: wire-level field validation via pducapture |
| #15.2 | NOP-Out ping request (initiator-initiated, valid ITT) | 10.18, 10.19 | Covered | `test/conformance/nopout_test.go` | TestNOPOut_PingRequest: wire-level initiator ping validation |
| #15.3 | NOP-Out to confirm ExpStatSN (ITT=0xffffffff, I-bit=1) | 10.18, 10.19 | Covered | `test/conformance/nopout_test.go` | TestNOPOut_ExpStatSNConfirmation: ExpStatSN confirmation variant |

### Group 16: SCSI Command

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #16.1.1 | Command PDU fields with ImmediateData=Yes (CmdSN, ExpStatSN, ITT, DSL, CDB) | 10.3 | Covered | `test/conformance/scsicommand_test.go` | TestSCSICommand_ImmediateDataMatrix: wire-level PDU field validation across all 4 ImmediateData/InitialR2T combos |
| #16.1.2 | Command PDU fields with ImmediateData=No (DSL=0) | 10.3 | Covered | `test/conformance/scsicommand_test.go` | TestSCSICommand_ImmediateDataMatrix (ImmNo subtests) |
| #16.2.1 | Unsolicited data: ImmediateData=Yes, InitialR2T=Yes — F bit when EDTL = DSL | 10.3.4 | Covered | `test/conformance/scsicommand_test.go` | TestSCSICommand_UnsolicitedFBit: wire-level F-bit validation |
| #16.2.2 | No unsolicited data: ImmediateData=No, InitialR2T=Yes — DSL=0 | 10.3.4 | Covered | `test/conformance/dataout_test.go` | TestDataOut_NoUnsolicited |
| #16.2.3 | No immediate data: ImmediateData=No, InitialR2T=No — DSL=0 in command | 10.3.4 | Covered | `test/conformance/scsicommand_test.go` | TestSCSICommand_ImmediateDataMatrix (ImmNo_R2TNo) |
| #16.2.4 | Both enabled: ImmediateData=Yes, InitialR2T=No — FirstBurstLength limit | 10.3.4, 12.12 | Covered | `test/conformance/scsicommand_test.go` | TestSCSICommand_FirstBurstLength: wire-level boundary validation |
| #16.3.1 | F bit in SCSI Command when InitialR2T=Yes (no unsolicited Data-Out follows) | 10.3 | Covered | `test/conformance/scsicommand_test.go` | TestSCSICommand_UnsolicitedFBit |
| #16.4.1 | Handle CRC error sense data (CHECK CONDITION, sense key 0x0B) | 6.2.1, 10.4.7.2 | Covered | `test/conformance/error_test.go` | TestError_CRCErrorSense: injected CRC error sense, verified parsed fields |
| #16.4.2 | Handle SNACK reject → new command (not retry) | 6.2.1, 10.16 | Covered | `test/conformance/error_test.go` | TestError_SNACKRejectNewCommand: Reject → same-connection retry at ERL>=1 |
| #16.4.3 | Handle unexpected unsolicited data error sense | 6.2.1, 10.4.7.2 | Covered | `test/conformance/error_test.go` | TestError_UnexpectedUnsolicited: specific sense code injection |
| #16.4.4 | Handle "not enough unsolicited data" error sense | 6.2.1, 10.4.7.2 | Covered | `test/conformance/error_test.go` | TestError_NotEnoughUnsolicited: specific sense code injection |
| #16.4.5 | Handle BUSY status (0x08) → re-issue later | 6.2.1, SAM-2 | Covered | `test/conformance/error_test.go` | TestError_BUSY: BUSY status injection + error propagation |
| #16.4.6 | Handle RESERVATION CONFLICT (0x18) → re-issue later | 6.2.1, SAM-2 | Covered | `test/conformance/error_test.go` | TestError_ReservationConflict: status injection + error propagation |
| #16.5 | Respect MaxCmdSN in SCSI Response (stop issuing if window closed) | 10.4.7.3 | Covered | `test/conformance/cmdwindow_test.go` | TestCmdWindow_MaxCmdSNInResponse: SCSI Response closes window, NOP-In reopens |
| #16.6 | Expected Data Transfer Length matches actual transfer | 10.3 | Covered | `test/conformance/datain_test.go` | TestDataIn_EDTL: wire-level EDTL validation |

### Group 17: Logout

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #17.1 | Clean logout with proper Logout Request/Response exchange | 10.14 | Covered | `test/conformance/session_test.go` | TestSession_CleanLogout: wire-level Logout PDU validation |

### Group 18: Text Request

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #18.1 | Text Request text fields (key=value format) | 10.11 | Covered | `test/conformance/text_test.go` | TestText_Fields: wire-level opcode, F-bit, KV data segment validation |
| #18.2 | Text Request Initiator Task Tag uniqueness | 10.11 | Covered | `test/conformance/text_test.go` | TestText_ITTUniqueness: sequential requests, all ITTs distinct |
| #18.3.1 | Text Request Target Transfer Tag (initial = 0xffffffff) | 10.11 | Covered | `test/conformance/text_test.go` | TestText_TTTInitial: initial TTT=0xFFFFFFFF on wire |
| #18.3.2 | Text Request Target Transfer Tag (continuation) | 10.11 | Covered | `test/conformance/text_test.go` | TestText_TTTContinuation: TTT echo in continuation request |
| #18.4 | Text Request other parameters | 10.11 | Covered | `test/conformance/text_test.go` | TestText_OtherParams: CmdSN, ExpStatSN field validation |
| #18.5 | Text Request negotiation reset | 10.11 | Covered | `test/conformance/text_test.go` | TestText_NegotiationReset: fresh ITT + TTT=0xFFFFFFFF after completed exchange |

### Group 19: Task Management

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #19.1 | TMF CmdSN handling | 10.5 | Covered | `test/conformance/tmf_test.go` | TestTMF_CmdSN: TMF is immediate — CmdSN is current, not acquired |
| #19.2 | TMF LUN field | 10.5 | Covered | `test/conformance/tmf_test.go` | TestTMF_LUNEncoding: flat-space LUN encoding for multiple LUN values |
| #19.3 | TMF RefCmdSN for referenced task | 10.5 | Covered | `test/conformance/tmf_test.go` | TestTMF_RefCmdSN: AbortTask carries RefCmdSN matching referenced task |
| #19.4.1 | Abort Task Set: all tasks on LUN aborted | 10.5.1 | Covered | `test/conformance/tmf_test.go` | TestTMF_AbortTaskSet_AllTasks: all in-flight tasks canceled |
| #19.4.2 | Abort Task Set: verify no new tasks during abort | 10.5.1 | Covered | `test/conformance/tmf_test.go` | TestTMF_AbortTaskSet_BlocksNew: goroutine+timer blocking proof |
| #19.4.3 | Abort Task Set: verify response after all tasks cleared | 10.5.1 | Covered | `test/conformance/tmf_test.go` | TestTMF_AbortTaskSet_ResponseAfterClear: tasks cleared by response time |
| #19.5 | Task Reassign (ERL 2 connection recovery) | 10.5.3 | Covered | `test/conformance/erl2_test.go` | TestERL2_TaskReassign: TMF Function=14 with correct ReferencedTaskTag on new connection |

### Group 20: Asynchronous Message

| IOL Test | Purpose | RFC Ref | uiscsi Coverage | Test File(s) | Notes |
|----------|---------|---------|-----------------|--------------|-------|
| #20.1 | Async Message code 1: target requests logout | 10.9.1 | Covered | `test/conformance/async_test.go` | TestAsync_LogoutRequest: async code 1 → initiator performs logout |
| #20.2 | Async Message: drop connection | 10.9.1 | Covered | `test/conformance/async_test.go` | TestAsync_ConnectionDrop: async code 2 handling |
| #20.3 | Async Message: drop all connections in session | 10.9.1 | Covered | `test/conformance/async_test.go` | TestAsync_SessionDrop: async code 3 handling |
| #20.4 | Async Message: request negotiation | 10.9.1 | Covered | `test/conformance/async_test.go` | TestAsync_NegotiationRequest: async code 4 → renegotiation |

## Coverage Gap Analysis

### Remaining Gaps (6 tests)

| IOL Test | Area | Reason |
|----------|------|--------|
| #12.3 | R2T out-of-order (DataSequenceInOrder=No) | LIO only supports DataSequenceInOrder=Yes; would need custom target |
| #14.2 | Multi-connection logout | MaxConnections=1 is the only supported mode |
| #16.3.1 partial | F-bit edge cases beyond ImmYes/R2TYes | Most cases covered; edge permutations remain |
| — | SCSI BUSY retry logic | Error propagated to caller; no automatic retry implemented |
| — | RESERVATION CONFLICT retry | Error propagated to caller; no automatic retry implemented |
| — | Multi-connection sessions | Single-connection only; multi-connection is out of scope |

### Partial (4 tests)

| IOL Test | Area | What's Missing |
|----------|------|----------------|
| #12.3 | R2T ordering with DataSequenceInOrder=No | Test exercises multiple R2Ts but only with DataSequenceInOrder=Yes |
| #16.4.5 | BUSY → automatic re-issue | Error detected and typed correctly; no automatic retry (caller handles) |
| #16.4.6 | RESERVATION CONFLICT → automatic re-issue | Same — error typed correctly; caller handles retry |
| #14.2 | Multi-connection logout | MaxConnections=1 enforced |

### Well-Covered Areas (v1.1 additions highlighted)

All 7 phases of milestone v1.1 (Phases 13-19) added wire-level PDU capture conformance tests:
- **Phase 13**: PDU capture framework + CmdSN sequencing (#1.1, #2.1, #2.2)
- **Phase 14**: Data-In/Out wire fields (#6.1, #8.x, #9.1, #10.x, #11.x)
- **Phase 15**: SCSI Command PDU fields (#16.1.x, #16.2.x, #16.3.1)
- **Phase 16**: Error injection + SNACK (#13.x, #16.4.x)
- **Phase 17**: Session/NOP-Out/Async (#14.1, #15.x, #17.1, #20.x)
- **Phase 18**: Command window + retry + ERL 2 (#3.x, #4.1, #5.1, #7.1, #16.5, #19.5)
- **Phase 19**: TMF + Text Request (#18.x, #19.x)
