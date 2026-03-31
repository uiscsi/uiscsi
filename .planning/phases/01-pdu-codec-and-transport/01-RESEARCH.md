# Phase 1: PDU Codec and Transport - Research

**Researched:** 2026-03-31
**Domain:** iSCSI PDU binary encoding/decoding, CRC32C digest, RFC 1982 serial arithmetic, TCP transport with goroutine pumps
**Confidence:** HIGH

## Summary

Phase 1 builds the protocol foundation: binary PDU codec for all 24 iSCSI opcodes, CRC32C digest computation, serial number arithmetic, and TCP transport with concurrent read/write pumps. The entire phase uses Go stdlib only (no external runtime dependencies). The key technical challenges are: (1) correct BHS byte-level layout with bitfield packing, (2) proper PDU framing over TCP using `io.ReadFull`, (3) CRC32C using the Castagnoli polynomial (verified working on NetBSD), and (4) serial number wrap-around arithmetic for sequence number comparisons.

All 24 opcodes (8 initiator, 10 target including Reject) must be implemented as separate Go types from day one per decision D-03. The PDU codec is pure encode/decode with no I/O -- transport reads/writes are separate. Buffer management uses `sync.Pool` for scratch buffers with copy-out ownership (D-04, D-05).

**Primary recommendation:** Build bottom-up: serial arithmetic helpers first, then PDU types and codec, then CRC32C digest, then TCP transport with read/write pumps and ITT routing. Each layer is independently testable with table-driven tests.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Claude's discretion on struct model (typed per-opcode vs generic + typed) -- prioritize reliability over cleverness
- **D-02:** PDU fields accessed via parsed structs (decode BHS into Go struct fields: Opcode, Flags, LUN, ITT, etc.) -- clean API, accept allocation per decode
- **D-03:** All 24 iSCSI opcodes implemented as separate types from day one -- complete codec now, no churn in later phases
- **D-04:** sync.Pool for reusable byte buffers (BHS and data segments) to reduce GC pressure under load
- **D-05:** Copy-out ownership model -- decoder copies data into new buffer owned by caller, safe to hold indefinitely. Pool manages transport-layer scratch buffers only.
- **D-06:** MaxRecvDataSegmentLength enforcement at the transport layer, not the PDU codec -- separation of concerns
- **D-07:** Layered internal structure -- public `iscsi/` API package, with `internal/pdu/`, `internal/transport/`, `internal/serial/` hidden behind Go's internal package boundary
- **D-08:** Module path: `github.com/{user}/uiscsi` -- standard Go convention (exact GitHub org/user TBD)
- **D-09:** Phase 1 uses mock net.Conn (net.Pipe()) for transport unit tests, plus gotgt for transport framing smoke tests against a real target
- **D-10:** gotgt setup approach (in-process vs subprocess) deferred to researcher -- see gotgt section below
- **D-11:** Tests organized alongside code (pdu_test.go next to pdu.go) -- standard Go convention

### Claude's Discretion
- PDU struct model choice (typed per-opcode vs generic + typed) -- must prioritize reliability
- Exact sync.Pool sizing and buffer growth strategy
- Internal package naming and file organization within each package
- Whether to use encoding/binary or manual byte slicing for BHS serialization (profile if needed)

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| PDU-01 | Binary PDU encoder/decoder for all iSCSI PDU types (BHS + AHS + data segment + padding) | BHS layout documented below, all 24 opcodes enumerated, padding formula verified, AHS handling required |
| PDU-02 | RFC 1982 serial number arithmetic for all sequence number comparisons | Algorithm verified on Go/NetBSD using int32 cast trick, test vectors for wrap-around documented |
| PDU-03 | CRC32C (Castagnoli) computation for header and data digests | `hash/crc32` with `crc32.Castagnoli` verified on NetBSD (0xe3069283 for "123456789"), scope rules documented |
| PDU-04 | PDU padding to 4-byte boundaries per RFC 7143 | Padding formula: `(4 - (len % 4)) % 4`, padding bytes must be zero |
| XPORT-01 | TCP connection management with configurable timeouts and context cancellation | stdlib `net` package, `net.DialContext`, `SetDeadline` for timeouts |
| XPORT-02 | PDU framing over TCP (read full BHS, then AHS + data segment based on lengths) | Must use `io.ReadFull`, framing pipeline documented below |
| XPORT-03 | Dedicated read/write goroutine pumps per connection (no concurrent TCP writes) | Channel-based write pump pattern, `errgroup` for lifecycle |
| XPORT-04 | ITT-based PDU routing/correlation | `map[uint32]chan<- *PDU` with `sync.Mutex`, ITT 0xFFFFFFFF reserved |
| TEST-03 | Table-driven unit tests for PDU encoding/decoding | All PDU types get round-trip encode/decode tests, edge cases documented |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Language:** Go 1.25 (verified: go1.25.5 netbsd/amd64 available)
- **Dependencies:** Minimal external -- stdlib only for runtime, gotgt for integration tests only
- **Standard:** RFC 7143 compliance drives all implementation decisions
- **Testing:** Must work without manual infrastructure; `go test -race` on every run
- **API style:** `context.Context` for cancellation, `io.Reader`/`io.Writer` where natural, structured errors
- **Quality:** High test coverage, no dead code, no speculative abstractions
- **Forbidden:** testify, protobuf, any kernel-dependent iSCSI libraries, third-party logging
- **Linting:** golangci-lint with errcheck, govet, staticcheck, unused, sloglint

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go | 1.25.5 | Language runtime | Verified on NetBSD 10.1 amd64. Has `testing/synctest` (graduated), range-over-func. |
| `encoding/binary` | stdlib | BHS field encode/decode | `binary.BigEndian.PutUint32`/`Uint32` for network byte order PDU fields. Direct byte manipulation, not struct-based Read/Write (too slow for hot path). |
| `hash/crc32` | stdlib | CRC32C digest | `crc32.MakeTable(crc32.Castagnoli)` -- verified on NetBSD: CRC32C("123456789") = 0xe3069283. Hardware-accelerated on amd64 via SSE4.2. |
| `net` | stdlib | TCP connections | `net.DialContext` for connection with timeout, `net.Pipe()` for unit tests. |
| `io` | stdlib | Exact reads | `io.ReadFull` for PDU framing -- critical to avoid partial reads on TCP stream. |
| `sync` | stdlib | Concurrency primitives | `sync.Pool` for buffer reuse, `sync.Mutex` for ITT map, `sync.Once` for init. |
| `context` | stdlib | Cancellation/timeouts | All transport operations accept `context.Context`. |
| `log/slog` | stdlib | Structured logging | Connection lifecycle, PDU traces at debug level. |
| `testing/synctest` | stdlib (Go 1.25) | Concurrent test determinism | API is `synctest.Test(t, func())` (NOT `synctest.Run`). Virtualizes time in bubbles. |

### Supporting (test only)
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `gostor/gotgt` | HEAD (Go 1.25 compatible) | Integration test target | Transport framing smoke tests against a real iSCSI target. Compatible with Go 1.25 (go.mod specifies go 1.25.0). |

**Installation:**
```bash
go mod init github.com/rkujawa/uiscsi  # or appropriate org
# No runtime dependencies -- stdlib only
# Test dependency:
go get github.com/gostor/gotgt@latest
```

## Architecture Patterns

### Recommended Package Structure
```
internal/
    pdu/
        opcode.go           # Opcode constants, opcode string names
        bhs.go              # BHS byte layout constants, common BHS encode/decode
        header.go           # Common header interface/base type
        initiator.go        # All 8 initiator opcode types + encode/decode
        target.go           # All 10 target opcode types + encode/decode
        ahs.go              # AHS types and parsing
        padding.go          # 4-byte padding helpers
        pdu.go              # Top-level PDU type (header + data segment)
        pdu_test.go         # Round-trip tests for all PDU types
    digest/
        crc32c.go           # CRC32C computation, header/data digest
        crc32c_test.go      # Test vectors from RFC 3720
    serial/
        serial.go           # RFC 1982 serial number arithmetic
        serial_test.go      # Wrap-around edge case tests
    transport/
        conn.go             # TCP connection wrapper, dial, deadline management
        framer.go           # PDU framing: read BHS, compute remaining, read rest
        pump.go             # Read pump + write pump goroutines
        router.go           # ITT-based PDU dispatch
        pool.go             # sync.Pool buffer management
        conn_test.go        # net.Pipe() based unit tests
        pump_test.go        # Concurrent pump tests with -race
        framer_test.go      # Framing edge cases (partial reads, back-to-back)
```

### Pattern 1: Typed PDU per Opcode with Common Interface

**What:** Each of the 24 opcodes gets its own Go struct implementing a common `PDU` interface. A base `Header` struct contains shared BHS fields. Opcode-specific fields are additional struct fields.

**When to use:** Always -- this is decision D-03.

**Why reliable:** Type safety catches opcode misuse at compile time. Each struct's `MarshalBinary`/`UnmarshalBinary` methods encode exactly the fields for that opcode. No runtime type assertions on opcode-specific fields.

**Example:**
```go
// Common interface for all PDU types
type PDU interface {
    Opcode() OpCode
    MarshalBHS() ([48]byte, error)
    // DataSegment returns the data segment (may be nil)
    DataSegment() []byte
}

// Shared BHS fields present in all PDUs
type Header struct {
    Immediate        bool
    OpCode           OpCode
    Final            bool
    TotalAHSLength   uint8    // in 4-byte words
    DataSegmentLen   uint32   // 24-bit value, max 16MB
    LUN              uint64   // 8 bytes
    InitiatorTaskTag uint32
}

// Example: SCSI Command (opcode 0x01)
type SCSICommand struct {
    Header
    Flags              uint8  // R, W, Attr bits
    ExpectedDataLen    uint32
    CmdSN              uint32
    ExpStatSN          uint32
    CDB                [16]byte
    // AHS for extended CDB if needed
    AdditionalCDB      []byte
    ImmediateData      []byte
}

func (c *SCSICommand) Opcode() OpCode { return OpSCSICommand }

func (c *SCSICommand) MarshalBHS() ([48]byte, error) {
    var bhs [48]byte
    bhs[0] = byte(c.OpCode)
    if c.Immediate {
        bhs[0] |= 0x40
    }
    bhs[1] = c.Flags
    bhs[4] = c.TotalAHSLength
    // DataSegmentLength in bytes 5-7 (24-bit big-endian)
    bhs[5] = byte(c.DataSegmentLen >> 16)
    bhs[6] = byte(c.DataSegmentLen >> 8)
    bhs[7] = byte(c.DataSegmentLen)
    binary.BigEndian.PutUint64(bhs[8:16], c.LUN)
    binary.BigEndian.PutUint32(bhs[16:20], c.InitiatorTaskTag)
    binary.BigEndian.PutUint32(bhs[20:24], c.ExpectedDataLen)
    binary.BigEndian.PutUint32(bhs[24:28], c.CmdSN)
    binary.BigEndian.PutUint32(bhs[28:32], c.ExpStatSN)
    copy(bhs[32:48], c.CDB[:])
    return bhs, nil
}
```

### Pattern 2: PDU Framing Pipeline

**What:** Read pump reads PDUs in stages: (1) read exactly 48 bytes BHS, (2) parse AHSLength and DataSegmentLength, (3) compute remaining bytes, (4) read remaining in one `io.ReadFull` call.

**When to use:** All PDU reads from TCP.

**Example:**
```go
func ReadPDU(r io.Reader, digestHeader, digestData bool) (*RawPDU, error) {
    // Stage 1: Read 48-byte BHS
    var bhs [48]byte
    if _, err := io.ReadFull(r, bhs[:]); err != nil {
        return nil, fmt.Errorf("read BHS: %w", err)
    }

    ahsLen := uint32(bhs[4]) * 4 // TotalAHSLength is in 4-byte words
    dsLen := uint32(bhs[5])<<16 | uint32(bhs[6])<<8 | uint32(bhs[7])
    padding := (4 - (dsLen % 4)) % 4

    // Stage 2: Compute total remaining bytes
    remaining := ahsLen
    if digestHeader { remaining += 4 }
    remaining += dsLen + padding
    if digestData && dsLen > 0 { remaining += 4 }

    // Stage 3: Read all remaining bytes in one call
    var payload []byte
    if remaining > 0 {
        payload = make([]byte, remaining) // or from sync.Pool
        if _, err := io.ReadFull(r, payload); err != nil {
            return nil, fmt.Errorf("read payload (%d bytes): %w", remaining, err)
        }
    }

    return &RawPDU{BHS: bhs, Payload: payload, AHSLen: ahsLen, DSLen: dsLen}, nil
}
```

### Pattern 3: Channel-Based Write Pump

**What:** Single goroutine owns TCP writes. All senders put PDUs on a buffered channel.

**Example:**
```go
func (c *Conn) writePump(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case pdu := <-c.writeCh:
            if err := c.writePDU(pdu); err != nil {
                return fmt.Errorf("write PDU: %w", err)
            }
        }
    }
}

func (c *Conn) Send(pdu PDU) error {
    select {
    case c.writeCh <- pdu:
        return nil
    case <-c.done:
        return ErrConnectionClosed
    }
}
```

### Anti-Patterns to Avoid
- **Multiple goroutines writing to net.Conn:** Interleaves PDU bytes. Use single write pump.
- **conn.Read() instead of io.ReadFull():** Returns partial reads. Always use io.ReadFull for BHS and payload.
- **Raw uint32 comparison on sequence numbers:** Breaks at wrap-around. Always use serial arithmetic.
- **encoding/binary.Read for BHS decoding:** Uses reflection, allocates, ~3-5x slower. Use direct byte manipulation.
- **Padding as `(4 - (n % 4))`:** Produces 4 when n is already aligned. Must be `(4 - (n % 4)) % 4`.

## iSCSI PDU Reference

### All 24 Opcodes

**Initiator Opcodes (8):**
| Opcode | Hex | Name | Key Fields |
|--------|-----|------|------------|
| 0x00 | 0x00 | NOP-Out | ITT, TTT (target transfer tag), ping data |
| 0x01 | 0x01 | SCSI Command | CDB[16], LUN, ExpectedDataLen, R/W flags |
| 0x02 | 0x02 | Task Management Function Request | Function code, Referenced Task Tag, LUN |
| 0x03 | 0x03 | Login Request | CSG, NSG, T bit, ISID, TSIH, CID |
| 0x04 | 0x04 | Text Request | TTT, key=value data segment |
| 0x05 | 0x05 | SCSI Data-Out | TTT, DataSN, Buffer Offset, data |
| 0x06 | 0x06 | Logout Request | Reason code, CID |
| 0x10 | 0x10 | SNACK Request | Type, BegRun, RunLength |

**Target Opcodes (10):**
| Opcode | Hex | Name | Key Fields |
|--------|-----|------|------------|
| 0x20 | 0x20 | NOP-In | ITT, TTT, StatSN |
| 0x21 | 0x21 | SCSI Response | Status, Response, SenseData, ResidualCount |
| 0x22 | 0x22 | Task Management Function Response | Response code |
| 0x23 | 0x23 | Login Response | CSG, NSG, T bit, Status-Class/Detail, ISID, TSIH |
| 0x24 | 0x24 | Text Response | TTT, key=value data segment |
| 0x25 | 0x25 | SCSI Data-In | DataSN, Buffer Offset, ResidualCount, S/F/O/U/A bits |
| 0x26 | 0x26 | Logout Response | Response code, Time2Wait, Time2Retain |
| 0x31 | 0x31 | Ready To Transfer (R2T) | R2TSN, Buffer Offset, Desired Data Transfer Length |
| 0x32 | 0x32 | Asynchronous Message | AsyncEvent, AsyncVCode |
| 0x3f | 0x3f | Reject | Reason, rejected PDU header in data segment |

### BHS Layout (48 bytes)

```
Byte 0:     [I(1)][Opcode(6-7 bits)]
            I = Immediate delivery bit (bit 6)
            Opcode = bits 5-0 (6 bits, bit 5 reserved for some opcodes)
Byte 1:     Final (F) bit (bit 7) + opcode-specific flags (bits 6-0)
Bytes 2-3:  Opcode-specific (reserved in most PDUs)
Byte 4:     TotalAHSLength (in 4-byte words)
Bytes 5-7:  DataSegmentLength (24-bit, network byte order) -- actual data, excludes padding
Bytes 8-15: LUN (8 bytes) or opcode-specific
Bytes 16-19: Initiator Task Tag (ITT) -- present in ALL PDUs
Bytes 20-47: Opcode-specific fields (28 bytes)
             Common fields at known offsets:
             - CmdSN: typically bytes 24-27 (initiator PDUs)
             - ExpStatSN: typically bytes 28-31 (initiator PDUs)
             - StatSN: typically bytes 24-27 (target PDUs)
             - ExpCmdSN: typically bytes 28-31 (target PDUs)
             - MaxCmdSN: typically bytes 32-35 (target PDUs)
```

### PDU Wire Format
```
+--------------------------------------------------+
| Basic Header Segment (BHS)    48 bytes            |
+--------------------------------------------------+
| Additional Header Segments    N * 4 bytes         |  (TotalAHSLength * 4)
+--------------------------------------------------+
| Header Digest (optional)      4 bytes             |  (CRC32C over BHS+AHS)
+--------------------------------------------------+
| Data Segment                  DataSegmentLength   |
+--------------------------------------------------+
| Data Padding                  0-3 bytes (zeros)   |  (pad to 4-byte boundary)
+--------------------------------------------------+
| Data Digest (optional)        4 bytes             |  (CRC32C over data+padding)
+--------------------------------------------------+
```

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| CRC32C computation | Custom CRC implementation | `hash/crc32` with `crc32.Castagnoli` | Hardware-accelerated, verified correct on NetBSD. Polynomial 0x82f63b78. |
| Serial number comparison | Ad-hoc uint32 comparisons | Dedicated `serialCmp` using `int32(s1-s2)` | RFC 1982 mandates modular arithmetic. Plain `<`/`>` breaks at 2^32 wrap. |
| TCP framing | Manual Read loops | `io.ReadFull` | TCP returns arbitrary byte counts. ReadFull guarantees exact-length reads. |
| Buffer pooling | Custom free lists | `sync.Pool` | GC-integrated, goroutine-safe, handles contention. |
| Byte order conversion | Manual bit shifting everywhere | `binary.BigEndian.PutUint32`/`Uint32` etc. | Stdlib, correct, readable. Use for 16/32/64-bit fields. Exception: DataSegmentLength (24-bit) needs manual 3-byte encode. |

## Common Pitfalls

### Pitfall 1: Padding Formula Off-by-One
**What goes wrong:** Using `(4 - (n % 4))` produces padding of 4 when `n` is already 4-byte aligned. Must be `(4 - (n % 4)) % 4`.
**Why it happens:** Easy to forget the outer modulo.
**How to avoid:** Single `padLen` function used everywhere: `func padLen(n uint32) uint32 { return (4 - (n % 4)) % 4 }`
**Warning signs:** Tests with data lengths divisible by 4 produce extra padding bytes.

### Pitfall 2: DataSegmentLength is 24-bit, Not 32-bit
**What goes wrong:** Using `binary.BigEndian.PutUint32` for DataSegmentLength overwrites byte 4 (TotalAHSLength). The field is bytes 5-7 only.
**Why it happens:** Most BHS fields are 32-bit aligned, but DSL is 24-bit sharing a word with TotalAHSLength.
**How to avoid:** Manual 3-byte encode: `bhs[5] = byte(dsLen >> 16); bhs[6] = byte(dsLen >> 8); bhs[7] = byte(dsLen)`. Decode similarly.
**Warning signs:** TotalAHSLength getting corrupted when DataSegmentLength > 0.

### Pitfall 3: Serial Arithmetic Undefined Case
**What goes wrong:** When two serial numbers differ by exactly 2^31, RFC 1982 says the comparison is undefined. The `int32(s1-s2)` trick returns `LT` for this case (since int32 min is negative).
**Why it happens:** The mathematical definition has a gap; implementations must choose behavior.
**How to avoid:** Document that the implementation treats the 2^31-apart case as `s1 < s2`. In practice, iSCSI command windows are never remotely this large, so the undefined case cannot occur in normal operation.
**Warning signs:** None in practice -- only matters for unit test completeness.

### Pitfall 4: Header Digest Scope
**What goes wrong:** Computing CRC32C over only the 48-byte BHS, forgetting AHS. Or including the header digest field itself in the computation.
**Why it happens:** Most PDUs have no AHS, so the bug is invisible until extended CDB support.
**How to avoid:** Digest covers bytes [0, 48 + TotalAHSLength*4). Digest value is placed AFTER the AHS, not included in computation.
**Warning signs:** Digest mismatches when AHS is present.

### Pitfall 5: Data Digest Includes Padding
**What goes wrong:** Computing data digest over only DataSegmentLength bytes, not including padding zeros.
**Why it happens:** RFC 7143 says data digest covers data segment AND padding.
**How to avoid:** Pad data to 4-byte boundary with zeros, then compute CRC32C over padded data.
**Warning signs:** Data digest failures on odd-length data segments.

### Pitfall 6: io.ReadFull vs io.Read for PDU Framing
**What goes wrong:** Using `conn.Read()` for BHS reads. TCP may return 20 bytes of a 48-byte BHS. Next read picks up remaining 28 bytes + start of AHS, corrupting all subsequent framing.
**Why it happens:** Developers assume TCP delivers complete "messages."
**How to avoid:** Always `io.ReadFull(conn, bhs[:])`. This loops internally until all 48 bytes are read or error.
**Warning signs:** Intermittent PDU decode failures, especially under load or with small TCP segments.

### Pitfall 7: Write Pump Interleaving
**What goes wrong:** Two goroutines write PDUs to the same `net.Conn` concurrently. BHS bytes from one PDU interleave with data segment from another.
**Why it happens:** `net.Conn.Write` is NOT goroutine-safe for concurrent calls.
**How to avoid:** Single writer goroutine consuming from a channel. All PDU sends go through the channel.
**Warning signs:** Tests pass without `-race` but fail with it. Corrupt PDUs on wire.

## Code Examples

### Serial Number Arithmetic (RFC 1982)
```go
// Source: RFC 1982, verified on Go 1.25.5/NetBSD
package serial

// LessThan returns true if s1 < s2 using RFC 1982 serial arithmetic.
// For 32-bit serial numbers (SERIAL_BITS=32).
// Note: undefined when s1 and s2 differ by exactly 2^31.
func LessThan(s1, s2 uint32) bool {
    return s1 != s2 && int32(s1-s2) < 0
}

// GreaterThan returns true if s1 > s2 using RFC 1982 serial arithmetic.
func GreaterThan(s1, s2 uint32) bool {
    return s1 != s2 && int32(s1-s2) > 0
}

// InWindow returns true if sn is within [lo, hi] inclusive using serial arithmetic.
// Used for CmdSN window checks: InWindow(cmdSN, expCmdSN, maxCmdSN).
func InWindow(sn, lo, hi uint32) bool {
    return sn == lo || sn == hi || (GreaterThan(sn, lo) && LessThan(sn, hi))
}

// Incr returns s + 1 mod 2^32.
func Incr(s uint32) uint32 { return s + 1 }
```

### CRC32C Digest
```go
// Source: hash/crc32 package docs + RFC 7143 Section 12.1
package digest

import "hash/crc32"

var crc32cTable = crc32.MakeTable(crc32.Castagnoli)

// HeaderDigest computes CRC32C over BHS + AHS bytes.
// The digest value itself is NOT included in the input.
func HeaderDigest(bhsAndAHS []byte) uint32 {
    return crc32.Checksum(bhsAndAHS, crc32cTable)
}

// DataDigest computes CRC32C over data segment + padding.
// Padding to 4-byte boundary with zeros must be included.
func DataDigest(data []byte) uint32 {
    padLen := (4 - (len(data) % 4)) % 4
    if padLen == 0 {
        return crc32.Checksum(data, crc32cTable)
    }
    // Append zero padding for digest computation
    h := crc32.New(crc32cTable)
    h.Write(data)
    h.Write(make([]byte, padLen))
    return h.Sum32()
}
```

### Padding Helper
```go
// PadLen returns number of padding bytes needed to align n to 4-byte boundary.
func PadLen(n uint32) uint32 {
    return (4 - (n % 4)) % 4
}
```

### ITT Router
```go
package transport

import "sync"

type Router struct {
    mu      sync.Mutex
    pending map[uint32]chan<- *RawPDU
    nextITT uint32
}

func NewRouter() *Router {
    return &Router{pending: make(map[uint32]chan<- *RawPDU)}
}

// Register allocates a new ITT and returns it with a response channel.
func (r *Router) Register() (uint32, <-chan *RawPDU) {
    r.mu.Lock()
    defer r.mu.Unlock()

    itt := r.nextITT
    r.nextITT++
    if r.nextITT == 0xFFFFFFFF {
        r.nextITT = 0 // skip reserved value
    }

    ch := make(chan *RawPDU, 1)
    r.pending[itt] = ch
    return itt, ch
}

// Dispatch delivers a response PDU to the waiting command goroutine.
func (r *Router) Dispatch(itt uint32, pdu *RawPDU) bool {
    r.mu.Lock()
    ch, ok := r.pending[itt]
    if ok {
        delete(r.pending, itt)
    }
    r.mu.Unlock()
    if ok {
        ch <- pdu
    }
    return ok
}

// Unregister removes a pending ITT (for timeout/cancel).
func (r *Router) Unregister(itt uint32) {
    r.mu.Lock()
    delete(r.pending, itt)
    r.mu.Unlock()
}
```

## CRC32C Test Vectors

Verified on Go 1.25.5 / NetBSD 10.1:

| Input | Expected CRC32C | Verified |
|-------|-----------------|----------|
| "123456789" (ASCII) | 0xe3069283 | YES |
| Empty (0 bytes) | 0x00000000 | YES |
| 32 bytes of 0x00 | 0x8a9136aa | YES |
| 32 bytes of 0xFF | 0x62a8ab43 | YES |

**Note on byte order:** The CRC32C value is stored in the PDU in the platform's uint32 representation, which on the wire is big-endian per RFC 7143. Use `binary.BigEndian.PutUint32` when writing digest to wire.

## gotgt Integration Test Strategy

**Decision D-10 resolution:** Use gotgt as a subprocess for Phase 1 smoke tests. Rationale:

1. gotgt's in-process API requires creating `SCSITargetService` + `ISCSITargetDriver` + configuring targets/LUNs -- heavyweight setup with undocumented internal API surface
2. Running `gotgt daemon` as a subprocess with a JSON config file is documented and stable
3. For Phase 1, we only need to verify PDU framing works over real TCP -- subprocess is sufficient
4. In-process embedding can be explored in later phases when login/session support enables richer integration tests

**Phase 1 gotgt scope:** Transport framing smoke test only -- connect, send a NOP-Out, expect NOP-In back (or observe framing). Full login requires Phase 2.

**Fallback:** If gotgt proves problematic, write a minimal Go iSCSI responder that reads a BHS and echoes a NOP-In. This is sufficient for framing validation.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `synctest.Run(func())` | `synctest.Test(t, func())` | Go 1.25 (Aug 2025) | Test function takes `*testing.T` parameter. `Run` deprecated, removed in Go 1.26. |
| `GOEXPERIMENT=synctest` | Standard import | Go 1.25 | No build tag or experiment flag needed. Direct `import "testing/synctest"`. |
| Manual CRC32C tables | `crc32.MakeTable(crc32.Castagnoli)` | Go 1.6+ | Hardware acceleration auto-detected on amd64. |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` stdlib (Go 1.25.5) |
| Config file | None needed -- Go test tooling works out of the box |
| Quick run command | `go test -race -count=1 ./internal/...` |
| Full suite command | `go test -race -count=1 -v ./...` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| PDU-01 | Round-trip encode/decode all 24 PDU types | unit | `go test -race ./internal/pdu/ -run TestRoundTrip -v` | Wave 0 |
| PDU-02 | Serial arithmetic with wrap-around | unit | `go test -race ./internal/serial/ -run TestSerial -v` | Wave 0 |
| PDU-03 | CRC32C matches known test vectors | unit | `go test -race ./internal/digest/ -run TestCRC32C -v` | Wave 0 |
| PDU-04 | Padding to 4-byte boundaries | unit | `go test -race ./internal/pdu/ -run TestPadding -v` | Wave 0 |
| XPORT-01 | TCP connection with timeout/cancel | unit | `go test -race ./internal/transport/ -run TestConn -v` | Wave 0 |
| XPORT-02 | PDU framing over TCP (back-to-back, partial) | unit | `go test -race ./internal/transport/ -run TestFraming -v` | Wave 0 |
| XPORT-03 | Concurrent read/write pumps no corruption | unit+race | `go test -race ./internal/transport/ -run TestPump -v` | Wave 0 |
| XPORT-04 | ITT routing/correlation | unit | `go test -race ./internal/transport/ -run TestRouter -v` | Wave 0 |
| TEST-03 | Table-driven tests for all PDU types | unit | `go test -race ./internal/pdu/ -v` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test -race -count=1 ./internal/...`
- **Per wave merge:** `go test -race -count=1 -v ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `go.mod` -- Module initialization (`go mod init`)
- [ ] `internal/serial/serial_test.go` -- RFC 1982 arithmetic tests
- [ ] `internal/digest/crc32c_test.go` -- CRC32C test vector validation
- [ ] `internal/pdu/pdu_test.go` -- Round-trip encode/decode for all 24 types
- [ ] `internal/transport/framer_test.go` -- PDU framing edge cases
- [ ] `internal/transport/pump_test.go` -- Concurrent pump tests under -race
- [ ] `internal/transport/router_test.go` -- ITT routing tests

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Everything | YES | 1.25.5 | -- |
| `hash/crc32` Castagnoli | PDU-03 | YES | stdlib | -- |
| `net.Pipe()` | Unit tests | YES | stdlib | -- |
| `testing/synctest` | Concurrent tests | YES | stdlib (Go 1.25) | Regular goroutine tests |
| `gostor/gotgt` | Smoke tests | TBD (go get at init) | HEAD | Minimal mock responder |
| `go test -race` | XPORT-03 | YES | Go 1.25 | -- |

**Missing dependencies with no fallback:** None

**Missing dependencies with fallback:**
- gotgt availability depends on `go get` success -- fallback is minimal mock iSCSI responder

## Open Questions

1. **gotgt in-process vs subprocess**
   - What we know: gotgt go.mod is compatible (Go 1.25.0). Subprocess (`gotgt daemon`) is documented. In-process API is underdocumented.
   - What's unclear: Exact in-process setup API for embedding in tests.
   - Recommendation: Use subprocess for Phase 1 smoke tests. Revisit in-process for Phase 2+ when we need scripted login exchanges.

2. **CRC32C hardware acceleration on NetBSD**
   - What we know: `hash/crc32` uses SSE4.2 on amd64 in Go. CRC32C produces correct results on NetBSD.
   - What's unclear: Whether the SSE4.2 path is actually taken on NetBSD (vs software fallback).
   - Recommendation: Benchmark with `go test -bench BenchmarkCRC32C` once code exists. Correctness is confirmed; performance optimization is non-blocking.

3. **BHS field layout for each opcode**
   - What we know: General BHS structure (48 bytes), common field positions.
   - What's unclear: Exact byte offsets for each opcode's specific fields (bytes 20-47 vary per opcode).
   - Recommendation: Reference RFC 7143 Section 11.3-11.19 during implementation. Each opcode subsection has an ASCII diagram of its BHS layout.

## Sources

### Primary (HIGH confidence)
- RFC 7143 (iSCSI Protocol Consolidated) -- PDU formats Section 11, BHS Section 11.2, all opcode definitions
- RFC 1982 (Serial Number Arithmetic) -- comparison and addition algorithms for 32-bit serial numbers
- RFC 3720 Appendix B.4 -- CRC32C test vectors reference
- Go `hash/crc32` package docs -- Castagnoli polynomial, MakeTable API
- Go `encoding/binary` package docs -- BigEndian byte order methods
- Go `testing/synctest` package docs -- Test() API (not Run), bubble mechanism
- Verified: CRC32C produces 0xe3069283 for "123456789" on Go 1.25.5/NetBSD 10.1
- Verified: Serial arithmetic using int32 cast works correctly at wrap-around boundaries
- Verified: `synctest.Test` is the correct API (not `synctest.Run` which is deprecated)

### Secondary (MEDIUM confidence)
- gotgt go.mod specifies Go 1.25.0 -- compatible with our Go 1.25.5
- gotgt `ISCSITargetDriver` has `Run(port int)`, `NewTarget()`, `Close()` methods for embedding
- `.planning/research/ARCHITECTURE.md` -- goroutine-per-connection pattern, ITT routing
- `.planning/research/PITFALLS.md` -- framing errors, serial arithmetic, digest scope

### Tertiary (LOW confidence)
- gotgt in-process embedding API surface -- underdocumented, needs experimentation in Phase 2
- CRC32C hardware acceleration status on NetBSD -- correct output verified, SSE4.2 path unconfirmed

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- all stdlib, verified on target platform
- Architecture: HIGH -- patterns from project research validated against RFC
- Pitfalls: HIGH -- sourced from RFC, UNH-IOL docs, verified edge cases on Go/NetBSD
- gotgt integration: MEDIUM -- compatible but in-process API underdocumented

**Research date:** 2026-03-31
**Valid until:** 2026-04-30 (stable domain -- RFC and stdlib unlikely to change)
