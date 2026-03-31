# Phase 1: PDU Codec and Transport - Context

**Gathered:** 2026-03-31
**Status:** Ready for planning

<domain>
## Phase Boundary

Binary encoding/decoding of all 24 iSCSI PDU opcodes, CRC32C digest computation, RFC 1982 serial number arithmetic, TCP connection management with goroutine-based read/write pumps, and ITT-based PDU routing. This phase delivers the protocol foundation that all subsequent phases build upon.

</domain>

<decisions>
## Implementation Decisions

### PDU Representation
- **D-01:** Claude's discretion on struct model (typed per-opcode vs generic + typed) — prioritize reliability over cleverness
- **D-02:** PDU fields accessed via parsed structs (decode BHS into Go struct fields: Opcode, Flags, LUN, ITT, etc.) — clean API, accept allocation per decode
- **D-03:** All 24 iSCSI opcodes implemented as separate types from day one — complete codec now, no churn in later phases

### Buffer Strategy
- **D-04:** sync.Pool for reusable byte buffers (BHS and data segments) to reduce GC pressure under load
- **D-05:** Copy-out ownership model — decoder copies data into new buffer owned by caller, safe to hold indefinitely. Pool manages transport-layer scratch buffers only.
- **D-06:** MaxRecvDataSegmentLength enforcement at the transport layer, not the PDU codec — separation of concerns

### Package Layout
- **D-07:** Layered internal structure — public `iscsi/` API package, with `internal/pdu/`, `internal/transport/`, `internal/serial/` hidden behind Go's internal package boundary
- **D-08:** Module path: `github.com/{user}/uiscsi` — standard Go convention (exact GitHub org/user TBD)

### Test Strategy
- **D-09:** Phase 1 uses mock net.Conn (net.Pipe()) for transport unit tests, plus gotgt for transport framing smoke tests against a real target
- **D-10:** gotgt setup approach (in-process vs subprocess) deferred to phase researcher — let research determine best API
- **D-11:** Tests organized alongside code (pdu_test.go next to pdu.go) — standard Go convention. Integration/conformance tests introduced in later phases.

### Claude's Discretion
- PDU struct model choice (typed per-opcode vs generic + typed) — must prioritize reliability
- Exact sync.Pool sizing and buffer growth strategy
- Internal package naming and file organization within each package
- Whether to use encoding/binary or manual byte slicing for BHS serialization (profile if needed)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### iSCSI Protocol
- RFC 7143 — iSCSI Protocol (Consolidated): PDU formats (Section 11), BHS structure (Section 11.2), all opcode definitions, header/data digest (Section 12.1), text negotiation keys (Section 13)
- RFC 1982 — Serial Number Arithmetic: sequence number comparison rules for CmdSN/StatSN/DataSN wrap-around handling

### Project Research
- `.planning/research/STACK.md` — Technology choices, stdlib recommendations, Go 1.25 features
- `.planning/research/ARCHITECTURE.md` — Component boundaries, build order, goroutine model
- `.planning/research/PITFALLS.md` — Serial arithmetic bugs, TCP writer corruption, PDU framing pitfalls, GC pressure from per-PDU allocations

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- None — greenfield project, no existing code

### Established Patterns
- None yet — this phase establishes the foundational patterns for all subsequent phases

### Integration Points
- PDU codec is consumed by connection layer (Phase 2)
- Transport is consumed by login state machine (Phase 2) and session layer (Phase 3)
- Serial arithmetic is consumed by command windowing (Phase 3) and error recovery (Phase 6)

</code_context>

<specifics>
## Specific Ideas

- Follow The Bronx Method: standards compliance non-negotiable, minimal ceremony, every abstraction must justify its existence
- Research identified CRC32C Castagnoli via `hash/crc32` with potential hardware acceleration (SSE 4.2) — verify on NetBSD
- Research flagged `io.ReadFull` vs raw `Read` as a critical framing decision — must use ReadFull for PDU BHS reads
- STATE.md notes: verify Go CRC32C hardware acceleration and TCP networking on NetBSD 10.1

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 01-pdu-codec-and-transport*
*Context gathered: 2026-03-31*
