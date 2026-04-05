# Phase 14: Data Transfer and R2T Wire Validation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-05
**Phase:** 14-data-transfer-and-r2t-wire-validation
**Areas discussed:** MockTarget Data-In/R2T extensions, Test organization, Negotiation parameter control, A-bit/SNACK DataACK scope

---

## MockTarget Data-In/R2T Extensions

| Option | Description | Selected |
|--------|-------------|----------|
| HandleSCSIFunc only | All tests build PDUs manually via tc.SendPDU() | |
| Dedicated helpers only | Declarative API for common patterns | |
| Hybrid | Helpers for common cases, HandleSCSIFunc for fault injection | ✓ |

**User's choice:** Hybrid — dedicated helpers for standard multi-PDU Data-In and R2T sequences, HandleSCSIFunc for fault injection edge cases.
**Notes:** Extends the existing HandleSCSIRead pattern. Implements deferred Phase 13 D-06 item 2.

---

## Test Organization for 18 Requirements

| Option | Description | Selected |
|--------|-------------|----------|
| Two files | data_transfer_test.go + r2t_test.go | |
| Three files | dataout_test.go + datain_test.go + r2t_test.go | ✓ |
| Single file | Everything in data_transfer_test.go | |
| You decide | Claude's discretion | |

**User's choice:** Three files mirroring the internal/session dataout.go / datain.go split.
**Notes:** None.

---

## Negotiation Parameter Control

| Option | Description | Selected |
|--------|-------------|----------|
| MockTarget-side config | Target offers specific values, initiator negotiates normally | ✓ |
| Initiator-side options | Initiator proposes, target accepts | |
| Both sides configured | Both sides set explicitly | |
| You decide | Claude's discretion | |

**User's choice:** MockTarget-side config — test controls target's offers.
**Notes:** Cleanest approach. Initiator follows production code path.

---

## A-bit/SNACK DataACK Scope (DATA-07)

| Option | Description | Selected |
|--------|-------------|----------|
| Keep in Phase 14 | DATA-07 in datain_test.go, build SNACK support here | ✓ |
| Defer to Phase 16 | Move to Phase 16 with SNACK-01 and SNACK-02 | |
| You decide | Claude picks during planning | |

**User's choice:** Keep in Phase 14 alongside other Data-In tests.
**Notes:** SNACK DataACK MockTarget support built in this phase.

---

## Claude's Discretion

- Exact API shape of multi-PDU helpers
- Method signatures for negotiation parameter config
- Test function vs subtest breakdown within the three files
- Plan count and task split

## Deferred Ideas

None — discussion stayed within phase scope.
