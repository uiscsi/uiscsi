---
phase: 13-pdu-wire-capture-framework-mocktarget-extensions-and-command-sequencing
verified: 2026-04-05T00:36:00Z
status: passed
score: 9/9 must-haves verified
re_verification: false
---

# Phase 13: PDU Wire Capture Framework, MockTarget Extensions, and Command Sequencing Verification Report

**Phase Goal:** Every subsequent phase can capture PDUs on the wire and assert field-level correctness; MockTarget supports fault injection, async messages, and command window control; validated by basic CmdSN sequencing tests
**Verified:** 2026-04-05T00:36:00Z
**Status:** passed
**Re-verification:** No — initial verification

---

## Goal Achievement

### Observable Truths

| #   | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1   | PDU capture Recorder collects every PDU sent and received via WithPDUHook | VERIFIED | `test/pducapture/capture.go` Recorder.Hook() returns `func(context.Context, uiscsi.PDUDirection, []byte)` compatible with WithPDUHook; 5 unit tests pass under -race |
| 2   | Captured PDUs are decoded into typed pdu.PDU structs, not raw bytes | VERIFIED | Hook calls `pdu.DecodeBHS(bhs)` at line 44 and stores `decoded pdu.PDU` in CapturedPDU.Decoded; test asserts `.Decoded.Opcode()` and type-asserts to `*pdu.SCSICommand` |
| 3   | Recorder.Sent(opcode) and Recorder.Received(opcode) filter by opcode and direction | VERIFIED | Methods at lines 87-93 delegate to Filter(); TestRecorder_Filter exercises both shorthands and verifies counts |
| 4   | MockTarget HandleSCSIFunc allows per-command routing by CDB opcode with call counter | VERIFIED | `func (mt *MockTarget) HandleSCSIFunc(...)` at line 179 uses `atomic.Int32` counter; TestHandleSCSIFunc_CallCount passes |
| 5   | MockTarget SessionState tracks ExpCmdSN and MaxCmdSN with configurable delta | VERIFIED | `SessionState` struct at line 68 with Update(), SetMaxCmdSNDelta(), ExpCmdSN() methods; TestSessionState_Update and TestSessionState_SetMaxCmdSNDelta pass |
| 6   | Test proves CmdSN increments by exactly 1 for each non-immediate SCSI command on the wire | VERIFIED | TestCmdSN_SequentialIncrement passes under -race; sends 5 TestUnitReady calls, asserts each delta==1 via rec.Sent(pdu.OpSCSICommand) |
| 7   | Test proves NOP-Out carries Immediate=true and does not advance CmdSN | VERIFIED | TestCmdSN_ImmediateDelivery_NonTMF passes under -race; solicited NOP-In triggers deterministic NOP-Out; Immediate flag and CmdSN delta verified on wire |
| 8   | Test proves TMF (LUN Reset) carries Immediate=true and does not advance CmdSN | VERIFIED | TestCmdSN_ImmediateDelivery_TMF passes under -race; verifies TaskMgmtReq.Header.Immediate and SCSI command delta=1 |
| 9   | All three tests pass under go test -race | VERIFIED | `go test -race ./test/conformance/ -run TestCmdSN -v` exits 0; all 25 conformance tests pass (zero regression) |

**Score:** 9/9 truths verified

---

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `test/pducapture/capture.go` | Recorder, CapturedPDU, Hook/All/Filter/Sent/Received | VERIFIED | 95 lines; all 7 exported types and methods present; no stubs |
| `test/pducapture/capture_test.go` | Unit tests for capture framework | VERIFIED | 178 lines; 5 table-driven tests covering decode, short data, invalid opcode, filter, and defensive copy |
| `test/target.go` | HandleSCSIFunc, SessionState, Session() | VERIFIED | SessionState struct at line 68, HandleSCSIFunc at line 179, Session() at line 171, `session *SessionState` field in MockTarget struct, initialized in NewMockTarget at line 148 |
| `test/conformance/cmdseq_test.go` | CMDSEQ-01, CMDSEQ-02, CMDSEQ-03 conformance tests | VERIFIED | 294 lines; TestCmdSN_SequentialIncrement, TestCmdSN_ImmediateDelivery_NonTMF, TestCmdSN_ImmediateDelivery_TMF all present and passing |

---

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `test/pducapture/capture.go` | `options.go WithPDUHook` | `Hook()` returns `func(context.Context, uiscsi.PDUDirection, []byte)` | WIRED | Line 35: return type matches WithPDUHook parameter exactly; cmdseq_test.go passes `rec.Hook()` to `uiscsi.WithPDUHook()` at lines 49, 164, 247 |
| `test/pducapture/capture.go` | `internal/pdu/header.go DecodeBHS` | Hook calls `pdu.DecodeBHS` | WIRED | Line 44: `decoded, err := pdu.DecodeBHS(bhs)` |
| `test/target.go HandleSCSIFunc` | `test/target.go SessionState` | HandleSCSIFunc receives cmd, caller calls `tgt.Session().Update()` | WIRED | HandleLogin seeds SessionState at line 311: `mt.session.Update(req.CmdSN, false)`; all three conformance tests call `tgt.Session().Update(cmd.CmdSN, cmd.Header.Immediate)` |
| `test/conformance/cmdseq_test.go` | `test/pducapture/capture.go` | `rec.Sent(pdu.Op...)` | WIRED | Lines 64, 186, 198, 271, 283 |
| `test/conformance/cmdseq_test.go` | `test/target.go HandleSCSIFunc` | `tgt.HandleSCSIFunc(...)` | WIRED | Lines 30, 126, 228 |
| `test/conformance/cmdseq_test.go` | `uiscsi.WithPDUHook` | `uiscsi.WithPDUHook(rec.Hook())` | WIRED | Lines 49, 164, 247 |

---

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| `test/pducapture/capture.go` Recorder | `r.pdus []CapturedPDU` | WithPDUHook callback receives live bytes from transport pump | Yes — bytes come from actual TCP reads in session | FLOWING |
| `test/conformance/cmdseq_test.go` | `cmds []CapturedPDU` via `rec.Sent(pdu.OpSCSICommand)` | Recorder populated during real Dial+TestUnitReady session | Yes — verified by test output showing 5 sequential window updates in slog | FLOWING |

---

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| pducapture unit tests pass under -race | `go test -race ./test/pducapture/ -v -count=1` | 5 tests PASS | PASS |
| MockTarget SessionState/HandleSCSIFunc tests pass | `go test -race ./test/ -run "TestHandleSCSIFunc\|TestSessionState" -v` | 3 tests PASS | PASS |
| CMDSEQ-01 sequential increment test passes | `go test -race ./test/conformance/ -run TestCmdSN_SequentialIncrement -v` | PASS (0.00s) | PASS |
| CMDSEQ-02 NOP-Out immediate delivery test passes | `go test -race ./test/conformance/ -run TestCmdSN_ImmediateDelivery_NonTMF -v` | PASS (0.10s) | PASS |
| CMDSEQ-03 TMF immediate delivery test passes | `go test -race ./test/conformance/ -run TestCmdSN_ImmediateDelivery_TMF -v` | PASS (0.00s) | PASS |
| Full conformance suite passes (regression check) | `go test -race ./test/conformance/ -count=1` | ok (15.188s, all 25 tests pass) | PASS |
| go vet clean on all modified packages | `go vet ./test/pducapture/ ./test/ ./test/conformance/` | No output (exit 0) | PASS |

---

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| CMDSEQ-01 | 13-01, 13-02 | E2E test validates CmdSN increments by 1 for each non-immediate command on wire (FFP #1.1) | SATISFIED | TestCmdSN_SequentialIncrement in test/conformance/cmdseq_test.go; sends 5 SCSI commands, asserts each consecutive delta==1 on wire via PDU capture |
| CMDSEQ-02 | 13-01, 13-02 | E2E test validates immediate delivery flag and CmdSN for non-TMF commands (FFP #2.1) | SATISFIED | TestCmdSN_ImmediateDelivery_NonTMF; solicited NOP-In triggers NOP-Out, asserts Immediate=true and SCSI CmdSN gap==1 despite intervening NOP-Out |
| CMDSEQ-03 | 13-01, 13-02 | E2E test validates immediate delivery CmdSN for task management commands (FFP #2.2) | SATISFIED | TestCmdSN_ImmediateDelivery_TMF; LUNReset TMF asserted Immediate=true, SCSI CmdSN delta==1 before and after |

**REQUIREMENTS.md cross-reference:** CMDSEQ-01, CMDSEQ-02, CMDSEQ-03 are mapped to Phase 13 in the requirements table (lines 121-123). All three are checked [x] as satisfied. No orphaned requirements — CMDSEQ-04 through CMDSEQ-09 are mapped to Phase 18 and not expected in this phase.

---

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| — | — | None found | — | — |

No TODO/FIXME/PLACEHOLDER markers, no stub return patterns, no hardcoded empty values in rendered output paths. All state variables in conformance tests are populated from real PDU captures.

---

### Human Verification Required

None. All must-haves are verifiable programmatically. Tests run in-process against MockTarget with no external service dependencies.

---

### Commit Verification

All four task commits documented in SUMMARY files are confirmed present in git history:

| Commit | Plan | Task |
| ------ | ---- | ---- |
| `f7814a9` | 13-01 | Create PDU capture framework |
| `911869f` | 13-01 | Extend MockTarget with HandleSCSIFunc and SessionState |
| `775f431` | 13-02 | CMDSEQ-01 sequential increment test |
| `a2a14fc` | 13-02 | CMDSEQ-02 and CMDSEQ-03 immediate delivery tests |

---

### Summary

Phase 13 goal is fully achieved. All three infrastructure components — PDU capture framework, MockTarget extensions, and CmdSN conformance tests — exist, are substantive, are wired together, and produce real data from live iSCSI sessions.

Key observations:

1. The pducapture.Recorder pattern is ready for use in future FFP phases: `rec := &pducapture.Recorder{}` then pass `uiscsi.WithPDUHook(rec.Hook())` to Dial, then assert with `rec.Sent(pdu.OpX)` returning `[]CapturedPDU` with typed `.Decoded` fields.

2. SessionState correctly models RFC 7143 Section 3.2.2.1: immediate commands do not advance ExpCmdSN. The deviation noted in 13-02-SUMMARY (default HandleNOPOut uses CmdSN+1 incorrectly for immediate NOP-Out) was fixed test-locally in cmdseq_test.go — the production HandleNOPOut is not broken for non-SessionState tests.

3. The 25-test conformance suite passes with zero regressions from HandleLogin's new SessionState seeding call.

---

_Verified: 2026-04-05T00:36:00Z_
_Verifier: Claude (gsd-verifier)_
