# Phase 16: Error Injection and SCSI Error Handling - Context

**Gathered:** 2026-04-05
**Status:** Ready for planning

<domain>
## Phase Boundary

MockTarget error injection infrastructure plus initiator error handling verification. Covers 8 requirements: ERR-01 through ERR-06 (SCSI status codes, sense data, reject PDUs, unsolicited data errors) and SNACK-01, SNACK-02 (DataSN gap SNACK, DataACK SNACK wire fields). This phase makes MockTarget capable of misbehaving — sending wrong PDUs, gaps, and error responses — and verifies the initiator handles each correctly.

</domain>

<decisions>
## Implementation Decisions

### MockTarget Fault Injection API
- **D-01:** Hybrid approach — HandleSCSIFunc for complex error scenarios (handler manually sends wrong PDUs, gaps, rejects via tc.SendPDU), plus 1-2 common helpers for simple status code tests (e.g., `HandleSCSIWithStatus(lun, status, senseData)` for ERR-03/04/05/06). Balances readability with minimal API growth. Complex scenarios like DataSN gaps and reject+retry remain inline in HandleSCSIFunc.

### Test File Organization
- **D-02:** Two files in `test/conformance/`: `error_test.go` (ERR-01 through ERR-06: CRC sense data, SNACK reject, unsolicited data errors, BUSY/RESERVATION CONFLICT status) and `snack_test.go` (SNACK-01, SNACK-02: DataSN gap SNACK construction, DataACK SNACK wire fields). Clean split by protocol mechanism.

### SNACK-02 Overlap with Phase 14 DATA-07
- **D-03:** Extend, don't duplicate. Phase 14 DATA-07 tested the A-bit trigger (target sends A-bit, initiator responds with DataACK). Phase 16 SNACK-02 focuses on the SNACK PDU wire field depth — verifying BegRun, RunLength, and Type fields on the captured SNACK PDU. References Phase 14 DATA-07 as the trigger test; Phase 16 adds field-level wire assertions.

### Error Surfacing Verification
- **D-04:** Check error type using Go idiomatic `errors.As()` pattern. Verify the returned error wraps or is a specific error type (e.g., `*SCSIError` or equivalent with a `Status` field). Tests assert `errors.As(&scsierr)` and then check `scsierr.Status == 0x08` (BUSY) or `scsierr.Status == 0x18` (RESERVATION CONFLICT). No string matching.

### Claude's Discretion
- Exact signature of HandleSCSIWithStatus helper
- How ERR-01 through ERR-06 map to individual test functions vs subtests
- Whether HandleSCSIWithStatus goes in test/target.go or helpers_test.go
- Plan count and task breakdown across the 8 requirements
- Whether existing error types in the codebase already support the errors.As pattern or need extension

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Test Infrastructure (from Phases 13-15)
- `test/pducapture/capture.go` — PDU capture framework
- `test/target.go` — MockTarget with HandleSCSIFunc, SessionState, NegotiationConfig, ReadPDU, SendR2TSequence
- `test/conformance/helpers_test.go` — Shared write-test helpers (Phase 15)
- `options.go` — WithPDUHook, WithOperationalOverrides

### Error Handling Implementation (under test)
- `internal/session/snack.go` — SNACK types (DataR2T=0, Status=1, DataACK=2), SNACK send logic
- `internal/session/datain.go` — Data-In processing, A-bit DataACK handling (Phase 14 fix)
- `internal/session/session.go` — Error surfacing to caller, task completion
- `internal/pdu/target.go` — Reject PDU struct, SCSIResponse with sense data

### Existing Error Tests (patterns to follow)
- `test/conformance/datain_test.go` — TestDataIn_ABitDataACK (Phase 14 SNACK DataACK test)
- `internal/session/snack_test.go` — Unit tests for SNACK construction

### Protocol References
- `doc/initiator_ffp.pdf` — UNH-IOL FFP test suite (Error handling tests)
- `doc/test_matrix_initiator_ffp.md` — Gap analysis

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `pducapture.Recorder`: Capture SNACK Request PDUs sent by initiator via `rec.Sent(pdu.OpSNACKRequest)`
- `HandleSCSIFunc`: Per-test SCSI handler — ideal for injecting wrong DataSN, sending Reject PDUs, returning error status codes
- `tc.SendPDU`: Target-side PDU injection — send Reject, SCSIResponse with sense data, Data-In with gaps
- Phase 14 A-bit DataACK infrastructure already working

### Integration Points
- New files: `test/conformance/error_test.go`, `test/conformance/snack_test.go`
- Possible new helper: `HandleSCSIWithStatus` in test/target.go for simple status code injection
- May need to verify/extend error type in `internal/session/` for errors.As compatibility

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

*Phase: 16-error-injection-and-scsi-error-handling*
*Context gathered: 2026-04-05*
