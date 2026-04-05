# Phase 18: Command Window, Retry, and ERL 2 - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-05
**Phase:** 18-command-window-retry-and-erl-2
**Areas discussed:** ERL 2 feasibility, Command window test mechanics, Command retry trigger strategy, Test file organization

---

## ERL 2 Feasibility

| Option | Description | Selected |
|--------|-------------|----------|
| Full E2E test (Recommended) | Test SESS-07 + SESS-08. Production code exists in connreplace.go. | ✓ |
| Skip ERL 2 tests | Defer SESS-07/SESS-08. Focus on 6 CMDSEQ only. | |
| You decide | Claude assesses feasibility. | |

**User's choice:** Full E2E test
**Notes:** connreplace.go has ~150 lines implementing full ERL 2. Test matrix "not implemented" note was stale.

---

| Option | Description | Selected |
|--------|-------------|----------|
| Accept reconnect to same listener (Recommended) | MockTarget listener accepts second connection. Minimal new code. | ✓ |
| Dedicated ERL 2 helper | Explicit ERL 2 methods on MockTarget. More API surface. | |
| You decide | Claude picks based on architecture. | |

**User's choice:** Accept reconnect to same listener
**Notes:** None.

---

## Command Window Test Mechanics

| Option | Description | Selected |
|--------|-------------|----------|
| HandleSCSIFunc with window control (Recommended) | Set MaxCmdSN in SCSI Response to close/open window. No new API. | ✓ |
| Dedicated window control API | SetCommandWindow(size) method on MockTarget. | |
| You decide | Claude picks. | |

**User's choice:** HandleSCSIFunc with window control
**Notes:** None.

---

| Option | Description | Selected |
|--------|-------------|----------|
| Goroutine + timer (Recommended) | Launch goroutine, verify it blocks, then open window via NOP-In. | ✓ |
| Channel-based signaling | Detect absence of SCSI Command PDU on wire. | |
| You decide | Claude picks most deterministic. | |

**User's choice:** Goroutine + timer
**Notes:** None.

---

## Command Retry Trigger Strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Reject + retry capture (Recommended) | ERL>=1, Reject on first call, capture both commands. Reuses ERR-02 pattern. | ✓ |
| Non-response timeout | Don't respond, wait for timeout retry. Slower. | |
| You decide | Claude picks. | |

**User's choice:** Reject + retry capture
**Notes:** None.

---

| Option | Description | Selected |
|--------|-------------|----------|
| Skip StatSN in SCSI Response (Recommended) | Jump StatSN in response, verify initiator detects gap. | ✓ |
| You decide | Claude picks based on gap detection impl. | |

**User's choice:** Skip StatSN in SCSI Response
**Notes:** None.

---

## Test File Organization

| Option | Description | Selected |
|--------|-------------|----------|
| Three files (Recommended) | cmdwindow_test.go, retry_test.go, erl2_test.go. | ✓ |
| Two files | cmdseq_test.go + erl2_test.go. | |
| You decide | Claude picks. | |

**User's choice:** Three files
**Notes:** Consistent with Phase 16-17 pattern of splitting by protocol mechanism.

---

## Claude's Discretion

- Timer durations for block verification
- Connection drop trigger for ERL 2
- CMDSEQ-09 as separate test or CMDSEQ-04 variant
- Task reassign verification method
- Plan count, waves, task breakdown

## Deferred Ideas

None.
