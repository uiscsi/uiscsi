# Phase 15: SCSI Command Write Mode Wire Tests - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-05
**Phase:** 15-scsi-command-write-mode-wire-tests
**Areas discussed:** Test file organization, Test matrix structure, Overlap with Phase 14, FirstBurstLength edge cases

---

## Test File Organization

| Option | Description | Selected |
|--------|-------------|----------|
| Single file | All 7 reqs in scsicommand_test.go | ✓ |
| Two files by mode | Split by ImmediateData Yes/No | |
| You decide | Claude's discretion | |

**User's choice:** Single file — only 7 reqs, all testing the same PDU type.
**Notes:** None.

---

## Test Matrix Structure

| Option | Description | Selected |
|--------|-------------|----------|
| Table-driven subtests | Parent test with subtests for each matrix cell | ✓ |
| Separate test functions | Individual test functions per combination | |
| Hybrid | Table for 2x2, separate for FBL edge cases | |
| You decide | Claude's discretion | |

**User's choice:** Table-driven subtests for the full matrix including FirstBurstLength cases.
**Notes:** Consistent with Go testing idioms.

---

## Overlap with Phase 14 Data-Out Tests

| Option | Description | Selected |
|--------|-------------|----------|
| Independent tests, shared infra | Phase 15 writes own assertions, uses same MockTarget/pducapture | |
| Extract shared helpers | Pull common setup into helpers_test.go | ✓ |
| You decide | Claude's discretion | |

**User's choice:** Extract shared helpers into helpers_test.go for bilateral negotiation setup and write handler patterns.
**Notes:** Reduces duplication across Phase 14 and 15 conformance tests.

---

## FirstBurstLength Edge Cases

| Option | Description | Selected |
|--------|-------------|----------|
| Three scenarios | EDTL < FBL, EDTL = FBL, EDTL > FBL | |
| Five scenarios | Add EDTL = MaxRecvDSL and EDTL = 2*FBL | ✓ |
| You decide | Claude's discretion | |

**User's choice:** Five boundary scenarios for fuller coverage.
**Notes:** None.

---

## Claude's Discretion

- Exact shared helper function signatures
- How SCSI-01 through SCSI-07 map to subtests
- Plan count and task breakdown
- Whether helpers benefit Phase 13 tests too

## Deferred Ideas

None — discussion stayed within phase scope.
