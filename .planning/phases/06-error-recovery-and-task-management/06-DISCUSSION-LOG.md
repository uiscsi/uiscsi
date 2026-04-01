# Phase 6: Error Recovery and Task Management - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md -- this log preserves the alternatives considered.

**Date:** 2026-04-01
**Phase:** 06-error-recovery-and-task-management
**Areas discussed:** ERL 0 reconnect strategy, ERL 1/2 scope and SNACK, TMF API shape, Error injection testing

---

## ERL 0 Reconnect Strategy

### Recovery trigger

| Option | Description | Selected |
|--------|-------------|----------|
| Library auto-reconnects | Session detects connection loss, automatically attempts reconnect + session reinstatement. Configurable retry count/backoff via SessionOption. | ✓ |
| Caller-driven recovery | Session signals connection loss via error/callback. Caller decides when to reconnect by calling sess.Reinstate(). | |
| Hybrid -- hooks with default | Auto-reconnect by default, but callers can register a RecoveryHandler callback to customize behavior. | |

**User's choice:** Library auto-reconnects
**Notes:** None

### In-flight command handling

| Option | Description | Selected |
|--------|-------------|----------|
| Retry all in-flight | After session reinstatement, re-submit all commands that were in-flight. Callers see transparent recovery. | ✓ |
| Fail all in-flight | All in-flight commands get Result with Err set. Caller resubmits if desired. | |
| Configurable per-command | Command struct gets a Retriable bool. Only retriable commands are retried. | |

**User's choice:** Retry all in-flight
**Notes:** None

### Retry limit

| Option | Description | Selected |
|--------|-------------|----------|
| 3 retries with backoff | Default 3 attempts with exponential backoff (1s, 2s, 4s). Configurable. After exhaustion, all in-flight commands fail. | ✓ |
| Unlimited retries with timeout | Keep retrying until DefaultTime2Retain expires. | |
| Single retry only | One reconnect attempt. If it fails, session is dead. | |

**User's choice:** 3 retries with backoff
**Notes:** None

### ISID/TSIH handling

| Option | Description | Selected |
|--------|-------------|----------|
| Reuse ISID, present old TSIH | Per RFC 7143 Section 6.3.5. Target recognizes as session reinstatement. | ✓ |
| New ISID, new session | Start fresh. Simpler but loses target-side state. | |

**User's choice:** Reuse ISID, present old TSIH
**Notes:** None

---

## ERL 1/2 Scope and SNACK

### ERL 1/2 implementation depth

| Option | Description | Selected |
|--------|-------------|----------|
| Full RFC compliance | Implement SNACK for Data-In/Status retransmission (ERL 1) and connection replacement with task reassignment (ERL 2). Core value is RFC 7143 compliance. | ✓ |
| ERL 1 full, ERL 2 stub | Full SNACK implementation. ERL 2 gets negotiation support but no actual recovery. | |
| Both minimal/negotiation only | Negotiate ERL 1/2 but implement only ERL 0 recovery. | |

**User's choice:** Full RFC compliance
**Notes:** None

### SNACK detection approach

| Option | Description | Selected |
|--------|-------------|----------|
| DataSN gap detection only | Track expected DataSN per task. Gap triggers immediate SNACK. Tail loss handled by caller's context timeout. | |
| Both gap + timeout | DataSN gap for fast mid-stream recovery, per-task timer for tail loss safety net. Timeout configurable. | ✓ |

**User's choice:** Both gap + timeout
**Notes:** User requested detailed comparison of options 1 vs 3 before deciding. Key differentiator: tail loss handling -- gap-only can't detect dropped final Data-In PDUs, timeout catches this edge case.

### ERL 2 scope

| Option | Description | Selected |
|--------|-------------|----------|
| Single-conn replacement | Drop failed connection, establish new TCP, login with same ISID/TSIH + Logout for connection recovery, reassign tasks. Functional ERL 2 without MC/S. | ✓ |
| Defer ERL 2 to v2 with MC/S | Without MC/S, ERL 2 is effectively identical to ERL 0 reinstatement. | |

**User's choice:** Single-conn replacement
**Notes:** None

---

## TMF API Shape

### Invocation style

| Option | Description | Selected |
|--------|-------------|----------|
| Methods on Session | sess.AbortTask(ctx, itt), sess.LUNReset(ctx, lun), etc. Consistent with sess.Submit() pattern. | ✓ |
| Separate tmf package | tmf.AbortTask(sess, itt). Clean separation but TMF is fundamentally session-scoped. | |
| Generic SendTMF method | sess.SendTMF(ctx, function, opts...). Single entry point, less discoverable. | |

**User's choice:** Methods on Session
**Notes:** None

### Return type

| Option | Description | Selected |
|--------|-------------|----------|
| TMFResult struct | Dedicated struct with Response code and error. Different from SCSI Result. | ✓ |
| Reuse session.Result | Same Result type. Status field carries TMF response code. Semantically muddy. | |
| Just error | nil on success, typed error on failure. Simplest but loses response detail. | |

**User's choice:** TMFResult struct
**Notes:** None

### Abort side effects

| Option | Description | Selected |
|--------|-------------|----------|
| Auto-resolve with error | Aborted task's Result channel gets Result{Err: ErrTaskAborted}. Clean signal, no dangling goroutines. | ✓ |
| Caller drains separately | TMF completes independently. Caller must drain original command's Result channel. | |

**User's choice:** Auto-resolve with error
**Notes:** None

### TMF cleanup

| Option | Description | Selected |
|--------|-------------|----------|
| Auto-cleanup on success | Removes task from s.tasks, unregisters ITT, resolves Result channels. One-stop. | ✓ |
| Cleanup is caller's job | TMF just sends/receives PDU. Caller handles task state. | |

**User's choice:** Auto-cleanup on success
**Notes:** None

---

## Error Injection Testing

### Fault injection approach

| Option | Description | Selected |
|--------|-------------|----------|
| Wrapper net.Conn with fault injection | faultConn wraps net.Conn with configurable faults. Deterministic, in-process. | ✓ |
| Proxy-based injection | TCP proxy with packet manipulation. More realistic but less deterministic. | |
| Mock transport layer | Replace entire transport.Conn with scripted mock. Most controlled but least realistic. | |

**User's choice:** Wrapper net.Conn with fault injection
**Notes:** None

### Test target strategy

| Option | Description | Selected |
|--------|-------------|----------|
| faultConn + gotgt | Real gotgt target wrapped in faultConn. Tests exercise real protocol + real recovery. | |
| Synthetic PDU replay only | Script exact PDU sequences. No external target dependency. | |
| Both depending on scenario | gotgt + faultConn for connection-level faults. Synthetic PDU replay for protocol-level faults. | ✓ |

**User's choice:** Both depending on scenario
**Notes:** gotgt + faultConn for ERL 0/2 connection-level faults. Synthetic PDU replay for ERL 1 SNACK testing (missing DataSN sequences, corrupt digests).

---

## Claude's Discretion

- Internal state machine design for recovery levels
- SNACK timeout default value and backoff strategy
- Task reassignment bookkeeping during ERL 2
- TMFResult response code enum values and naming
- faultConn internal design
- Re-login orchestration during reinstatement
- Test file organization

## Deferred Ideas

None -- discussion stayed within phase scope
