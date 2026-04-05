# Phase 13: PDU Wire Capture Framework, MockTarget Extensions, and Command Sequencing - Context

**Gathered:** 2026-04-04
**Status:** Ready for planning

<domain>
## Phase Boundary

Build reusable test infrastructure for wire-level PDU assertions and extend MockTarget with fault injection capabilities. Validate the infrastructure with 3 CmdSN sequencing tests (CMDSEQ-01, CMDSEQ-02, CMDSEQ-03). This phase is the foundation for all subsequent v1.1 phases.

</domain>

<decisions>
## Implementation Decisions

### PDU Capture
- **D-01:** Capture on initiator-side only, using `WithPDUHook`. This works with both MockTarget and real LIO target — single capture point for all tests.
- **D-02:** Capture helper decodes raw bytes into typed `pdu.PDU` structs. Assertions read fields directly (e.g., `capture.Filter(OpSCSICommand)` then check `.CmdSN`). No raw byte offset manipulation in tests.
- **D-03:** PDU capture helpers live in dedicated `test/pducapture/` package, importable by both conformance and E2E tests.

### MockTarget Handler Model
- **D-04:** Add `HandleSCSIFunc(func)` pattern — a single handler registered on `OpSCSICommand` that receives the decoded SCSI command and lets the test author route by CDB opcode internally. Maximum flexibility for error injection, multi-command scenarios, etc.
- **D-05:** MockTarget tracks session state (CmdSN, ExpStatSN, MaxCmdSN) internally with correct-by-default behavior. Tests configure MaxCmdSN policy (e.g., `SetMaxCmdSNDelta`). Individual handlers don't need to manage sequencing.

### MockTarget Extensions Required (for later phases)
- **D-06:** Five extension categories needed for v1.1: (1) stateful session tracking for command window control, (2) multi-PDU Data-In with configurable DataSN gaps, (3) async message injection via `TargetConn.SendPDU`, (4) per-command handler routing (HandleSCSIFunc), (5) PDU capture middleware. Build the foundation (1, 4, 5) in this phase; extensions (2, 3) can be added incrementally in their respective phases.

### Test Organization
- **D-07:** FFP conformance test file organization is Claude's discretion — either extend `test/conformance/` or create new `test/ffp/` package based on what makes most sense for maintainability.

### Wire Validation
- **D-08:** Field-strict validation where the FFP test specifies exact values (CmdSN delta, DataSN sequence, F-bit, TTT echo). Behavioral assertions only when field checks aren't meaningful for the test.
- **D-09:** Use `t.Error` (collect all violations) rather than `t.Fatal` for PDU field assertions. Report all violations in a single test run for better debugging.

### Claude's Discretion
- FFP test file organization (D-07)
- Exact API shape of the capture/assertion helpers
- Whether to build all five MockTarget extensions in Phase 13 or defer some to consuming phases

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Test Infrastructure
- `test/target.go` — Existing MockTarget with handler-based PDU dispatch, TargetConn, all Handle* methods
- `internal/transport/faultconn.go` — FaultConn for transport-level byte fault injection (complements MockTarget)
- `options.go:100` — `WithPDUHook` public API that captures BHS+DataSegment as []byte

### Existing Test Patterns
- `test/conformance/fullfeature_test.go` — `setupFullFeatureTarget` pattern with MockTarget
- `test/conformance/error_test.go` — Error injection via `HandleSCSIError`
- `test/e2e/tmf_test.go` — PDU hook usage for ITT capture during E2E tests

### Protocol References
- `doc/initiator_ffp.pdf` — UNH-IOL FFP test suite (source of all 62 tests)
- `doc/test_matrix_initiator_ffp.md` — Gap analysis mapping FFP tests to current coverage

### Research
- `.planning/research/FAULT-INJECTION.md` — Research on MockTarget extension approach (confirms no TCP proxy needed)

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `WithPDUHook` (options.go:100): Already captures every PDU as []byte. Needs assertion wrapper, not replacement.
- `MockTarget` (test/target.go): Has `Handle(opcode, handler)` dispatch, TargetConn with SendPDU/NextStatSN. Needs per-SCSI-opcode routing and stateful session tracking.
- `FaultConn` (internal/transport/faultconn.go): Transport-level fault injection. Stays for connection-drop scenarios.
- `pdu.Decode` (internal/pdu/): Decodes raw BHS bytes into typed PDU structs — use in capture helper.

### Established Patterns
- Handler registration: `mt.HandleSCSIRead(lun, data)`, `mt.HandleSCSIError(status, senseData)` — all register on OpSCSICommand, can't coexist.
- PDU hook signature: `func(context.Context, PDUDirection, []byte)` — direction + raw bytes.
- TargetConn: Has `NextStatSN()`, `StatSN()`, `SendPDU()`, `SendRaw()`, `Close()`.

### Integration Points
- New `test/pducapture/` package imported by conformance and E2E tests
- MockTarget extensions in `test/target.go` (no new file — extend existing)
- New conformance test files import both `test` and `test/pducapture`

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

*Phase: 13-pdu-wire-capture-framework-mocktarget-extensions-and-command-sequencing*
*Context gathered: 2026-04-04*
