# Phase 1: PDU Codec and Transport - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-03-31
**Phase:** 01-PDU Codec and Transport
**Areas discussed:** PDU representation, Buffer strategy, Package layout, Test target

---

## PDU Representation

### PDU Model

| Option | Description | Selected |
|--------|-------------|----------|
| Typed structs | One struct per opcode with PDU interface — type-safe, verbose | |
| Generic + typed | Generic PDU with BHS fields + type-asserted payload structs | |
| You decide | Claude picks based on RFC 7143 structure | ✓ |

**User's choice:** You decide — but keep in mind reliability
**Notes:** Reliability is the priority over cleverness or performance tricks

### Field Access

| Option | Description | Selected |
|--------|-------------|----------|
| Parsed structs | Decode BHS into struct fields — clean API, allocation per decode | ✓ |
| Lazy byte view | Methods read fields from underlying []byte — zero-alloc, less ergonomic | |
| Both layers | Byte-view for hot path, parsed structs for consumer code | |

**User's choice:** Parsed structs

### Opcode Coverage

| Option | Description | Selected |
|--------|-------------|----------|
| All 24 upfront | Complete PDU codec now — no churn later | ✓ |
| Phase-gated | Start with subset, add as needed | |
| You decide | Claude picks | |

**User's choice:** All 24 upfront

---

## Buffer Strategy

### Buffer Allocation

| Option | Description | Selected |
|--------|-------------|----------|
| sync.Pool | Reusable byte buffers — reduces GC pressure | ✓ |
| Fresh alloc | Allocate per PDU, let GC handle — simpler | |
| You decide | Claude picks | |

**User's choice:** sync.Pool

### Buffer Ownership

| Option | Description | Selected |
|--------|-------------|----------|
| Copy out | Decoder copies data, caller owns it — safe | ✓ |
| Borrow + return | Caller borrows, must return to pool — zero-copy | |
| You decide | Claude picks | |

**User's choice:** Copy out

### MaxRecvDataSegmentLength Enforcement

| Option | Description | Selected |
|--------|-------------|----------|
| Codec enforces | Decoder rejects oversized segments | |
| Transport layer | Transport checks before passing to decoder | ✓ |
| You decide | Claude picks | |

**User's choice:** Transport layer

---

## Package Layout

### Module Structure

| Option | Description | Selected |
|--------|-------------|----------|
| Layered internal | iscsi/ public API, internal/pdu/, internal/transport/, internal/serial/ | ✓ |
| Flat public | Single iscsi/ package | |
| Domain packages | pdu/, transport/, session/, scsi/ all public | |
| You decide | Claude picks | |

**User's choice:** Layered internal

### Module Path

| Option | Description | Selected |
|--------|-------------|----------|
| github.com based | Standard Go convention | ✓ |
| Custom domain | Vanity import path | |
| You decide | Claude picks | |

**User's choice:** github.com based

---

## Test Target

### Phase 1 Test Approach

| Option | Description | Selected |
|--------|-------------|----------|
| Mock net.Conn | net.Pipe() for transport tests, no real target | |
| gotgt early | Set up gotgt from Phase 1 | |
| Both | Mock for unit tests, gotgt for transport smoke tests | ✓ |

**User's choice:** Both

### gotgt Setup Method

| Option | Description | Selected |
|--------|-------------|----------|
| In-process | Import gotgt as Go library, start in TestMain | |
| Subprocess | Spawn gotgt binary as subprocess | |
| Defer to research | Let researcher determine best approach | ✓ |

**User's choice:** Defer to research

### Test Organization

| Option | Description | Selected |
|--------|-------------|----------|
| Alongside code | *_test.go next to source — standard Go | ✓ |
| Separate suite | tests/ directory for integration tests | |
| Both | Unit alongside + separate integration/ | |

**User's choice:** Alongside code

---

## Claude's Discretion

- PDU struct model (typed per-opcode vs generic + typed) — prioritize reliability
- sync.Pool sizing and buffer growth strategy
- Internal package naming and file organization
- encoding/binary vs manual byte slicing for BHS serialization

## Deferred Ideas

None — discussion stayed within phase scope
