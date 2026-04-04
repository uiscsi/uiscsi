# Phase 14: Data Transfer and R2T Wire Validation - Context

**Gathered:** 2026-04-05
**Status:** Ready for planning

<domain>
## Phase Boundary

Verify all Data-Out and Data-In PDU fields at the wire level — DataSN, F-bit, Buffer Offset, TTT echo, burst lengths, and R2T fulfillment ordering. Covers 18 requirements: DATA-01 through DATA-14 and R2T-01 through R2T-04. This is the largest single phase in v1.1.

</domain>

<decisions>
## Implementation Decisions

### MockTarget Data-In/R2T Extensions
- **D-01:** Hybrid approach — dedicated helpers for common multi-PDU Data-In responses and R2T sequences (e.g., `HandleSCSIReadMultiPDU`, R2T sequence generation), plus `HandleSCSIFunc` for fault injection and edge cases. Helpers handle correct DataSN/offset/F-bit construction; `HandleSCSIFunc` gives full manual control when tests need wrong values.
- **D-02:** This implements the deferred Phase 13 D-06 item 2 (multi-PDU Data-In with configurable DataSN gaps).

### Test Organization
- **D-03:** Three test files in `test/conformance/`: `dataout_test.go` (DATA-01 through DATA-05, DATA-08, DATA-10, DATA-11, DATA-12, DATA-13), `datain_test.go` (DATA-06, DATA-07, DATA-09, DATA-14), `r2t_test.go` (R2T-01 through R2T-04). Mirrors the `internal/session/dataout.go` / `datain.go` split.

### Negotiation Parameter Control
- **D-04:** MockTarget-side configuration — add a method to control what parameter values the target offers during login negotiation (ImmediateData, InitialR2T, FirstBurstLength, MaxBurstLength, MaxRecvDataSegmentLength). The initiator negotiates normally against the target's offers. Tests control outcomes by controlling the target side.

### A-bit/SNACK DataACK Scope
- **D-05:** DATA-07 (A-bit SNACK DataACK at ERL>=1) stays in Phase 14 in `datain_test.go`. Whatever MockTarget support is needed for A-bit injection and SNACK DataACK verification is built in this phase, not deferred to Phase 16.

### Claude's Discretion
- Exact API shape of multi-PDU Data-In and R2T helpers
- Exact method name/signature for MockTarget negotiation parameter config
- How DATA requirements map to individual test functions vs subtests within the three files
- Plan count and task breakdown across the 18 requirements

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Test Infrastructure (from Phase 13)
- `test/pducapture/capture.go` — PDU capture framework with Recorder, Filter, Sent, Received helpers
- `test/target.go` — MockTarget with HandleSCSIFunc, SessionState, SetMaxCmdSNDelta, TargetConn.SendPDU
- `options.go:100` — WithPDUHook public API for initiator-side capture

### Existing Data Transfer Implementation
- `internal/session/dataout.go` — task.sendDataOutBurst, task.handleR2T, task.sendUnsolicitedDataOut
- `internal/session/datain.go` — Data-In processing path
- `internal/session/snack.go` — SNACK/DataACK implementation (needed for DATA-07)

### Existing Tests (patterns to follow)
- `test/conformance/cmdseq_test.go` — Phase 13 wire conformance tests using pducapture + HandleSCSIFunc
- `internal/session/dataout_test.go` — Unit tests for R2T handling, multi-R2T sequences, unsolicited data
- `internal/session/datain_test.go` — Unit tests for Data-In processing

### Protocol References
- `doc/initiator_ffp.pdf` — UNH-IOL FFP test suite (DATA and R2T test specifications: FFP #6.1, #8.1-#8.3, #9.1, #10.1-#10.2, #11.1.1-#11.4, #12.1-#12.4, #16.6)
- `doc/test_matrix_initiator_ffp.md` — Gap analysis mapping FFP tests to current coverage

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `pducapture.Recorder`: Captures all PDUs via WithPDUHook, Filter by opcode+direction. Ready to use for Data-Out/Data-In/R2T assertions.
- `HandleSCSIFunc`: Flexible per-test SCSI handler with call count. Use for fault injection tests (wrong DataSN, bad TTT).
- `SessionState`: Tracks CmdSN/ExpStatSN/MaxCmdSN. Extend for negotiation parameter control.
- `TargetConn.SendPDU/SendRaw`: Manual PDU injection from target side. Use for multi-PDU Data-In and R2T generation in HandleSCSIFunc.
- `HandleSCSIRead(lun, data)`: Single-PDU read handler. Extend pattern for multi-PDU variant.

### Established Patterns
- Wire conformance tests: setup MockTarget → run initiator operation → capture PDUs → assert field values with t.Error (not t.Fatal)
- Handler registration: `mt.HandleSCSIFunc(func(tc, cmd, count) error)` for per-test behavior
- PDU capture: `rec := &pducapture.Recorder{}; opts = append(opts, uiscsi.WithPDUHook(rec.Hook()))`

### Integration Points
- New helpers added to `test/target.go` (MockTarget extensions)
- Three new test files in `test/conformance/`
- May need to extend `test/pducapture/capture.go` with Data-Out/R2T-specific filter helpers

</code_context>

<specifics>
## Specific Ideas

No specific requirements — open to standard approaches within the decisions above.

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 14-data-transfer-and-r2t-wire-validation*
*Context gathered: 2026-04-05*
