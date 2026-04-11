# uiscsi

A pure-userspace iSCSI initiator library for Go.

**Status:** Full RFC 7143 compliance with 87 wire-level conformance tests and 21 E2E tests against real LIO kernel targets. Grouped Session API. Bounded-memory streaming I/O. Deterministic session shutdown. Configurable performance tuning. Observability hooks for metrics, state changes, and wire-level inspection.

## Overview

uiscsi implements the iSCSI protocol (RFC 7143) entirely in userspace. There are no kernel modules, no open-iscsi dependency, and no external tools. Go applications import the library and communicate directly with iSCSI targets over TCP.

The library provides a grouped API organized by concern:
- **`sess.SCSI()`** -- typed SCSI commands (ReadBlocks, Inquiry, ModeSelect, etc.)
- **`sess.Raw()`** -- raw CDB pass-through with bounded-memory streaming
- **`sess.TMF()`** -- task management (AbortTask, LUNReset, etc.)
- **`sess.Protocol()`** -- low-level iSCSI protocol operations

It supports CHAP and mutual CHAP authentication, header and data digest negotiation with CRC32C, and error recovery levels 0 through 2.

## Features

- **Pure userspace** -- no kernel iSCSI stack, no iscsiadm, no external tools
- **RFC 7143 compliant** -- PDU codec, login negotiation, full feature phase
- **Grouped API** -- organized by concern (SCSI, TMF, Raw, Protocol) for discoverability
- **Go-idiomatic** -- context.Context, io.Reader/io.Writer, functional options
- **Authentication** -- CHAP and mutual CHAP
- **Block I/O** -- ReadBlocks/WriteBlocks via `sess.SCSI()`
- **Raw CDB pass-through** -- Execute (buffered) and StreamExecute (bounded-memory streaming) via `sess.Raw()`
- **Streaming I/O** -- StreamExecute streams Data-In PDUs via bounded-memory channel, suitable for tape drives at 400+ MB/s
- **Tunable PDU size** -- `WithMaxRecvDataSegmentLength` for high-throughput workloads (default 8KB, recommended 256KB for tape)
- **Mode pages** -- ModeSense6/10 and ModeSelect6/10 for device configuration
- **Error recovery** -- ERL 0 (session reconnect), ERL 1 (SNACK), ERL 2 (connection replace)
- **Task management** -- ABORT TASK, LUN RESET, TARGET WARM/COLD RESET via `sess.TMF()`
- **Deterministic shutdown** -- `Close()` waits for all pump goroutines via WaitGroup; no leaked goroutines after session teardown
- **Observability** -- `WithMetricsHook` (per-command latency/bytes), `WithStateChangeHook` (session lifecycle), `WithPDUHook` (wire-level inspection), `WithLogger` for slog integration
- **Security hazard notices** -- godoc warns on dangerous entry points (raw CDB pass-through, FORMAT UNIT access)
- **Complete godoc** -- full coverage with runnable examples
- **Digests** -- CRC32C header and data digest negotiation and verification
- **Discovery** -- SendTargets enumeration, multi-portal support
- **CLI tool** -- `uiscsi-ls` for lsscsi-style target discovery from the command line

## Quick Start

```
go get github.com/uiscsi/uiscsi
```

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/uiscsi/uiscsi"
)

func main() {
    ctx := context.Background()

    sess, err := uiscsi.Dial(ctx, "192.168.1.100:3260",
        uiscsi.WithTarget("iqn.2026-03.com.example:storage"),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer sess.Close()

    // Typed SCSI commands via sess.SCSI()
    data, err := sess.SCSI().ReadBlocks(ctx, 0, 0, 1, 512)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Read %d bytes from LBA 0\n", len(data))
}
```

## API Reference

Full documentation is available on [pkg.go.dev](https://pkg.go.dev/github.com/uiscsi/uiscsi).

### Session Accessors

| Accessor | Returns | Purpose |
|----------|---------|---------|
| `sess.SCSI()` | `*SCSIOps` | Typed SCSI commands with automatic status/sense handling |
| `sess.Raw()` | `*RawOps` | Raw CDB pass-through (caller interprets status) |
| `sess.TMF()` | `*TMFOps` | Task management functions |
| `sess.Protocol()` | `*ProtocolOps` | Low-level iSCSI protocol operations |

### SCSI Commands (`sess.SCSI()`)

| Method | Description |
|--------|-------------|
| `ReadBlocks` | Read blocks from a LUN |
| `WriteBlocks` | Write blocks to a LUN |
| `Inquiry` | SCSI INQUIRY |
| `ReadCapacity` | Query LUN capacity |
| `TestUnitReady` | Check LUN readiness |
| `ModeSense6` / `ModeSense10` | Query mode pages |
| `ModeSelect6` / `ModeSelect10` | Set mode pages |
| `ReportLuns` | Enumerate LUNs |
| `SynchronizeCache` | Flush volatile cache |
| `Verify` | Verify LBA range |
| `WriteSame` | Fill LBA range |
| `Unmap` | Thin provisioning deallocate |
| `CompareAndWrite` | Atomic read-compare-write |
| `StartStopUnit` | Power management |
| `PersistReserveIn` / `PersistReserveOut` | Persistent reservations |

### Raw CDB (`sess.Raw()`)

| Method | Description |
|--------|-------------|
| `Execute` | Send any CDB, returns buffered `*RawResult` with `[]byte` data |
| `StreamExecute` | Send any CDB, returns streaming `*StreamResult` with `io.Reader` data |
| `WithDataIn` | Configure read response allocation |
| `WithDataOut` | Configure write data |

> **Security notice:** `Execute` and `StreamExecute` accept arbitrary CDB bytes. Callers can issue any SCSI command to the target, including destructive operations (FORMAT UNIT, WRITE SAME with unmap, persistent reservation preempt-and-abort). This is intentional for a low-level library. Applications must ensure only authorized users can invoke these methods.

### Helpers

| Function | Description |
|----------|-------------|
| `ParseSenseData` | Parse raw sense bytes into `SenseInfo` |
| `CheckStatus` | Convert SCSI status + sense into `*SCSIError` |
| `DecodeLUN` | Decode SAM LUN encoding |
| `DeviceTypeName` | Human-readable device type |

## Performance Tuning

For high-throughput workloads (tape drives, large sequential I/O):

```go
sess, err := uiscsi.Dial(ctx, addr,
    uiscsi.WithTarget(iqn),
    uiscsi.WithStreamBufDepth(128),              // streaming PDU buffer (default 128)
    uiscsi.WithRouterBufDepth(64),               // dispatch buffer (default 64)
    uiscsi.WithMaxRecvDataSegmentLength(262144),  // max PDU size (default 8KB)
    uiscsi.WithMaxBurstLength(524288),            // write burst size (default 256KB)
    uiscsi.WithFirstBurstLength(131072),          // unsolicited write (default 64KB)
)
```

**StreamBufDepth** and **RouterBufDepth** control internal PDU buffering. These are critical for tape drives: shallow buffers cause TCP backpressure during GC pauses, stopping the tape drive (shoe-shining). The defaults (128 + 64) provide ~1.5MB of buffering at 8KB MRDSL — enough to absorb 50+ ms of consumer stalls.

## Observability

Three hook types allow callers to instrument sessions without modifying library internals.

### Metrics Hook

`WithMetricsHook` receives a `CommandMetrics` struct after each SCSI command completes:

```go
sess, err := uiscsi.Dial(ctx, addr,
    uiscsi.WithTarget(iqn),
    uiscsi.WithMetricsHook(func(m uiscsi.CommandMetrics) {
        log.Printf("opcode=0x%02x latency=%s bytesIn=%d bytesOut=%d err=%v",
            m.Opcode, m.Latency, m.BytesIn, m.BytesOut, m.Err)
    }),
)
```

`CommandMetrics` fields: `Opcode` (SCSI command code), `Latency` (round-trip duration), `BytesIn` (data received from target), `BytesOut` (data sent to target), `Err` (nil on success).

### State Change Hook

`WithStateChangeHook` receives callbacks when the session transitions between states:

```go
sess, err := uiscsi.Dial(ctx, addr,
    uiscsi.WithTarget(iqn),
    uiscsi.WithStateChangeHook(func(old, new uiscsi.SessionState) {
        log.Printf("session state: %s -> %s", old, new)
    }),
)
```

States: `StateLogin`, `StateFullFeature`, `StateRecovery`, `StateClosed`.

### PDU Hook

`WithPDUHook` exposes raw PDU bytes for wire-level inspection and debugging:

```go
sess, err := uiscsi.Dial(ctx, addr,
    uiscsi.WithTarget(iqn),
    uiscsi.WithPDUHook(func(direction uiscsi.PDUDirection, pdu []byte) {
        // direction is PDUDirectionSend or PDUDirectionRecv
        log.Printf("%s PDU opcode=0x%02x len=%d", direction, pdu[0]&0x3f, len(pdu))
    }),
)
```

The hook is called synchronously in the read/write pump goroutine. Keep it fast to avoid backpressure.

### Logger

`WithLogger` injects a `slog.Logger` for session diagnostics. The library emits structured debug-level events for login negotiation, connection lifecycle, and error recovery:

```go
logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
sess, err := uiscsi.Dial(ctx, addr,
    uiscsi.WithTarget(iqn),
    uiscsi.WithLogger(logger),
)
```

### Error Types

| Type | Description |
|------|-------------|
| `SCSIError` | SCSI command failure with sense data |
| `TransportError` | iSCSI transport/connection failure |
| `AuthError` | Authentication failure |

## Testing

The library includes three test tiers:

- **Unit tests** -- table-driven tests for PDU codec, serial arithmetic, sense parsing (`go test ./...`)
- **Conformance tests** -- 87 wire-level tests against an in-process mock iSCSI target with PDU capture (`test/conformance/`). Covers 84% of the UNH-IOL Initiator Full Feature Phase test suite (see `doc/test_matrix_initiator_ffp.md`).
- **E2E tests** -- 21 tests against a real Linux LIO kernel target via configfs (`sudo go test -tags e2e ./test/e2e/`)

All test suites run with `-race` and [goleak](https://github.com/uber-go/goleak) to catch goroutine leaks.

## Requirements

- Go 1.25 or later
- No external runtime dependencies (stdlib only)
- E2E tests require Linux with `target_core_mod` and `iscsi_target_mod` kernel modules
