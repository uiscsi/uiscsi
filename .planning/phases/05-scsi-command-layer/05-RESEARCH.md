# Phase 5: SCSI Command Layer - Research

**Researched:** 2026-04-01
**Domain:** SCSI CDB construction, response parsing, sense data interpretation
**Confidence:** HIGH

## Summary

Phase 5 builds a standalone `internal/scsi/` package that constructs SCSI Command Descriptor Blocks (CDBs) and parses responses for 19 SCSI commands. This is a pure data-transformation layer: functions take parameters (LBA, block count, flags) and produce `session.Command` structs with correctly packed CDB bytes. Parse functions take `session.Result` and return typed Go structs. No networking, no session state, no concurrency -- just binary encoding/decoding per SPC-4 and SBC-3 specifications.

The domain is well-defined by SCSI standards (T10). Every CDB has a fixed byte layout documented to the bit level. Every response has a known structure. The primary risk is not complexity but volume: 19 commands with varying CDB sizes (6/10/12/16 bytes), multiple response formats (INQUIRY standard vs VPD, sense fixed vs descriptor), and bitfield packing. The mitigation is systematic table-driven testing with golden bytes for every command.

**Primary recommendation:** Implement in command-group waves (core commands first, then extended), with each file covering related commands. Use table-driven tests with hex-literal golden CDB bytes verified against the SCSI specifications. Functional options for optional flags (FUA, DPO, etc.) following the established `session.SessionOption` pattern.

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** New `internal/scsi/` package. Clean separation from iSCSI transport -- scsi/ owns CDB building, response parsing, sense data. Session stays transport-only.
- **D-02:** Files organized by command group: inquiry.go, readwrite.go, capacity.go, sense.go, provisioning.go (WRITE SAME, UNMAP), reservations.go (PERSISTENT RESERVE), vpd.go, etc. ~7-8 source files.
- **D-03:** Plain functions returning `session.Command`. `scsi.Read10(lba, blocks)` returns a Command with CDB filled in. Required parameters are positional args.
- **D-04:** Optional flags via functional options. `scsi.Write10(lba, blocks, data, scsi.WithFUA(), scsi.WithDPO())`. Consistent with Session's existing pattern.
- **D-05:** Parse commonly used fields into typed structs, expose raw bytes for niche fields.
- **D-06:** Sense data as typed struct with SenseKey enum, ASC/ASCQ uint8 pair, and String() method. Both fixed and descriptor formats parsed.
- **D-07:** Standalone functions only -- no methods on Session. scsi package has zero dependency on Session internals.
- **D-08:** Parse functions take `session.Result` directly. `scsi.ParseInquiry(result)` handles status checking and parsing.

### Claude's Discretion
- Exact ASC/ASCQ string lookup table coverage (common vs exhaustive)
- Internal helper patterns for CDB byte packing
- Test fixture organization (golden bytes, table-driven, etc.)
- Whether to export the functional option type or keep it internal

### Deferred Ideas (OUT OF SCOPE)
None
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| SCSI-01 | TEST UNIT READY | Opcode 0x00, 6-byte CDB, no data transfer. Simplest command -- good starting point. |
| SCSI-02 | INQUIRY (standard data) | Opcode 0x12, 6-byte CDB, parse 36+ byte response into device type, vendor, product, revision. |
| SCSI-03 | INQUIRY VPD pages (0x00, 0x80, 0x83) | Same opcode 0x12 with EVPD=1 bit. Page 0x00: supported pages list. 0x80: serial number (ASCII). 0x83: device identification descriptors with association/type/identifier. |
| SCSI-04 | READ CAPACITY (10) and (16) | Opcodes 0x25 (10-byte) and 0x9E/0x10 (16-byte SERVICE ACTION IN). Returns last LBA + block size. RC16 adds protection, provisioning info. |
| SCSI-05 | READ (10) and READ (16) | Opcodes 0x28 (10-byte) and 0x88 (16-byte). LBA + transfer length in CDB. Read direction, ExpectedDataTransferLen = blocks * blockSize. |
| SCSI-06 | WRITE (10) and WRITE (16) | Opcodes 0x2A (10-byte) and 0x8A (16-byte). Same LBA/length layout as READ. Write direction, data from io.Reader. |
| SCSI-07 | REQUEST SENSE | Opcode 0x03, 6-byte CDB, allocation length in byte 4. Returns sense data (reuses sense parsing from SCSI-10). |
| SCSI-08 | REPORT LUNS | Opcode 0xA0, 12-byte CDB, allocation length in bytes 6-9. Response: 4-byte list length + 8-byte LUN entries. |
| SCSI-09 | MODE SENSE (6) and MODE SENSE (10) | Opcodes 0x1A (6-byte) and 0x5A (10-byte). Page code, subpage code, DBD flag. Parse mode parameter header + block descriptor + mode pages. |
| SCSI-10 | Structured sense data parsing | Response codes 0x70/0x71 (fixed) and 0x72/0x73 (descriptor). Sense key in byte 2 bits 3-0. ASC byte 12, ASCQ byte 13 (fixed format). Descriptor format: sense key byte 1, ASC byte 2, ASCQ byte 3. |
| SCSI-11 | SYNCHRONIZE CACHE (10) and (16) | Opcodes 0x35 (10-byte) and 0x91 (16-byte). LBA + number of blocks. Optional IMMED flag. |
| SCSI-12 | WRITE SAME (10) and (16) | Opcodes 0x41 (10-byte) and 0x93 (16-byte). LBA + number of blocks + single block of write data. UNMAP, ANCHOR, NDOB flags. |
| SCSI-13 | UNMAP (thin provisioning / TRIM) | Opcode 0x42, 10-byte CDB. Parameter list in data-out: header (8 bytes) + descriptors (16 bytes each: LBA + block count). |
| SCSI-14 | VERIFY (10) and (16) | Opcodes 0x2F (10-byte) and 0x8F (16-byte). LBA + verification length. BYTCHK flag for compare verification. |
| SCSI-15 | PERSISTENT RESERVE IN | Opcode 0x5E, 10-byte CDB. Service action selects: READ KEYS (0x00), READ RESERVATION (0x01), REPORT CAPABILITIES (0x02), READ FULL STATUS (0x03). Parse response per service action. |
| SCSI-16 | PERSISTENT RESERVE OUT | Opcode 0x5F, 10-byte CDB. Service action: REGISTER (0x00), RESERVE (0x01), RELEASE (0x02), CLEAR (0x03), PREEMPT (0x04), PREEMPT AND ABORT (0x05), REGISTER AND IGNORE (0x06), REGISTER AND MOVE (0x07). 24-byte parameter data. |
| SCSI-17 | COMPARE AND WRITE | Opcode 0x89, 16-byte CDB. LBA + number of blocks. Data contains compare data followed by write data (2x blocks). Atomic operation. |
| SCSI-18 | Extended VPD pages (0xB0, 0xB1, 0xB2) | Same INQUIRY VPD mechanism. 0xB0: block limits (max transfer, optimal sizes, unmap granularity). 0xB1: block characteristics (medium rotation rate, nominal form factor). 0xB2: logical block provisioning (thin provisioning type, UNMAP support). |
| SCSI-19 | START STOP UNIT | Opcode 0x1B, 6-byte CDB. IMMED flag, power condition field, START/LOEJ bits. |
</phase_requirements>

## Project Constraints (from CLAUDE.md)

- **Language:** Go 1.25 -- use modern features where they improve clarity
- **Dependencies:** Minimal external dependencies (stdlib only)
- **Standard:** RFC 7143 compliance, SPC-4/SBC-3 for SCSI commands
- **Testing:** Table-driven tests with stdlib `testing`, no testify
- **API style:** Go idiomatic -- functional options, structured errors
- **Serialization:** `encoding/binary.BigEndian` for all byte packing
- **No dead code, no speculative abstractions**

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `encoding/binary` | stdlib (Go 1.25) | CDB byte packing, response field extraction | BigEndian.PutUint16/32/64 maps directly to SCSI big-endian CDB fields. Already used throughout codebase. |
| `io` | stdlib | io.Reader for write command data | Matches session.Command.Data field type. |
| `fmt` | stdlib | String() methods, error formatting | Sense data human-readable output. |
| `errors` | stdlib | Error wrapping | Parse functions return wrapped errors for status/sense failures. |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `bytes` | stdlib | bytes.NewReader for test data, buffer building for UNMAP/PR OUT parameter lists | Test fixtures and parameter data construction. |
| `strings` | stdlib | Trimming INQUIRY vendor/product ASCII fields | INQUIRY response strings are space-padded to fixed widths. |

No external dependencies needed. This package is pure data transformation.

## Architecture Patterns

### Recommended Project Structure
```
internal/scsi/
  opcode.go          # Opcode constants, shared types (SenseKey enum, Option type)
  sense.go           # Sense data parsing (fixed + descriptor), SenseKey, ASC/ASCQ table
  inquiry.go         # INQUIRY standard + VPD dispatch
  vpd.go             # VPD page parsers (0x00, 0x80, 0x83, 0xB0, 0xB1, 0xB2)
  capacity.go        # READ CAPACITY 10/16
  readwrite.go       # READ 10/16, WRITE 10/16
  modesense.go       # MODE SENSE 6/10
  provisioning.go    # WRITE SAME 10/16, UNMAP, SYNCHRONIZE CACHE 10/16
  reservations.go    # PERSISTENT RESERVE IN/OUT
  commands.go        # TEST UNIT READY, REQUEST SENSE, REPORT LUNS, VERIFY 10/16,
                     # COMPARE AND WRITE, START STOP UNIT
  # Tests mirror source files: sense_test.go, inquiry_test.go, etc.
```

### Pattern 1: CDB Builder Function
**What:** Each SCSI command has a public function that returns `session.Command` with a correctly packed CDB.
**When to use:** Every command builder follows this pattern.
**Example:**
```go
// Read10 builds a SCSI READ(10) command.
// lba is the starting logical block address. blocks is the number of
// blocks to read. blockSize is the logical block size in bytes (needed
// for ExpectedDataTransferLen).
func Read10(lba uint32, blocks uint16, blockSize uint32, opts ...Option) session.Command {
    var cfg options
    for _, o := range opts {
        o(&cfg)
    }
    var cdb [16]byte
    cdb[0] = OpRead10 // 0x28
    // byte 1: RDPROTECT(7-5), DPO(4), FUA(3), RARC(2), FUA_NV(1)
    if cfg.fua {
        cdb[1] |= 0x08
    }
    if cfg.dpo {
        cdb[1] |= 0x10
    }
    binary.BigEndian.PutUint32(cdb[2:6], lba)
    // byte 6: group number (leave 0)
    binary.BigEndian.PutUint16(cdb[7:9], blocks)
    // byte 9: control (leave 0)
    return session.Command{
        CDB:                    cdb,
        Read:                   true,
        ExpectedDataTransferLen: uint32(blocks) * blockSize,
    }
}
```

### Pattern 2: Response Parser Function
**What:** Each parse function takes `session.Result`, checks status, and returns a typed struct.
**When to use:** Every command that returns data.
**Example:**
```go
// ReadCapacity10Response holds the parsed READ CAPACITY(10) response.
type ReadCapacity10Response struct {
    LastLBA   uint32 // Returned Logical Block Address (last addressable)
    BlockSize uint32 // Block Length In Bytes
}

// ParseReadCapacity10 parses a READ CAPACITY(10) response from result.
func ParseReadCapacity10(result session.Result) (*ReadCapacity10Response, error) {
    if result.Err != nil {
        return nil, fmt.Errorf("scsi: read capacity(10): %w", result.Err)
    }
    if result.Status != StatusGood {
        sense, _ := ParseSense(result.SenseData)
        return nil, &CommandError{Status: result.Status, Sense: sense}
    }
    data, err := io.ReadAll(result.Data)
    if err != nil {
        return nil, fmt.Errorf("scsi: read capacity(10) read data: %w", err)
    }
    if len(data) < 8 {
        return nil, fmt.Errorf("scsi: read capacity(10): response too short: %d bytes", len(data))
    }
    return &ReadCapacity10Response{
        LastLBA:   binary.BigEndian.Uint32(data[0:4]),
        BlockSize: binary.BigEndian.Uint32(data[4:8]),
    }, nil
}
```

### Pattern 3: Functional Options for Command Flags
**What:** Optional CDB flags (FUA, DPO, IMMED, etc.) via functional options.
**When to use:** Commands with optional boolean/enum flags beyond the required positional args.
**Example:**
```go
// Option configures optional CDB flags for SCSI commands.
type Option func(*options)

// options holds optional flags. Unexported to enforce construction via
// Option functions. Zero value means all flags disabled.
type options struct {
    fua    bool
    dpo    bool
    immed  bool
    anchor bool
    unmap  bool
    ndob   bool
}

// WithFUA sets the Force Unit Access flag. Data is written to/read from
// non-volatile storage, bypassing any cache.
func WithFUA() Option {
    return func(o *options) { o.fua = true }
}
```

**Recommendation on export:** Keep `Option` type exported (callers need it in signatures), keep `options` struct unexported. This matches the session package pattern where `SessionOption` is exported but `sessionConfig` is not.

### Pattern 4: Sense Data Error Type
**What:** A `CommandError` type that wraps SCSI status + parsed sense data.
**When to use:** Parse functions return this when status != GOOD.
**Example:**
```go
// CommandError represents a SCSI command that completed but returned
// a non-GOOD status (Check Condition, Busy, etc.).
type CommandError struct {
    Status uint8
    Sense  *SenseData // nil if no sense data available
}

func (e *CommandError) Error() string {
    if e.Sense != nil {
        return fmt.Sprintf("scsi: status 0x%02x: %s", e.Status, e.Sense)
    }
    return fmt.Sprintf("scsi: status 0x%02x", e.Status)
}

// IsSenseKey reports whether the error has the given sense key.
func IsSenseKey(err error, key SenseKey) bool {
    var ce *CommandError
    if errors.As(err, &ce) && ce.Sense != nil {
        return ce.Sense.Key == key
    }
    return false
}
```

### Anti-Patterns to Avoid
- **Giant switch in one file:** Don't put all 19 commands in a single file. Group by function (read/write, inquiry/vpd, provisioning, reservations).
- **Encoding CDB with struct marshal:** Don't use `binary.Write` with struct -- CDBs have bitfields that don't map to Go struct fields. Manual byte packing with `BigEndian.PutUintXX` is correct.
- **Returning raw bytes from parsers:** Parse into typed structs with named fields. Expose `.Raw []byte` for niche sub-fields, not as primary API.
- **Importing session internals:** The scsi package must depend only on `session.Command` and `session.Result` types. Zero knowledge of session state, connection, or PDUs.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| CDB byte packing | Custom bit manipulation helpers | `encoding/binary.BigEndian.PutUint16/32/64` + bitwise OR for flags | stdlib handles endianness; flags are simple bit-or on specific bytes |
| ASC/ASCQ descriptions | Generate from full T10 table at build time | Static map of ~40-60 most common codes | Full T10 table is 800+ entries. Most are irrelevant for block storage. Cover the common ones, return hex string for unknown. |
| SCSI status codes | Custom enum | Simple constants: `StatusGood = 0x00`, `StatusCheckCondition = 0x02`, `StatusBusy = 0x08`, etc. | Only ~8 defined status values. |

**Key insight:** SCSI CDB construction is fundamentally byte-level. The stdlib `encoding/binary` package plus bitwise operations is the exact right tool. No abstraction layer needed -- each CDB is just setting specific bytes/bits in a [16]byte array.

## Common Pitfalls

### Pitfall 1: READ CAPACITY(16) Is a Service Action
**What goes wrong:** Treating RC16 as opcode 0x9E. It is actually SERVICE ACTION IN (opcode 0x9E) with service action 0x10 in CDB byte 1.
**Why it happens:** RC10 has its own opcode (0x25), so developers assume RC16 does too.
**How to avoid:** CDB byte 0 = 0x9E, byte 1 = 0x10 (service action). Allocation length in bytes 10-13.
**Warning signs:** Target returns ILLEGAL REQUEST for READ CAPACITY(16).

### Pitfall 2: INQUIRY Allocation Length vs Actual Response Length
**What goes wrong:** Assuming response exactly fills the allocation length. INQUIRY returns ADDITIONAL LENGTH in byte 4 indicating actual data size, which may be shorter.
**Why it happens:** Allocation length is a maximum, not an exact size.
**How to avoid:** Parse ADDITIONAL LENGTH field and only read that many bytes beyond the fixed header (5 bytes).
**Warning signs:** Parsing garbage bytes past the actual response data.

### Pitfall 3: Fixed vs Descriptor Sense Format Detection
**What goes wrong:** Assuming all sense data is fixed format (0x70/0x71). Modern targets often return descriptor format (0x72/0x73).
**Why it happens:** Fixed format is more common in documentation/examples.
**How to avoid:** Check response code byte 0: 0x70/0x71 = fixed, 0x72/0x73 = descriptor. Field offsets differ completely between formats.
**Warning signs:** Sense key, ASC, ASCQ values are nonsensical.

### Pitfall 4: Sense Data in Descriptor Format Has Different Field Offsets
**What goes wrong:** Using fixed-format offsets (sense key at byte 2 bits 3-0, ASC at byte 12, ASCQ at byte 13) for descriptor format.
**Why it happens:** Copy-paste from fixed format code.
**How to avoid:** Descriptor format: sense key at byte 1 bits 3-0, ASC at byte 2, ASCQ at byte 3. The header is only 8 bytes; descriptors follow.
**Warning signs:** Wrong sense key parsed.

### Pitfall 5: VPD Page 0x83 Descriptor Length Parsing
**What goes wrong:** Incorrectly parsing multi-descriptor VPD 0x83 responses. Each descriptor has a variable-length identifier.
**Why it happens:** Walking the descriptor list requires reading each descriptor's length field to find the next.
**How to avoid:** Each identification descriptor: 4-byte header (code set, protocol, type, association, length) + variable identifier. Walk by reading length field at byte 3 of each descriptor.
**Warning signs:** Second/third descriptors parsed incorrectly, off-by-one in descriptor boundary.

### Pitfall 6: UNMAP Parameter Data Construction
**What goes wrong:** Sending wrong parameter list format. UNMAP has a specific header + descriptor list structure in the data-out buffer.
**Why it happens:** Unlike most commands where CDB alone suffices, UNMAP requires parameter data.
**How to avoid:** Parameter data: 8-byte header (data length, block descriptor data length) + 16-byte descriptors (8-byte LBA + 4-byte block count + 4 reserved). Total data length in CDB bytes 7-8.
**Warning signs:** Target rejects UNMAP with ILLEGAL REQUEST.

### Pitfall 7: PERSISTENT RESERVE OUT Parameter Data
**What goes wrong:** Forgetting the 24-byte parameter data for PR OUT commands.
**Why it happens:** Most commands only need CDB fields.
**How to avoid:** PR OUT always sends a 24-byte parameter list: reservation key (8 bytes), service action reservation key (8 bytes), scope-specific address, type, etc.
**Warning signs:** Target returns ILLEGAL REQUEST or PARAMETER LIST LENGTH ERROR.

### Pitfall 8: COMPARE AND WRITE Data Length
**What goes wrong:** Setting wrong transfer length. CAW data = compare data (N blocks) + write data (N blocks) = 2N blocks total.
**Why it happens:** The CDB's "number of logical blocks" field refers to N, but data transfer is 2N.
**How to avoid:** ExpectedDataTransferLen = 2 * numberOfBlocks * blockSize. CDB field = N.
**Warning signs:** Short data transfer or target rejection.

### Pitfall 9: INQUIRY Vendor/Product String Padding
**What goes wrong:** Not trimming trailing spaces from INQUIRY vendor (8 bytes), product (16 bytes), revision (4 bytes) fields.
**Why it happens:** Fields are fixed-width, right-padded with ASCII spaces (0x20).
**How to avoid:** `strings.TrimRight(string(data[8:16]), " ")` for vendor, etc.
**Warning signs:** Strings compare incorrectly due to trailing spaces.

### Pitfall 10: LUN Field in session.Command
**What goes wrong:** Forgetting to set LUN field on commands targeting specific logical units.
**Why it happens:** Many test scenarios use LUN 0 where the default zero value works.
**How to avoid:** All CDB builder functions should accept a LUN parameter or option. REPORT LUNS and TEST UNIT READY may default to LUN 0, but READ/WRITE/etc. need explicit LUN.
**Warning signs:** Commands go to wrong LUN or target returns errors.

## Code Examples

### SCSI Opcode Constants
```go
// Opcode constants for SCSI commands per SPC-4 and SBC-3.
const (
    OpTestUnitReady      = 0x00
    OpRequestSense       = 0x03
    OpInquiry            = 0x12
    OpModeSense6         = 0x1A
    OpStartStopUnit      = 0x1B
    OpReadCapacity10     = 0x25
    OpRead10             = 0x28
    OpWrite10            = 0x2A
    OpVerify10           = 0x2F
    OpSynchronizeCache10 = 0x35
    OpWriteSame10        = 0x41
    OpUnmap              = 0x42
    OpModeSense10        = 0x5A
    OpPersistReserveIn   = 0x5E
    OpPersistReserveOut  = 0x5F
    OpRead16             = 0x88
    OpCompareAndWrite    = 0x89
    OpWrite16            = 0x8A
    OpVerify16           = 0x8F
    OpSynchronizeCache16 = 0x91
    OpWriteSame16        = 0x93
    OpServiceActionIn16  = 0x9E // Used by READ CAPACITY(16) with SA=0x10
    OpReportLuns         = 0xA0
)
```

### Sense Key Enum
```go
// SenseKey represents a SCSI sense key (4-bit value, SPC-4 Table 28).
type SenseKey uint8

const (
    SenseNoSense        SenseKey = 0x00
    SenseRecoveredError SenseKey = 0x01
    SenseNotReady       SenseKey = 0x02
    SenseMediumError    SenseKey = 0x03
    SenseHardwareError  SenseKey = 0x04
    SenseIllegalRequest SenseKey = 0x05
    SenseUnitAttention  SenseKey = 0x06
    SenseDataProtect    SenseKey = 0x07
    SenseBlankCheck     SenseKey = 0x08
    SenseVendorSpecific SenseKey = 0x09
    SenseCopyAborted    SenseKey = 0x0A
    SenseAbortedCommand SenseKey = 0x0B
    SenseVolumeOverflow SenseKey = 0x0D
    SenseMiscompare     SenseKey = 0x0E
)

func (sk SenseKey) String() string {
    // Map to human-readable names
}
```

### Fixed Format Sense Data Parsing
```go
// SenseData holds parsed SCSI sense data.
type SenseData struct {
    ResponseCode uint8    // 0x70/0x71 (fixed) or 0x72/0x73 (descriptor)
    Key          SenseKey // Sense key
    ASC          uint8    // Additional Sense Code
    ASCQ         uint8    // Additional Sense Code Qualifier
    Information  uint32   // Valid only for fixed format with Valid bit
    Valid        bool     // Information field is valid
    Raw          []byte   // Original sense bytes
}

// ParseSense parses raw sense data bytes into a SenseData struct.
// Handles both fixed format (0x70/0x71) and descriptor format (0x72/0x73).
func ParseSense(data []byte) (*SenseData, error) {
    if len(data) < 2 {
        return nil, fmt.Errorf("scsi: sense data too short: %d bytes", len(data))
    }
    sd := &SenseData{
        ResponseCode: data[0] & 0x7F,
        Raw:          append([]byte(nil), data...), // defensive copy
    }
    switch sd.ResponseCode {
    case 0x70, 0x71: // Fixed format
        if len(data) < 18 {
            // Minimum fixed format is 18 bytes per SPC-4
            return nil, fmt.Errorf("scsi: fixed sense too short: %d bytes", len(data))
        }
        sd.Valid = data[0]&0x80 != 0
        sd.Key = SenseKey(data[2] & 0x0F)
        sd.Information = binary.BigEndian.Uint32(data[3:7])
        sd.ASC = data[12]
        sd.ASCQ = data[13]
    case 0x72, 0x73: // Descriptor format
        if len(data) < 8 {
            return nil, fmt.Errorf("scsi: descriptor sense too short: %d bytes", len(data))
        }
        sd.Key = SenseKey(data[1] & 0x0F)
        sd.ASC = data[2]
        sd.ASCQ = data[3]
    default:
        return nil, fmt.Errorf("scsi: unknown sense response code: 0x%02x", sd.ResponseCode)
    }
    return sd, nil
}
```

### SCSI Status Constants
```go
// SCSI status codes per SAM-5.
const (
    StatusGood                 = 0x00
    StatusCheckCondition       = 0x02
    StatusConditionMet         = 0x04
    StatusBusy                 = 0x08
    StatusReservationConflict  = 0x18
    StatusTaskSetFull          = 0x28
    StatusACAActive            = 0x30
    StatusTaskAborted          = 0x40
)
```

### Test Pattern: Golden Byte CDB Verification
```go
func TestRead10CDB(t *testing.T) {
    tests := []struct {
        name      string
        lba       uint32
        blocks    uint16
        blockSize uint32
        opts      []Option
        wantCDB   [16]byte
        wantRead  bool
        wantLen   uint32
    }{
        {
            name:      "simple read at LBA 0",
            lba:       0,
            blocks:    1,
            blockSize: 512,
            wantCDB:   [16]byte{0x28, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00},
            wantRead:  true,
            wantLen:   512,
        },
        {
            name:      "read with FUA at high LBA",
            lba:       0x01000000,
            blocks:    256,
            blockSize: 4096,
            opts:      []Option{WithFUA()},
            wantCDB:   [16]byte{0x28, 0x08, 0x01, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00},
            wantRead:  true,
            wantLen:   256 * 4096,
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            cmd := Read10(tt.lba, tt.blocks, tt.blockSize, tt.opts...)
            if cmd.CDB != tt.wantCDB {
                t.Errorf("CDB mismatch:\n got: %x\nwant: %x", cmd.CDB, tt.wantCDB)
            }
            if cmd.Read != tt.wantRead {
                t.Errorf("Read = %v, want %v", cmd.Read, tt.wantRead)
            }
            if cmd.ExpectedDataTransferLen != tt.wantLen {
                t.Errorf("ExpectedDataTransferLen = %d, want %d", cmd.ExpectedDataTransferLen, tt.wantLen)
            }
        })
    }
}
```

## SCSI CDB Byte Layouts Reference

Verified against T10 operation code assignments and libiscsi header definitions (HIGH confidence).

### 6-Byte CDBs
| Command | Op | Byte 1 | Byte 2 | Byte 3 | Byte 4 | Byte 5 |
|---------|-----|--------|--------|--------|--------|--------|
| TEST UNIT READY | 0x00 | 0 | 0 | 0 | 0 | Control |
| REQUEST SENSE | 0x03 | DESC(0) | 0 | 0 | AllocLen | Control |
| INQUIRY | 0x12 | EVPD(0) | PageCode | AllocLen(hi) | AllocLen(lo) | Control |
| MODE SENSE(6) | 0x1A | DBD(3) | PC(7-6)+PageCode(5-0) | SubpageCode | AllocLen | Control |
| START STOP UNIT | 0x1B | IMMED(0) | 0 | PwrCond(7-4) | 0 | START(0)+LOEJ(1) Control |

### 10-Byte CDBs
| Command | Op | Key Fields |
|---------|-----|------------|
| READ CAPACITY(10) | 0x25 | Bytes 2-5: LBA (obsolete, usually 0). No allocation length -- fixed 8-byte response. |
| READ(10) | 0x28 | Byte 1: DPO(4), FUA(3). Bytes 2-5: LBA. Bytes 7-8: Transfer Length. |
| WRITE(10) | 0x2A | Byte 1: DPO(4), FUA(3). Bytes 2-5: LBA. Bytes 7-8: Transfer Length. |
| VERIFY(10) | 0x2F | Byte 1: BYTCHK(2-1). Bytes 2-5: LBA. Bytes 7-8: Verification Length. |
| SYNCHRONIZE CACHE(10) | 0x35 | Byte 1: IMMED(1). Bytes 2-5: LBA. Bytes 7-8: Number of Blocks. |
| WRITE SAME(10) | 0x41 | Byte 1: UNMAP(3), ANCHOR(2), NDOB(0). Bytes 2-5: LBA. Bytes 7-8: Num Blocks. |
| UNMAP | 0x42 | Byte 1: ANCHOR(0). Bytes 7-8: Parameter List Length. |
| MODE SENSE(10) | 0x5A | Byte 1: DBD(3). Byte 2: PC(7-6)+PageCode(5-0). Byte 3: SubpageCode. Bytes 7-8: AllocLen. |
| PERSIST RESERVE IN | 0x5E | Byte 1: ServiceAction(4-0). Bytes 7-8: AllocLen. |
| PERSIST RESERVE OUT | 0x5F | Byte 1: ServiceAction(4-0). Byte 2: Type(7-4)+Scope(3-0). Bytes 5-8: ParamListLen. |

### 12-Byte CDBs
| Command | Op | Key Fields |
|---------|-----|------------|
| REPORT LUNS | 0xA0 | Byte 2: Select Report. Bytes 6-9: AllocLen. |

### 16-Byte CDBs
| Command | Op | Key Fields |
|---------|-----|------------|
| READ(16) | 0x88 | Byte 1: DPO(4), FUA(3). Bytes 2-9: LBA. Bytes 10-13: Transfer Length. |
| WRITE(16) | 0x8A | Byte 1: DPO(4), FUA(3). Bytes 2-9: LBA. Bytes 10-13: Transfer Length. |
| COMPARE AND WRITE | 0x89 | Bytes 2-9: LBA. Byte 13: Number of Blocks. Data = 2x blocks. |
| VERIFY(16) | 0x8F | Byte 1: BYTCHK(2-1). Bytes 2-9: LBA. Bytes 10-13: Verification Length. |
| SYNCHRONIZE CACHE(16) | 0x91 | Byte 1: IMMED(1). Bytes 2-9: LBA. Bytes 10-13: Number of Blocks. |
| WRITE SAME(16) | 0x93 | Byte 1: UNMAP(3), ANCHOR(2), NDOB(0). Bytes 2-9: LBA. Bytes 10-13: Num Blocks. |
| READ CAPACITY(16) | 0x9E | Byte 1: SA=0x10. Bytes 10-13: AllocLen. Response: 32 bytes. |

## ASC/ASCQ Lookup Table Recommendation

**Recommendation:** Include ~50 most common codes as a static map. Return "UNKNOWN (0xNN/0xNN)" for unrecognized codes. This covers the vast majority of real-world block storage scenarios without bloating the binary with 800+ entries.

Common codes to include (sourced from T10 ASC/ASCQ numerical listing):

| Category | ASC/ASCQ Range | Example Codes |
|----------|---------------|---------------|
| Not Ready | 0x04/0x00-0x1F | Becoming ready, manual intervention, operation in progress |
| Communication | 0x08/0x00-0x02 | Communication failure, timeout, parity error |
| Read Errors | 0x11/0x00-0x01 | Unrecovered read error, retries exhausted |
| Write Protected | 0x27/0x00 | Write protected |
| Medium Changed | 0x28/0x00 | Not ready to ready change, medium may have changed |
| Reset | 0x29/0x00 | Power on, reset, or bus device reset occurred |
| Parameters Changed | 0x2A/0x00-0x06 | Mode parameters changed |
| Commands Cleared | 0x2F/0x00 | Commands cleared by another initiator |
| Medium Not Present | 0x3A/0x00 | Medium not present |
| Internal Failure | 0x44/0x00 | Internal target failure |
| Invalid Opcode | 0x20/0x00 | Invalid command operation code |
| Invalid Field | 0x24/0x00 | Invalid field in CDB |
| Invalid Param List | 0x26/0x00 | Invalid field in parameter list |

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Fixed-format sense only | Both fixed (0x70) and descriptor (0x72) formats | SPC-3 (2005+) | Must handle both; descriptor is increasingly common with modern targets |
| READ CAPACITY(10) only | RC10 + RC16 for >2TB devices | SBC-2 (2004+) | RC10 maxes at 2TB. RC16 mandatory for large LUNs. |
| No thin provisioning | UNMAP, WRITE SAME with UNMAP, VPD 0xB0/0xB2 | SBC-3 (2011+) | Block limits and provisioning VPDs needed for thin provisioning awareness |

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib testing (Go 1.25) |
| Config file | none -- Go convention |
| Quick run command | `go test ./internal/scsi/ -count=1` |
| Full suite command | `go test ./... -count=1 -race` |

### Phase Requirements to Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| SCSI-01 | TEST UNIT READY CDB | unit | `go test ./internal/scsi/ -run TestTestUnitReady -count=1` | Wave 0 |
| SCSI-02 | INQUIRY standard CDB + parse | unit | `go test ./internal/scsi/ -run TestInquiry -count=1` | Wave 0 |
| SCSI-03 | INQUIRY VPD CDB + parse 0x00/0x80/0x83 | unit | `go test ./internal/scsi/ -run TestVPD -count=1` | Wave 0 |
| SCSI-04 | READ CAPACITY 10/16 CDB + parse | unit | `go test ./internal/scsi/ -run TestReadCapacity -count=1` | Wave 0 |
| SCSI-05 | READ 10/16 CDB | unit | `go test ./internal/scsi/ -run TestRead -count=1` | Wave 0 |
| SCSI-06 | WRITE 10/16 CDB | unit | `go test ./internal/scsi/ -run TestWrite -count=1` | Wave 0 |
| SCSI-07 | REQUEST SENSE CDB | unit | `go test ./internal/scsi/ -run TestRequestSense -count=1` | Wave 0 |
| SCSI-08 | REPORT LUNS CDB + parse | unit | `go test ./internal/scsi/ -run TestReportLuns -count=1` | Wave 0 |
| SCSI-09 | MODE SENSE 6/10 CDB + parse | unit | `go test ./internal/scsi/ -run TestModeSense -count=1` | Wave 0 |
| SCSI-10 | Sense data fixed + descriptor parse | unit | `go test ./internal/scsi/ -run TestParseSense -count=1` | Wave 0 |
| SCSI-11 | SYNCHRONIZE CACHE 10/16 CDB | unit | `go test ./internal/scsi/ -run TestSyncCache -count=1` | Wave 0 |
| SCSI-12 | WRITE SAME 10/16 CDB | unit | `go test ./internal/scsi/ -run TestWriteSame -count=1` | Wave 0 |
| SCSI-13 | UNMAP CDB + parameter data | unit | `go test ./internal/scsi/ -run TestUnmap -count=1` | Wave 0 |
| SCSI-14 | VERIFY 10/16 CDB | unit | `go test ./internal/scsi/ -run TestVerify -count=1` | Wave 0 |
| SCSI-15 | PERSISTENT RESERVE IN CDB + parse | unit | `go test ./internal/scsi/ -run TestPersistReserveIn -count=1` | Wave 0 |
| SCSI-16 | PERSISTENT RESERVE OUT CDB + param data | unit | `go test ./internal/scsi/ -run TestPersistReserveOut -count=1` | Wave 0 |
| SCSI-17 | COMPARE AND WRITE CDB | unit | `go test ./internal/scsi/ -run TestCompareAndWrite -count=1` | Wave 0 |
| SCSI-18 | Extended VPD 0xB0/0xB1/0xB2 parse | unit | `go test ./internal/scsi/ -run TestExtendedVPD -count=1` | Wave 0 |
| SCSI-19 | START STOP UNIT CDB | unit | `go test ./internal/scsi/ -run TestStartStopUnit -count=1` | Wave 0 |

### Sampling Rate
- **Per task commit:** `go test ./internal/scsi/ -count=1`
- **Per wave merge:** `go test ./... -count=1 -race`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] `internal/scsi/` directory -- does not exist yet
- [ ] All test files -- none exist yet (this is a greenfield package)
- [ ] No framework install needed -- stdlib testing

## Open Questions

1. **LUN parameter placement**
   - What we know: All commands except REPORT LUNS target a specific LUN. session.Command has a LUN field.
   - What's unclear: Should LUN be a required positional parameter on every CDB builder, or set via an option?
   - Recommendation: Make LUN a positional parameter on commands that always target a LUN (READ, WRITE, etc.). Commands that may operate without a LUN (TEST UNIT READY, REPORT LUNS) default to LUN 0. This matches the CDB builder signature style from D-03.

2. **MODE SENSE page parsing depth**
   - What we know: MODE SENSE returns a header + optional block descriptors + mode pages. Mode pages are numerous and complex.
   - What's unclear: How deep to parse mode pages -- just return raw page bytes, or parse common pages (caching, control)?
   - Recommendation: Parse the mode parameter header and block descriptor. Return mode page data as raw bytes with page code/length metadata. Mode page parsing is a potential future extension. This aligns with D-05 (parse common fields, expose raw for niche).

3. **blockSize parameter on READ/WRITE**
   - What we know: CDB builders need blockSize to calculate ExpectedDataTransferLen.
   - What's unclear: Should blockSize be a positional param or derived from a prior ReadCapacity call?
   - Recommendation: Positional param. The scsi package is stateless -- it doesn't know block size. Caller gets it from ReadCapacity and passes it in. Clean separation.

## Sources

### Primary (HIGH confidence)
- [T10 Operation Code Assignments](https://www.t10.org/lists/op-num.htm) -- all SCSI opcodes and CDB sizes verified
- [RFC 7143](https://datatracker.ietf.org/doc/html/rfc7143) -- SCSI Command/Response PDU format, sense data delivery
- [libiscsi scsi-lowlevel.h](https://github.com/sahlberg/libiscsi/blob/master/include/scsi-lowlevel.h) -- CDB structures, sense data parsing, response structs
- [gotgt spc.go](https://github.com/gostor/gotgt/blob/master/pkg/scsi/spc.go) -- Go CDB parsing patterns for INQUIRY, MODE SENSE, REPORT LUNS

### Secondary (MEDIUM confidence)
- [SCSI Sense Data - Wikistix](https://www.stix.id.au/wiki/SCSI_Sense_Data) -- Fixed format sense byte layout, sense key definitions
- [T10 ASC/ASCQ Numerical Listing](https://www.t10.org/lists/asc-num.txt) -- Additional sense code descriptions
- [Wikipedia Key Code Qualifier](https://en.wikipedia.org/wiki/Key_Code_Qualifier) -- Sense key overview

### Tertiary (LOW confidence)
- None -- all findings verified against primary specifications or reference implementations

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- stdlib only, same pattern as rest of codebase
- Architecture: HIGH -- decisions locked in CONTEXT.md, patterns follow existing codebase
- CDB layouts: HIGH -- verified against T10 opcode table and libiscsi header
- Sense data: HIGH -- fixed format well documented; descriptor format verified against multiple sources
- Pitfalls: HIGH -- derived from specification knowledge and reference implementation study

**Research date:** 2026-04-01
**Valid until:** 2026-05-01 (SCSI specifications are extremely stable)
