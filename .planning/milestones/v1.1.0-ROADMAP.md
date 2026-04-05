# Roadmap: uiscsi

## Milestones

- ✅ **v1.0** — Phases 1-11 (shipped 2026-04-03)
- 🚧 **v1.1.0 Full Test Compliance and Coverage** — Phases 13-19 (in progress)

## Phases

<details>
<summary>✅ v1.0 (Phases 1-11) — SHIPPED 2026-04-03</summary>

- [x] Phase 1: PDU Codec and Transport (3/3 plans)
- [x] Phase 2: Connection and Login (3/3 plans)
- [x] Phase 3: Session, Read Path, and Discovery (3/3 plans)
- [x] Phase 4: Write Path (4/4 plans)
- [x] Phase 5: SCSI Command Layer (3/3 plans)
- [x] Phase 6: Error Recovery and Task Management (3/3 plans)
- [x] Phase 6.1: Observability and Debugging Infrastructure (3/3 plans)
- [x] Phase 7: Public API, Observability, and Release (3/3 plans)
- [x] Phase 8: lsscsi Discovery Utility (2/2 plans)
- [x] Phase 9: LIO E2E Tests (2/2 plans)
- [x] Phase 10: E2E Coverage Expansion (5/5 plans)
- [x] Phase 11: Audit Remediation (4/4 plans)

**Total:** 12 phases, 38 plans, 98 requirements verified

See [milestones/v1.0-ROADMAP.md](milestones/v1.0-ROADMAP.md) for full phase details.

</details>

### 🚧 v1.1.0 Full Test Compliance and Coverage (In Progress)

**Milestone Goal:** Achieve full UNH-IOL Initiator FFP test suite coverage -- promote all 66 tests from partial/not-covered to covered with wire-level validation.

**Test Strategy:** Two-tier approach using shared PDU capture/assertion framework:
- **MockTarget (conformance tests):** For tests requiring a misbehaving target (error injection, DataSN gaps, async messages, command window control). Uses `test/target.go` extended with fault injection.
- **LIO E2E tests:** For tests validating our PDU correctness on real protocol exchange (CmdSN, DataSN, F-bit, TTT, Buffer Offset). Uses `test/lio/` against real kernel target.
- **PDU capture hook:** `WithPDUHook` captures every PDU sent/received — works with both MockTarget and LIO.

- [x] **Phase 13: PDU Wire Capture Framework and Command Sequencing** - Test infrastructure for PDU-level assertions plus basic CmdSN wire validation (completed 2026-04-04)
- [x] **Phase 14: Data Transfer and R2T Wire Validation** - Data-Out/Data-In field assertions and R2T fulfillment verification on the wire (completed 2026-04-05)
- [x] **Phase 15: SCSI Command Write Mode Wire Tests** - ImmediateData/InitialR2T/FirstBurstLength matrix with PDU-level verification (completed 2026-04-05)
- [x] **Phase 16: Error Injection and SCSI Error Handling** - MockTarget error injection for status codes, sense data, SNACK reject, and DataSN gaps (completed 2026-04-05)
- [x] **Phase 17: Session Management, NOP-Out, and Async Messages** - Async message injection, NOP-Out variants, and logout wire validation (completed 2026-04-05)
- [x] **Phase 18: Command Window, Retry, and ERL 2** - Command window enforcement, command retry wire validation, and ERL 2 connection reassignment (completed 2026-04-05)
- [x] **Phase 19: Task Management and Text Negotiation** - TMF field validation, Abort Task Set behavior, and Text Request wire tests (completed 2026-04-05)

## Phase Details

### Phase 13: PDU Wire Capture Framework, MockTarget Extensions, and Command Sequencing
**Goal**: Every subsequent phase can capture PDUs on the wire and assert field-level correctness; MockTarget supports fault injection, async messages, and command window control; validated by basic CmdSN sequencing tests
**Depends on**: Nothing (first v1.1.0 phase)
**Requirements**: CMDSEQ-01, CMDSEQ-02, CMDSEQ-03
**Success Criteria** (what must be TRUE):
  1. A reusable PDU capture/assertion helper exists that records all PDUs exchanged during a test and allows field-level queries (opcode, CmdSN, DataSN, flags, etc.)
  2. MockTarget extended with: stateful session tracking (CmdSN/ExpStatSN), multi-PDU Data-In with configurable DataSN gaps, async message injection, per-command handler routing (HandleSCSIFunc pattern), and command window control (MaxCmdSN manipulation)
  3. Test validates that CmdSN increments by exactly 1 for each non-immediate SCSI command observed on the wire
  4. Test validates that immediate delivery commands carry correct CmdSN values for both SCSI and TMF opcodes
  5. All new tests pass under `go test -race`
**Plans**: 2 plans
Plans:
- [x] 13-01-PLAN.md — PDU capture framework + MockTarget extensions (HandleSCSIFunc, SessionState)
- [x] 13-02-PLAN.md — CmdSN wire conformance tests (CMDSEQ-01, CMDSEQ-02, CMDSEQ-03)

### Phase 14: Data Transfer and R2T Wire Validation
**Goal**: All Data-Out and Data-In PDU fields are verified at the wire level -- DataSN, F-bit, Buffer Offset, TTT echo, burst lengths, and R2T fulfillment ordering
**Depends on**: Phase 13 (PDU capture framework)
**Requirements**: DATA-01, DATA-02, DATA-03, DATA-04, DATA-05, DATA-06, DATA-07, DATA-08, DATA-09, DATA-10, DATA-11, DATA-12, DATA-13, DATA-14, R2T-01, R2T-02, R2T-03, R2T-04
**Success Criteria** (what must be TRUE):
  1. Tests verify Data-Out DataSN starts at 0 and increments correctly per R2T sequence, with F-bit set on final PDU of each sequence
  2. Tests verify unsolicited data respects FirstBurstLength and solicited data respects MaxBurstLength/MaxRecvDataSegmentLength, with correct mode behavior for all ImmediateData/InitialR2T combinations
  3. Tests verify Data-Out echoes Target Transfer Tag from R2T and Buffer Offset increases correctly across PDUs
  4. Tests verify Data-In with S+F status acceptance, A-bit SNACK DataACK trigger, and zero-length DataSegmentLength handling
  5. Tests verify R2T fulfillment with correct single-PDU and multi-PDU responses, including out-of-order and parallel command scenarios
**Plans**: 4 plans
Plans:
- [x] 14-01-PLAN.md — MockTarget extensions (NegotiationConfig, ReadPDU, HandleSCSIReadMultiPDU, R2T helpers)
- [x] 14-02-PLAN.md — Data-Out wire conformance tests (DATA-01,02,03,04,05,08,10,11,12,13)
- [x] 14-03-PLAN.md — Data-In wire conformance tests (DATA-06,07,09,14)
- [x] 14-04-PLAN.md — R2T fulfillment conformance tests (R2T-01,02,03,04)

### Phase 15: SCSI Command Write Mode Wire Tests
**Goal**: SCSI Command PDU fields are verified at the wire level across all ImmediateData/InitialR2T/FirstBurstLength combinations
**Depends on**: Phase 13 (PDU capture framework)
**Requirements**: SCSI-01, SCSI-02, SCSI-03, SCSI-04, SCSI-05, SCSI-06, SCSI-07
**Success Criteria** (what must be TRUE):
  1. Tests verify Command PDU fields (W-bit, F-bit, EDTL, DataSegmentLength) are correct with ImmediateData=Yes and ImmediateData=No
  2. Tests verify no unsolicited data is sent when ImmediateData=No/InitialR2T=Yes, and no immediate data when ImmediateData=No/InitialR2T=No
  3. Tests verify unsolicited data F-bit behavior when EDTL equals DataSegmentLength and when FirstBurstLength limits apply
  4. Tests verify F-bit in SCSI Command PDU when InitialR2T=Yes (no unsolicited data follows)
**Plans**: 2 plans
Plans:
- [x] 15-01-PLAN.md — Shared write-test helpers extraction (helpers_test.go)
- [x] 15-02-PLAN.md — SCSI Command PDU wire conformance tests (SCSI-01 through SCSI-07)

### Phase 16: Error Injection and SCSI Error Handling
**Goal**: MockTarget can inject error conditions (status codes, sense data, reject PDUs, DataSN gaps) and initiator handles each correctly
**Depends on**: Phase 13 (PDU capture framework)
**Requirements**: ERR-01, ERR-02, ERR-03, ERR-04, ERR-05, ERR-06, SNACK-01, SNACK-02
**Success Criteria** (what must be TRUE):
  1. Tests verify initiator correctly parses CRC error sense data and SNACK reject followed by successful command retry
  2. Tests verify initiator handles unexpected unsolicited data and not-enough-unsolicited-data error responses gracefully
  3. Tests verify initiator surfaces BUSY (0x08) and RESERVATION CONFLICT (0x18) status codes to the caller without retry
  4. Tests verify initiator constructs Data/R2T SNACK on DataSN gap and DataACK SNACK in response to A-bit, both with correct PDU fields on the wire
**Plans**: 2 plans
Plans:
- [x] 16-01-PLAN.md — HandleSCSIWithStatus helper + error handling tests (ERR-01 through ERR-06)
- [x] 16-02-PLAN.md — SNACK wire conformance tests (SNACK-01, SNACK-02)

### Phase 17: Session Management, NOP-Out, and Async Messages
**Goal**: MockTarget can inject async messages and the initiator responds correctly to all session management scenarios
**Depends on**: Phase 13 (PDU capture framework)
**Requirements**: SESS-01, SESS-02, SESS-03, SESS-04, SESS-05, SESS-06, ASYNC-01, ASYNC-02, ASYNC-03, ASYNC-04
**Success Criteria** (what must be TRUE):
  1. Tests verify initiator performs clean Logout after receiving AsyncMessage code 1 on both single-connection and multi-connection sessions
  2. Tests verify NOP-Out ping response echoes TTT with correct ITT, I-bit, and LUN fields; NOP-Out ping request uses valid ITT; and ExpStatSN confirmation variant is correct on the wire
  3. Tests verify clean logout exchange with correct PDU field values
  4. Tests verify initiator handles async message codes for connection drop, session drop, and negotiation request by taking the appropriate action (close connection, close session, re-negotiate)
**Plans**: 4 plans
Plans:
- [x] 17-01-PLAN.md — MockTarget SendAsyncMsg/HandleText + production code fixes + NOP-Out ping tests (SESS-03, SESS-04)
- [x] 17-02-PLAN.md — ExpStatSN confirmation test (SESS-05) + session lifecycle tests (SESS-01, SESS-06)
- [x] 17-03-PLAN.md — Async message conformance tests (ASYNC-01, ASYNC-02, ASYNC-03, ASYNC-04)

### Phase 18: Command Window, Retry, and ERL 2
**Goal**: Initiator correctly enforces command window boundaries, retries commands with original fields, recovers from ExpStatSN gaps, and performs ERL 2 connection reassignment with task reassign
**Depends on**: Phase 13 (PDU capture framework), Phase 16 (error injection for reject/retry scenarios)
**Requirements**: CMDSEQ-04, CMDSEQ-05, CMDSEQ-06, CMDSEQ-07, CMDSEQ-08, CMDSEQ-09, SESS-07, SESS-08
**Success Criteria** (what must be TRUE):
  1. Tests verify initiator blocks new commands when MaxCmdSN=ExpCmdSN-1 (zero window) and resumes when window opens, correctly uses large windows, and respects window size of 1
  2. Tests verify command retry carries original ITT, CDB, and CmdSN on the wire
  3. Tests verify ExpStatSN gap detection triggers recovery and MaxCmdSN in SCSI Response correctly closes the command window
  4. Tests verify ERL 2 connection reassignment after drop and task reassign on the new connection with correct PDU fields
**Plans**: 4 plans
Plans:
- [x] 18-01-PLAN.md — Command window conformance tests (CMDSEQ-04, CMDSEQ-05, CMDSEQ-06, CMDSEQ-09)
- [x] 18-02-PLAN.md — Command retry and ExpStatSN gap tests (CMDSEQ-07, CMDSEQ-08)
- [x] 18-03-PLAN.md — ERL 2 dispatch fix + connection reassignment and task reassign tests (SESS-07, SESS-08)
- [x] 18-04-PLAN.md — Gap closure: same-connection retry with original ITT/CDB/CmdSN (CMDSEQ-07)

### Phase 19: Task Management and Text Negotiation
**Goal**: TMF PDU fields are verified at wire level and Text Request negotiation covers all advanced features
**Depends on**: Phase 13 (PDU capture framework)
**Requirements**: TMF-01, TMF-02, TMF-03, TMF-04, TMF-05, TMF-06, TEXT-01, TEXT-02, TEXT-03, TEXT-04, TEXT-05, TEXT-06
**Success Criteria** (what must be TRUE):
  1. Tests verify TMF PDU carries correct CmdSN, LUN encoding, and RefCmdSN for the referenced task on the wire
  2. Tests verify Abort Task Set aborts all tasks on LUN, blocks new tasks during abort, and response arrives after tasks are cleared
  3. Tests verify Text Request fields, ITT uniqueness across requests, initial TTT=0xFFFFFFFF, TTT continuation echo, other parameters, and negotiation reset behavior
**Plans**: 2 plans
Plans:
- [x] 19-01-PLAN.md — TMF wire conformance tests (TMF-01, TMF-02, TMF-03, TMF-04, TMF-05, TMF-06)
- [x] 19-02-PLAN.md — Text Request wire conformance tests (TEXT-01, TEXT-02, TEXT-03, TEXT-04, TEXT-05, TEXT-06)

## Progress

**Execution Order:**
Phases execute in numeric order: 13 -> 14 -> 15 -> 16 -> 17 -> 18 -> 19
(Phases 14, 15, 16, 17 can execute in any order after 13; Phase 18 depends on 13+16; Phase 19 depends on 13)

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 1. PDU Codec and Transport | v1.0 | 3/3 | Complete | 2026-03-31 |
| 2. Connection and Login | v1.0 | 3/3 | Complete | 2026-03-31 |
| 3. Session, Read Path, Discovery | v1.0 | 3/3 | Complete | 2026-04-01 |
| 4. Write Path | v1.0 | 4/4 | Complete | 2026-04-01 |
| 5. SCSI Command Layer | v1.0 | 3/3 | Complete | 2026-04-01 |
| 6. Error Recovery and TMF | v1.0 | 3/3 | Complete | 2026-04-01 |
| 6.1. Observability | v1.0 | 3/3 | Complete | 2026-04-02 |
| 7. Public API and Release | v1.0 | 3/3 | Complete | 2026-04-02 |
| 8. uiscsi-ls CLI | v1.0 | 2/2 | Complete | 2026-04-02 |
| 9. LIO E2E Tests | v1.0 | 2/2 | Complete | 2026-04-02 |
| 10. E2E Coverage Expansion | v1.0 | 5/5 | Complete | 2026-04-03 |
| 11. Audit Remediation | v1.0 | 4/4 | Complete | 2026-04-03 |
| 13. PDU Wire Capture + CmdSN | v1.1.0 | 2/2 | Complete    | 2026-04-04 |
| 14. Data Transfer + R2T | v1.1.0 | 4/4 | Complete    | 2026-04-05 |
| 15. SCSI Write Mode | v1.1.0 | 2/2 | Complete    | 2026-04-05 |
| 16. Error Injection + SNACK | v1.1.0 | 2/2 | Complete   | 2026-04-05 |
| 17. Session Mgmt + Async | v1.1.0 | 3/3 | Complete   | 2026-04-05 |
| 18. Cmd Window + ERL 2 | v1.1.0 | 4/4 | Complete    | 2026-04-05 |
| 19. TMF + Text | v1.1.0 | 2/2 | Complete    | 2026-04-05 |
