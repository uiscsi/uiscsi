# Phase 17: Session Management, NOP-Out, and Async Messages - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-05
**Phase:** 17-session-management-nop-out-and-async-messages
**Areas discussed:** Async message injection API, NOP-Out wire test approach, Async handler verification, Test file organization

---

## Async Message Injection API

| Option | Description | Selected |
|--------|-------------|----------|
| Generic SendAsyncMsg (Recommended) | Single method: SendAsyncMsg(tc, event, params). Minimal API, tests build specifics inline. | ✓ |
| Per-event helpers | Dedicated methods per event code: SendAsyncLogoutRequest, SendAsyncConnDrop, etc. More readable but 4+ methods. | |
| You decide | Claude picks based on MockTarget patterns. | |

**User's choice:** Generic SendAsyncMsg
**Notes:** Consistent with HandleSCSIFunc pattern.

---

| Option | Description | Selected |
|--------|-------------|----------|
| Skip SESS-02 (Recommended) | Multi-connection not implemented. Test single-connection variant thoroughly. | ✓ |
| Minimal multi-conn mock | Simulate with two TCP connections to same session. Complex, may not reflect real semantics. | |
| You decide | Claude assesses feasibility. | |

**User's choice:** Skip SESS-02
**Notes:** None.

---

## NOP-Out Wire Test Approach

| Option | Description | Selected |
|--------|-------------|----------|
| Implement + test (Recommended) | Add sendExpStatSNConfirmation() and test it. Small addition, only new production code in phase. | ✓ |
| Test existing only, defer SESS-05 | Only test NOP-Out variants that exist. Defer SESS-05. | |
| You decide | Claude assesses triviality. | |

**User's choice:** Implement + test
**Notes:** None.

---

| Option | Description | Selected |
|--------|-------------|----------|
| Full field validation (Recommended) | Assert all fields: ITT, TTT echo, I-bit, F-bit, CmdSN, ExpStatSN, LUN echo, ping data echo. | ✓ |
| Key fields only | Just TTT echo and ITT=0xFFFFFFFF. | |
| You decide | Claude decides based on conformance expectations. | |

**User's choice:** Full field validation
**Notes:** None.

---

## Async Handler Verification

| Option | Description | Selected |
|--------|-------------|----------|
| PDU capture + side effects (Recommended) | Recorder captures PDUs + verify behavioral side effects (session close, errors, reconnect). | ✓ |
| Callback-based verification | WithAsyncHandler callback fires with correct event code. Doesn't verify autonomous reaction. | |
| You decide | Claude picks best coverage approach. | |

**User's choice:** PDU capture + side effects
**Notes:** None.

---

| Option | Description | Selected |
|--------|-------------|----------|
| Verify event dispatch only (Recommended) | Test callback invocation. Renegotiation not implemented. | |
| Full renegotiation | Implement renegotiation logic and test E2E. Significant new production code. | ✓ |

**User's choice:** Full renegotiation
**Notes:** User confirmed after scope check — accepts the scope expansion. Text Request exchange after AsyncEvent 4.

---

## Test File Organization

| Option | Description | Selected |
|--------|-------------|----------|
| Three files (Recommended) | session_test.go, nopout_test.go, async_test.go. Clean split by protocol mechanism. | ✓ |
| Two files | session_test.go (all SESS), async_test.go (all ASYNC). NOP-Out grouped with session. | |
| You decide | Claude picks for navigability. | |

**User's choice:** Three files
**Notes:** Consistent with existing error_test.go/snack_test.go pattern.

---

## Claude's Discretion

- AsyncParams struct fields and signature
- sendExpStatSNConfirmation visibility (public vs internal)
- Renegotiation Text Request parameter selection
- ASYNC-01 vs SESS-01 deduplication
- Plan count and task breakdown

## Deferred Ideas

- SESS-02: Multi-connection session testing — not implemented
