# Phase 15: SCSI Command Write Mode Wire Tests - Context

**Gathered:** 2026-04-05
**Status:** Ready for planning

<domain>
## Phase Boundary

Verify SCSI Command PDU fields (W-bit, F-bit, EDTL, DataSegmentLength) at the wire level across all ImmediateData/InitialR2T/FirstBurstLength combinations. Covers 7 requirements: SCSI-01 through SCSI-07. Tests focus specifically on the SCSI Command PDU itself, not on Data-Out PDU fields (which were covered in Phase 14).

</domain>

<decisions>
## Implementation Decisions

### Test File Organization
- **D-01:** Single file: `test/conformance/scsicommand_test.go` for all 7 requirements. Only 7 reqs all testing the same PDU type — one file keeps related assertions together.

### Test Matrix Structure
- **D-02:** Table-driven subtests. One parent test function with subtests for each cell of the 2x2 ImmediateData x InitialR2T matrix. Compact, easy to extend, consistent with Go `testing` idioms. FirstBurstLength edge cases (SCSI-05/06/07) are additional subtests within the same table-driven structure.

### Shared Write-Test Helpers
- **D-03:** Extract shared helpers into `test/conformance/helpers_test.go`. Pull common write-test setup (bilateral negotiation config via SetNegotiationConfig + WithOperationalOverrides, HandleSCSIFunc write handler patterns) into shared helpers. Both Phase 14's `dataout_test.go` and Phase 15's `scsicommand_test.go` use them. This removes duplication in negotiation boilerplate.

### FirstBurstLength Edge Cases
- **D-04:** Five boundary scenarios for SCSI-05/06/07:
  1. EDTL < FirstBurstLength (all fits in unsolicited data)
  2. EDTL = FirstBurstLength (exact boundary — tests F-bit on last unsolicited PDU)
  3. EDTL > FirstBurstLength (unsolicited limited, R2T needed for remainder)
  4. EDTL = MaxRecvDSL (single PDU — simplest case)
  5. EDTL = 2 * FirstBurstLength (exactly two bursts — unsolicited + one R2T)

### Claude's Discretion
- Exact shared helper function signatures and what to extract vs inline
- How SCSI-01 through SCSI-07 map to specific table-driven subtest cases
- Plan count and task breakdown
- Whether helpers_test.go also benefits existing Phase 13 cmdseq tests

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Test Infrastructure (from Phases 13-14)
- `test/pducapture/capture.go` — PDU capture framework with Recorder, Filter, Sent, Received helpers
- `test/target.go` — MockTarget with HandleSCSIFunc, SessionState, SetNegotiationConfig, NegotiationConfig, ReadPDU, ReadDataOutPDUs, SendR2TSequence
- `options.go:100` — WithPDUHook public API for initiator-side capture
- `options.go` — WithOperationalOverrides for initiator-side negotiation parameter proposals

### Existing Write Path Tests (Phase 14 — patterns to follow and helpers to extract from)
- `test/conformance/dataout_test.go` — Data-Out wire conformance tests using bilateral negotiation + pducapture + HandleSCSIFunc
- `test/conformance/r2t_test.go` — R2T fulfillment tests

### Implementation Under Test
- `internal/session/dataout.go` — task.sendDataOutBurst, task.handleR2T, task.sendUnsolicitedDataOut
- `internal/pdu/initiator.go` — SCSICommand PDU struct (W-bit, F-bit, EDTL, DataSegmentLength fields)

### Protocol References
- `doc/initiator_ffp.pdf` — UNH-IOL FFP test suite (SCSI Command tests: FFP #5.1-#5.7)
- `doc/test_matrix_initiator_ffp.md` — Gap analysis mapping FFP tests to current coverage

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `pducapture.Recorder`: Captures all PDUs via WithPDUHook. Use `rec.Sent(pdu.OpSCSICommand)` to capture the SCSI Command PDU sent by the initiator.
- `SetNegotiationConfig` + `WithOperationalOverrides`: Bilateral negotiation control for ImmediateData/InitialR2T/FirstBurstLength — built in Phase 14.
- `HandleSCSIFunc`: Per-test SCSI handler for target-side write handling (R2T + Data-Out consume).
- `ReadDataOutPDUs`: Inline Data-Out collection within HandleSCSIFunc — target reads until Final bit.

### Established Patterns
- Wire conformance tests: setup MockTarget → configure negotiation → run initiator write → capture PDUs → assert SCSI Command PDU field values
- Bilateral negotiation: both target (SetNegotiationConfig) and initiator (WithOperationalOverrides) must agree for boolean params (AND-semantics per RFC 7143 Section 13.3)

### Integration Points
- New test file: `test/conformance/scsicommand_test.go`
- New helper file: `test/conformance/helpers_test.go` (shared write-test setup extracted from dataout_test.go patterns)

</code_context>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches within the decisions above.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope.

</deferred>

---

*Phase: 15-scsi-command-write-mode-wire-tests*
*Context gathered: 2026-04-05*
