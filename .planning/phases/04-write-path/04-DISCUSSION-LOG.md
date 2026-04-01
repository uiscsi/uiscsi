# Phase 4: Write Path - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md -- this log preserves the alternatives considered.

**Date:** 2026-04-01
**Phase:** 04-write-path
**Areas discussed:** Write API shape, R2T flow control, Data chunking strategy, Error semantics

---

## Write API Shape

### How callers provide write data

| Option | Description | Selected |
|--------|-------------|----------|
| io.Reader | Symmetric with read path. Session reads chunks as R2Ts arrive. Memory-efficient. | ✓ |
| []byte | Simpler but requires full write in memory. | |
| Both via interface | Accept io.Reader, callers use bytes.NewReader for []byte. | |

**User's choice:** io.Reader
**Notes:** Matches established Phase 3 pattern of io.Reader for data delivery.

### Auto-detect vs caller controls

| Option | Description | Selected |
|--------|-------------|----------|
| Auto-detect from NegotiatedParams | Session checks ImmediateData, InitialR2T, burst lengths. Callers just Submit with data. | ✓ |
| Caller controls | Caller sets flags like cmd.Immediate=true. Leaks protocol details. | |

**User's choice:** Auto-detect
**Notes:** Keeps protocol complexity inside the session layer.

### Read vs write distinction

| Option | Description | Selected |
|--------|-------------|----------|
| cmd.Data != nil means write | If Command.Data is non-nil, it's a write. Simple, matches SCSI semantics. | ✓ |
| Explicit Direction field | More explicit but redundant with Data presence. | |
| CDB-inferred | Parse CDB opcode. Fragile without SCSI-layer knowledge. | |

**User's choice:** cmd.Data != nil means write

---

## R2T Flow Control

### Concurrent R2T handling

| Option | Description | Selected |
|--------|-------------|----------|
| Sequential per task | Process R2Ts one at a time. Simpler, most targets use MaxOutstandingR2T=1. | ✓ |
| Parallel goroutines | Goroutine per R2T. More complex, needs io.Reader seek coordination. | |
| Buffered pipeline | Channel + worker pool bounded by MaxOutstandingR2T. | |

**User's choice:** Sequential per task
**Notes:** Optimization deferred until benchmarking shows need.

### Task goroutine reuse

| Option | Description | Selected |
|--------|-------------|----------|
| Extend existing per-task goroutine | Same goroutine handles both Data-In (reads) and R2T (writes). | ✓ |
| Separate write goroutine | Dedicated goroutine for write-side handling. | |

**User's choice:** Extend existing per-task goroutine

---

## Data Chunking Strategy

### Write data buffering

| Option | Description | Selected |
|--------|-------------|----------|
| Read-on-demand from io.Reader | Read chunks as R2Ts arrive. No pre-buffering. | ✓ |
| Pre-buffer entire write | Read all data at Submit time. Defeats io.Reader purpose. | |
| Chunked pre-read per R2T | Pre-read MaxBurstLength per R2T. Middle ground. | |

**User's choice:** Read-on-demand from io.Reader

### Buffer pool reuse

| Option | Description | Selected |
|--------|-------------|----------|
| Reuse transport pool | Phase 1 size-class pooling for Data-Out data segments. | ✓ |
| Stack-allocated per PDU | New allocation per PDU. More GC pressure. | |

**User's choice:** Reuse transport pool

---

## Error Semantics

### Write error reporting

| Option | Description | Selected |
|--------|-------------|----------|
| Same Result, Data=nil | Uniform with reads. Status, SenseData, Err carry write outcome. | ✓ |
| Separate WriteResult type | More precise but diverges from read path. | |
| Result with BytesWritten | Single type, slightly overloaded. | |

**User's choice:** Same Result, Data=nil

### io.Reader failure mid-write

| Option | Description | Selected |
|--------|-------------|----------|
| Abort task, return error in Result | Stop sending Data-Out. Target times out R2T. No TMF abort (Phase 6). | ✓ |
| Abort + send TMF ABORT TASK | Cleaner but depends on Phase 6. | |
| Retry read from io.Reader | Only works if Reader supports seeking. | |

**User's choice:** Abort task, return error in Result

## Claude's Discretion

- DataSN numbering within Data-Out sequences
- How immediate data is attached to SCSI Command PDU
- Internal write task state machine design
- Buffer offset tracking across R2T sequences
- How ExpectedDataTransferLength is set on SCSI Command PDU for writes

## Deferred Ideas

None -- discussion stayed within phase scope
