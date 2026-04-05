# Phase 13: PDU Wire Capture Framework, MockTarget Extensions, and Command Sequencing - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-04
**Phase:** 13-pdu-wire-capture-framework-mocktarget-extensions-and-command-sequencing
**Areas discussed:** PDU capture placement, MockTarget handler model, Test file organization, Wire validation approach

---

## PDU Capture Placement

| Option | Description | Selected |
|--------|-------------|----------|
| Initiator-side only | Use WithPDUHook on the initiator. Works with both MockTarget and LIO. | ✓ |
| Both sides | Capture on initiator AND MockTarget. More data but more complexity. | |
| You decide | Claude picks the approach | |

**User's choice:** Initiator-side only (Recommended)
**Notes:** None

| Option | Description | Selected |
|--------|-------------|----------|
| Decode to typed PDUs | Capture helper decodes bytes to pdu.PDU structs. Cleaner test code. | ✓ |
| Raw byte assertions | Work on raw BHS bytes with offset helpers. Lower-level. | |
| You decide | Claude picks based on maintainability | |

**User's choice:** Decode to typed PDUs
**Notes:** None

---

## MockTarget Handler Model

| Option | Description | Selected |
|--------|-------------|----------|
| HandleSCSIFunc | Single handler on OpSCSICommand with user-provided routing logic. Maximum flexibility. | ✓ |
| CDB-aware dispatch | MockTarget inspects CDB byte 0 and routes to separate handlers. Simpler but less flexible. | |
| Per-command sequence | Handlers fire in registration order. Good for scripted scenarios. | |

**User's choice:** HandleSCSIFunc (Recommended)
**Notes:** None

| Option | Description | Selected |
|--------|-------------|----------|
| MockTarget tracks state | Auto-increments StatSN, tracks ExpCmdSN, sets MaxCmdSN. Correct by default. | ✓ |
| Manual per-handler | Handlers set all fields themselves. Full control but more boilerplate. | |
| You decide | Claude picks | |

**User's choice:** MockTarget tracks state
**Notes:** None

---

## Test File Organization

| Option | Description | Selected |
|--------|-------------|----------|
| Extend test/conformance/ | Add new files in existing package. Reuses existing pattern. | |
| New test/ffp/ package | Separate package for FFP tests. Clean separation. | |
| You decide | Claude picks based on codebase conventions | ✓ |

**User's choice:** You decide
**Notes:** None

| Option | Description | Selected |
|--------|-------------|----------|
| test/pducapture/ package | Dedicated package importable by both conformance and E2E tests. | ✓ |
| In test/ alongside target.go | Same package as MockTarget. Less package proliferation. | |
| You decide | Claude picks | |

**User's choice:** test/pducapture/ package
**Notes:** None

---

## Wire Validation Approach

| Option | Description | Selected |
|--------|-------------|----------|
| Field-strict where possible | Check exact field values when FFP test specifies them. Behavioral only when fields aren't meaningful. | ✓ |
| Behavioral focus | Focus on behavioral assertions. Don't micro-validate individual PDU fields. | |
| You decide | Claude picks per test | |

**User's choice:** Field-strict where possible
**Notes:** None

| Option | Description | Selected |
|--------|-------------|----------|
| t.Error (collect all) | Report all field violations, don't stop at first. Better for debugging. | ✓ |
| t.Fatal (fail fast) | Stop at first violation. If CmdSN wrong, subsequent checks meaningless. | |
| You decide | Claude picks per-test | |

**User's choice:** t.Error (collect all)
**Notes:** None

---

## Claude's Discretion

- FFP test file organization (extend test/conformance/ or create test/ffp/)
- Exact API shape of capture/assertion helpers
- Whether to build all five MockTarget extensions in Phase 13 or defer some

## Deferred Ideas

None
