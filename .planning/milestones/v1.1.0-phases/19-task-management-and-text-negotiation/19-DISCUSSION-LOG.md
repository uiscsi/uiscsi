# Phase 19: Task Management and Text Negotiation - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-05
**Phase:** 19-task-management-and-text-negotiation
**Areas discussed:** TMF wire test mechanics, Text Request negotiation, Test file organization, Abort Task Set concurrency

---

## TMF Wire Test Mechanics

| Option | Description | Selected |
|--------|-------------|----------|
| Stall in HandleSCSIFunc | HandleSCSIFunc on callCount==0 blocks so SCSI command stays in-flight. Proven pattern from Phase 18. | ✓ |
| Fast command + capture ITT | Send normal command, capture ITT before response. Tight timing — may be racy. | |
| You decide | Claude picks approach. | |

**User's choice:** Stall in HandleSCSIFunc
**Notes:** Recommended approach, proven in Phase 18 ERL tests.

---

| Option | Description | Selected |
|--------|-------------|----------|
| Single-level LUN only | LUN 0 and LUN 1 with standard 8-byte encoding. What LIO uses. | |
| Multiple LUN formats | Flat space, peripheral device, and extended LUN formats per SAM-5. | ✓ |
| You decide | Claude picks based on production code. | |

**User's choice:** Multiple LUN formats
**Notes:** More thorough coverage of LUN encoding.

---

| Option | Description | Selected |
|--------|-------------|----------|
| AbortTask only | Only TMF that references specific task by ITT+RefCmdSN per RFC 7143. | ✓ |
| AbortTask + TaskReassign | Also verify TASK REASSIGN RefCmdSN (partially covered in Phase 18). | |
| You decide | Claude picks based on RFC 7143. | |

**User's choice:** AbortTask only
**Notes:** Other TMFs don't carry RefCmdSN.

---

## Text Request Negotiation

| Option | Description | Selected |
|--------|-------------|----------|
| Sequential Renegotiate calls | Call Renegotiate() multiple times, capture Text Requests, verify unique ITTs. | ✓ |
| Parallel Renegotiate calls | Concurrent goroutines. Text negotiation is serial per RFC 7143. | |
| You decide | Claude picks. | |

**User's choice:** Sequential Renegotiate calls

---

| Option | Description | Selected |
|--------|-------------|----------|
| HandleText returns partial + TTT | Continue=true, non-0xFFFFFFFF TTT. Initiator echoes. Reuses Phase 17 pattern. | ✓ |
| Large key-value payload | Exceed MaxRecvDataSegmentLength for natural continuation. Harder to control. | |
| You decide | Claude picks. | |

**User's choice:** HandleText returns partial + TTT

---

| Option | Description | Selected |
|--------|-------------|----------|
| Fresh exchange after completion | After complete exchange (TTT=0xFFFFFFFF), verify new request uses new ITT and TTT=0xFFFFFFFF. | ✓ |
| Abort mid-continuation | Start continuation, send new request with TTT=0xFFFFFFFF to reset. | |
| You decide | Claude picks. | |

**User's choice:** Fresh exchange after completion

---

## Test File Organization

| Option | Description | Selected |
|--------|-------------|----------|
| Two files by domain | tmf_test.go (TMF-01-06) + text_test.go (TEXT-01-06). One-file-per-domain pattern. | ✓ |
| Three files by complexity | Separate aborttaskset_test.go for complex concurrency tests. | |
| You decide | Claude picks. | |

**User's choice:** Two files by domain

---

## Abort Task Set Concurrency

| Option | Description | Selected |
|--------|-------------|----------|
| Goroutine + timer pattern | Same as Phase 18 zero-window. Proven blocking proof. | ✓ |
| Error-based proof | Expect error from ReadBlocks during AbortTaskSet. Simpler but no timing proof. | |
| You decide | Claude picks. | |

**User's choice:** Goroutine + timer pattern

---

| Option | Description | Selected |
|--------|-------------|----------|
| Immediate TMF Response | HandleTMF sends response immediately. Test verifies tasks canceled by response time. | ✓ |
| Delayed TMF Response | HandleTMF waits for handlers to return. Tests target, not initiator. | |
| You decide | Claude picks. | |

**User's choice:** Immediate TMF Response

---

## Claude's Discretion

- Timer durations for blocking proof tests
- HandleTMF response codes and field values
- Text Request key-value content

## Deferred Ideas

None
