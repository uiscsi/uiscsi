# Phase 6: Error Recovery and Task Management - Context

**Gathered:** 2026-04-01
**Status:** Ready for planning

<domain>
## Phase Boundary

Implement all three iSCSI error recovery levels (ERL 0, 1, 2) and all six task management functions per RFC 7143. ERL 0: automatic session reinstatement with in-flight command retry. ERL 1: SNACK-based retransmission of missing Data-In/Status PDUs. ERL 2: connection replacement with task reassignment within a single-connection session (MC/S is v2). TMF: ABORT TASK, ABORT TASK SET, LUN RESET, TARGET WARM RESET, TARGET COLD RESET, CLEAR TASK SET as methods on Session. Error injection test infrastructure for verifying recovery behavior. Does not include multi-connection sessions (MC/S), iSER, or public API polish -- those belong in other phases/versions.

</domain>

<decisions>
## Implementation Decisions

### ERL 0: Session Reinstatement
- **D-01:** Library auto-reconnects on connection loss. Session detects connection drop (via read pump error or AsyncEvent 2/3), automatically attempts reconnect + session reinstatement. Configurable via SessionOption.
- **D-02:** 3 reconnect attempts with exponential backoff (1s, 2s, 4s) by default. Configurable via `WithMaxReconnectAttempts()` and `WithReconnectBackoff()`. After exhaustion, all in-flight commands fail and `Session.Err()` returns the connection error.
- **D-03:** Reuse same ISID and present old TSIH for session reinstatement per RFC 7143 Section 6.3.5. Store ISID/TSIH in Session struct. Target recognizes as reinstatement and restores task allegiance.
- **D-04:** Retry all in-flight commands after successful session reinstatement. Commands that were in the `s.tasks` map when connection dropped are re-submitted with new CmdSN/ExpStatSN. Callers see transparent recovery.

### ERL 1: SNACK-Based Retransmission
- **D-05:** Full SNACK implementation for Data-In/Status retransmission without dropping connection. Uses existing `pdu.SNACKReq` PDU type.
- **D-06:** Dual detection: DataSN gap detection for fast mid-stream recovery (immediate SNACK when received DataSN > expected DataSN) plus per-task timeout as safety net for tail loss (final Data-In PDUs dropped). Timeout configurable via `WithSNACKTimeout()` SessionOption.
- **D-07:** DataSN gap detection extends existing `datain.go` reassembly tracking. When gap detected, SNACK Request sent with BegRun=expected DataSN and RunLength=gap size.

### ERL 2: Connection Replacement
- **D-08:** Single-connection replacement within MaxConnections=1. Drop failed connection, establish new TCP connection, login with same ISID/TSIH + Logout for connection recovery, reassign tasks to new connection. Functional ERL 2 without requiring MC/S.

### TMF API
- **D-09:** Methods on Session: `sess.AbortTask(ctx, itt)`, `sess.AbortTaskSet(ctx, lun)`, `sess.LUNReset(ctx, lun)`, `sess.TargetWarmReset(ctx)`, `sess.TargetColdReset(ctx)`, `sess.ClearTaskSet(ctx, lun)`. Consistent with `sess.Submit()` pattern. Uses existing `pdu.TaskMgmtReq` / `pdu.TaskMgmtResp` PDU types.
- **D-10:** Dedicated `TMFResult` struct with Response code (function complete, not supported, task does not exist, rejected, etc.) and error. Separate from `session.Result` since TMF responses have different semantics (no sense data, no residuals).
- **D-11:** Successful ABORT TASK auto-resolves the aborted command's Result channel with `Result{Err: ErrTaskAborted}`. No dangling goroutines -- task goroutine is cleaned up.
- **D-12:** Successful ABORT TASK / ABORT TASK SET / LUN RESET / CLEAR TASK SET auto-cleans affected tasks: removes from `s.tasks` map, unregisters ITT from Router, resolves Result channels. One-stop cleanup for callers.

### Error Injection Testing
- **D-13:** `faultConn` wrapper around `net.Conn` with configurable faults: drop after N bytes, inject read/write errors at specific points, add latency. Deterministic and reproducible. Used with `net.Pipe()` for in-process testing.
- **D-14:** Dual test approach: gotgt + faultConn for connection-level faults (ERL 0, ERL 2 -- real protocol behavior with injected transport failures). Synthetic PDU replay for protocol-level faults (missing DataSN sequences for SNACK testing, corrupt digests for ERL 1). Pick the right tool per scenario.

### Claude's Discretion
- Internal state machine design for recovery levels (ERL 0 reconnect FSM, etc.)
- SNACK timeout default value and backoff strategy
- How task reassignment bookkeeping works during ERL 2 connection replacement
- TMFResult response code enum values and naming
- faultConn internal design (hook points, configuration API)
- How re-login is orchestrated during reinstatement (reuse login package or separate path)
- Test file organization for error injection scenarios

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Error Recovery Protocol
- RFC 7143 Section 7 -- Error handling and recovery: ERL 0, 1, 2 definitions, retry rules, session reinstatement
- RFC 7143 Section 6.3.5 -- Session reinstatement: ISID/TSIH reuse, task allegiance
- RFC 7143 Section 11.16 -- SNACK Request PDU format (Type, BegRun, RunLength)
- RFC 7143 Section 7.2 -- Retry and reassign in recovery: CmdSN rules for retried commands
- RFC 7143 Section 7.3 -- Usage of retry: when and how to retry after connection failure
- RFC 7143 Section 7.4 -- Recovery classes: within-command, within-connection, connection, session recovery
- RFC 7143 Section 13.19 -- ErrorRecoveryLevel negotiation key
- RFC 7143 Section 13.8 -- DefaultTime2Wait (reinstatement timing)
- RFC 7143 Section 13.7 -- DefaultTime2Retain (task allegiance window)

### Task Management Protocol
- RFC 7143 Section 11.5 -- Task Management Function Request PDU format (Function codes, ReferencedTaskTag, RefCmdSN)
- RFC 7143 Section 11.6 -- Task Management Function Response PDU format (Response codes)
- RFC 7143 Section 11.5.1 -- Function definitions: ABORT TASK (1), ABORT TASK SET (2), LOGICAL UNIT RESET (5), TARGET WARM RESET (6), TARGET COLD RESET (7), CLEAR TASK SET (3)

### Existing Code
- `internal/session/session.go` -- Session struct, Submit, task tracking (s.tasks map), background goroutines
- `internal/session/types.go` -- Command, Result, AsyncEvent, SessionOption, sessionConfig
- `internal/session/async.go` -- handleAsyncMsg with EventCode 2/3 (connection/session drop) -- ERL 0 trigger point
- `internal/session/datain.go` -- Data-In reassembly with DataSN tracking -- extend for SNACK gap detection
- `internal/session/dataout.go` -- Data-Out generation -- in-flight write recovery
- `internal/session/keepalive.go` -- NOP-Out/In keepalive -- connection liveness detection
- `internal/session/logout.go` -- Logout for connection recovery (EVT-03, already implemented)
- `internal/session/cmdwindow.go` -- CmdSN windowing -- retried commands need new CmdSN
- `internal/pdu/initiator.go` -- TaskMgmtReq (lines 79-110), SNACKReq (lines 266-295) -- both fully implemented
- `internal/pdu/target.go` -- TaskMgmtResp (lines 102-130) -- fully implemented
- `internal/transport/router.go` -- ITT-based PDU correlation -- used for TMF response routing
- `internal/transport/conn.go` -- Conn wrapper -- faultConn extends this pattern
- `internal/login/params.go` -- NegotiatedParams with ErrorRecoveryLevel, DefaultTime2Wait, DefaultTime2Retain
- `internal/login/login.go` -- Login flow -- reused for session reinstatement

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `pdu.TaskMgmtReq` / `pdu.TaskMgmtResp` -- fully implemented with Marshal/Unmarshal, Function field, ReferencedTaskTag, RefCmdSN
- `pdu.SNACKReq` -- fully implemented with Type, BegRun, RunLength fields
- `Session.handleAsyncMsg` -- already handles EventCode 2 (connection drop) and 3 (session drop) with error recording
- `transport.Router` -- ITT allocation and PDU correlation, RegisterPersistent for multi-PDU responses
- `login.NegotiatedParams.ErrorRecoveryLevel` -- already negotiated, stored, accessible via `s.params`
- `Session.logout()` -- internal logout method, reusable for connection recovery logout (ERL 2)
- `cmdWindow` -- CmdSN/ExpCmdSN/MaxCmdSN management, retried commands acquire new CmdSN slots

### Established Patterns
- Per-task goroutine (`taskLoop`) drains Router channel -- recovery must terminate these cleanly before retry
- `s.tasks` map tracks all in-flight tasks by ITT -- snapshot for retry after reinstatement
- `writeCh` serializes all outgoing PDUs -- TMF requests use same write channel
- Functional options pattern (`WithX()`) for all SessionOption configuration -- extend for recovery options
- `s.closeOnce` / `s.done` / `s.cancel()` for session lifecycle -- recovery needs careful interaction with these

### Integration Points
- ERL 0 trigger: read pump error or AsyncEvent 2/3 in `handleAsyncMsg` -- initiate reconnect
- SNACK trigger: DataSN gap in `datain.go` reassembly or per-task timeout -- send via writeCh
- TMF methods: build TaskMgmtReq PDU, send via writeCh, register for TaskMgmtResp via Router
- faultConn: wraps `net.Conn` before passing to `transport.NewConn()` -- transparent to transport layer
- Session reinstatement: re-run `login.Login()` with same ISID/TSIH, then rebuild Session internal state

</code_context>

<specifics>
## Specific Ideas

- ERL 0 auto-reconnect should feel transparent to callers -- commands that were in-flight when connection dropped complete normally after reinstatement, as if nothing happened
- SNACK dual detection (gap + timeout) mirrors how production initiators work -- gap detection handles 95% of cases fast, timeout is safety net for tail loss edge case
- TMF auto-cleanup is critical -- callers should not have to manually drain aborted command channels or clean up task state after abort/reset
- faultConn approach allows precise, deterministic fault injection without external infrastructure -- consistent with project constraint "testable without manual infrastructure setup"

</specifics>

<deferred>
## Deferred Ideas

None -- discussion stayed within phase scope

</deferred>

---

*Phase: 06-error-recovery-and-task-management*
*Context gathered: 2026-04-01*
