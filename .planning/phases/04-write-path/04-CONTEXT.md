# Phase 4: Write Path - Context

**Gathered:** 2026-04-01
**Status:** Ready for planning

<domain>
## Phase Boundary

Implement the iSCSI write path: R2T handling, Data-Out PDU generation, immediate data piggybacking, unsolicited Data-Out, and burst length enforcement. Callers can write data to an iSCSI target through all four ImmediateData x InitialR2T combinations with correct MaxBurstLength and MaxOutstandingR2T compliance. Does not include error recovery (ERL 1/2), task management (TMF), or high-level typed write helpers -- those belong in later phases.

</domain>

<decisions>
## Implementation Decisions

### Write API Shape
- **D-01:** Write data provided as `io.Reader` on the Command struct (`cmd.Data`). Callers with `[]byte` use `bytes.NewReader()`. Symmetric with the read path's `io.Reader` output.
- **D-02:** Session auto-detects write behavior from NegotiatedParams -- callers just Submit with data. Session handles ImmediateData piggybacking, unsolicited Data-Out, and R2T-solicited writes automatically based on negotiated InitialR2T, ImmediateData, FirstBurstLength, MaxBurstLength.
- **D-03:** `cmd.Data != nil` means write. If Command.Data is non-nil, it's a write command. If nil, it's a read or non-data command. No explicit Direction field needed.

### R2T Flow Control
- **D-04:** Sequential R2T processing per task. Even when MaxOutstandingR2T > 1, process R2Ts one at a time within a single task goroutine. Most targets use MaxOutstandingR2T=1. Can be optimized later if benchmarking shows a bottleneck.
- **D-05:** Extend existing per-task goroutine pattern. The per-task goroutine from the read path (drains Router channel for Data-In) also handles R2T PDUs for writes: receives R2T, reads chunk from io.Reader, sends Data-Out. Same goroutine, same lifetime.

### Data Chunking Strategy
- **D-06:** Read-on-demand from io.Reader. When R2T arrives, read exactly the needed chunk (up to MaxRecvDataSegmentLength per PDU, up to MaxBurstLength per R2T sequence). For immediate/unsolicited data, read FirstBurstLength upfront. No pre-buffering of entire write data.
- **D-07:** Reuse transport layer's buffer pool for Data-Out PDU data segments. Consistent with Phase 1's size-class buffer pooling (4KB/64KB/16MB) and copy-out ownership model.

### Error Semantics
- **D-08:** Write commands return same Result type as reads, with Data=nil. Status, SenseData, Err fields carry write outcome. Residual counts indicate how much data the target didn't accept. Uniform error handling for callers.
- **D-09:** On io.Reader error mid-write, abort the task and return the Reader error in Result.Err. No Data-Out PDUs sent after Reader failure. No iSCSI-level task abort (TMF is Phase 6). Target will time out the incomplete R2T sequence.

### Claude's Discretion
- DataSN numbering within Data-Out sequences
- How immediate data is attached to the SCSI Command PDU (inline in BHS data segment vs separate)
- Internal write task state machine design
- Buffer offset tracking across R2T sequences
- How ExpectedDataTransferLength is set on the SCSI Command PDU for writes

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Write Path Protocol
- RFC 7143 Section 11.8 -- Data-Out PDU format (DataSN, offset, Final bit, data segment)
- RFC 7143 Section 11.9 -- R2T PDU format (R2TSN, DesiredDataTransferLength, BufferOffset)
- RFC 7143 Section 11.2 -- SCSI Command PDU (ImmediateData flag, ExpectedDataTransferLength, W-bit)
- RFC 7143 Section 4.2 -- Command ordering: immediate data bounded by FirstBurstLength
- RFC 7143 Section 13.10 -- InitialR2T negotiation key (Yes/No, default Yes)
- RFC 7143 Section 13.9 -- ImmediateData negotiation key (Yes/No, default Yes)
- RFC 7143 Section 13.14 -- FirstBurstLength (max unsolicited data, default 65536)
- RFC 7143 Section 13.13 -- MaxBurstLength (max solicited data per R2T sequence, default 262144)
- RFC 7143 Section 13.17 -- MaxOutstandingR2T (max concurrent R2Ts, default 1)
- RFC 7143 Section 13.15 -- MaxRecvDataSegmentLength (per-PDU data segment limit)

### Existing Code
- `internal/session/session.go` -- Session struct, Submit, dispatchLoop, per-task goroutine pattern
- `internal/session/types.go` -- Command, Result, task types to extend for write support
- `internal/session/datain.go` -- Data-In reassembly pattern (symmetric reference for Data-Out generation)
- `internal/session/cmdwindow.go` -- CmdSN windowing (writes consume CmdSN like reads)
- `internal/pdu/initiator.go` -- DataOut PDU type (already implemented in Phase 1)
- `internal/pdu/target.go` -- R2T PDU type (already implemented in Phase 1)
- `internal/transport/pool.go` -- Buffer pool with size classes
- `internal/login/params.go` -- NegotiatedParams with InitialR2T, ImmediateData, FirstBurstLength, MaxBurstLength, MaxOutstandingR2T

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `pdu.DataOut` -- Already has DataSN, BufferOffset, Final, MarshalBHS/UnmarshalBHS from Phase 1
- `pdu.R2T` -- Already has R2TSN, DesiredDataTransferLength, BufferOffset, UnmarshalBHS from Phase 1
- `transport.Pool` -- Size-class buffer pool for Data-Out data segment allocation
- `session.task` -- Per-task goroutine pattern with Router channel. Extend for write-side R2T handling
- `login.NegotiatedParams` -- All write-relevant parameters already negotiated and stored

### Established Patterns
- Per-task goroutine drains Router channel (read path) -- extend to handle R2T PDUs for writes
- Buffered Data-In reassembly via bytes.Buffer (Phase 3) -- inverse pattern for Data-Out generation
- CmdSN acquired before command send (read path) -- writes follow same windowing
- Router.Register for single-response, Router.RegisterPersistent for multi-PDU correlation

### Integration Points
- Submit detects write via `cmd.Data != nil`, sets W-bit on SCSI Command PDU
- Per-task goroutine receives R2T PDUs via Router channel, generates Data-Out PDUs
- Data-Out PDUs sent through writeCh (existing write serialization channel)
- NegotiatedParams consulted for ImmediateData, InitialR2T, burst lengths at Submit time

</code_context>

<specifics>
## Specific Ideas

- Read-on-demand from io.Reader mirrors how production initiators work -- they don't pre-buffer entire writes because MaxBurstLength can be 16MB+
- Sequential R2T processing is the conservative choice; parallel R2T handling can be added as an optimization in a future phase if benchmarking against real targets shows it matters
- The four ImmediateData x InitialR2T combinations must all be tested parametrically -- this is the most error-prone part of the write path

</specifics>

<deferred>
## Deferred Ideas

None -- discussion stayed within phase scope

</deferred>

---

*Phase: 04-write-path*
*Context gathered: 2026-04-01*
