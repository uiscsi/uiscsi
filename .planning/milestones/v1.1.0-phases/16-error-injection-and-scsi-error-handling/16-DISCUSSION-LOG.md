# Phase 16: Error Injection and SCSI Error Handling - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-05
**Phase:** 16-error-injection-and-scsi-error-handling
**Areas discussed:** MockTarget fault injection API, Test file organization, SNACK-02 overlap, Error surfacing verification

---

## MockTarget Fault Injection API

| Option | Description | Selected |
|--------|-------------|----------|
| HandleSCSIFunc only | All error injection inline via callbacks | |
| Dedicated methods | InjectDataSNGap, InjectReject, etc. | |
| Hybrid | HandleSCSIFunc for complex + simple helpers for status codes | ✓ |
| You decide | Claude's discretion | |

**User's choice:** Hybrid — HandleSCSIFunc for complex error scenarios, plus HandleSCSIWithStatus helper for simple status code tests.

---

## Test File Organization

| Option | Description | Selected |
|--------|-------------|----------|
| Two files | error_test.go (ERR-01-06) + snack_test.go (SNACK-01-02) | ✓ |
| Single file | All 8 in error_test.go | |
| Three files | status + reject + snack split | |
| You decide | Claude's discretion | |

**User's choice:** Two files split by protocol mechanism.

---

## SNACK-02 Overlap with Phase 14 DATA-07

| Option | Description | Selected |
|--------|-------------|----------|
| Extend, don't duplicate | Phase 16 adds wire field depth to Phase 14's trigger test | ✓ |
| Skip SNACK-02 | Mark as satisfied by DATA-07 | |
| Full standalone | Independent test even with overlap | |

**User's choice:** Extend — Phase 16 SNACK-02 focuses on BegRun/RunLength/Type wire assertions.

---

## Error Surfacing Verification

| Option | Description | Selected |
|--------|-------------|----------|
| Check error type | errors.As() with Status field check | ✓ |
| Check error message | String matching on error.Error() | |
| Both type and message | Belt and suspenders | |
| You decide | Claude's discretion | |

**User's choice:** Go idiomatic errors.As() pattern.

---

## Claude's Discretion

- HandleSCSIWithStatus signature and location
- Test function vs subtest mapping for ERR requirements
- Error type compatibility with errors.As
- Plan count and task breakdown

## Deferred Ideas

None — discussion stayed within phase scope.
