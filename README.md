# uiscsi

A pure-userspace iSCSI initiator library for Go.

## Overview

uiscsi implements the iSCSI protocol (RFC 7143) entirely in userspace. There are no kernel modules, no open-iscsi dependency, and no external tools. Go applications import the library and communicate directly with iSCSI targets over TCP.

The library provides both a high-level typed API (ReadBlocks, WriteBlocks, Inquiry) and a low-level raw CDB pass-through for arbitrary SCSI commands. It supports CHAP and mutual CHAP authentication, header and data digest negotiation with CRC32C, and error recovery levels 0 through 2.

## Features

- **Pure userspace** -- no kernel iSCSI stack, no iscsiadm, no external tools
- **RFC 7143 compliant** -- PDU codec, login negotiation, full feature phase
- **Go-idiomatic API** -- context.Context, io.Reader/io.Writer, functional options
- **Authentication** -- CHAP and mutual CHAP
- **Block I/O** -- ReadBlocks/WriteBlocks with `[]byte`, streaming with io.Reader
- **Raw CDB pass-through** -- send any SCSI command via Execute
- **Error recovery** -- ERL 0 (session reconnect), ERL 1 (SNACK), ERL 2 (connection replace)
- **Task management** -- ABORT TASK, LUN RESET, TARGET WARM/COLD RESET
- **Observability** -- slog structured logging, PDU hooks, metrics callbacks
- **Digests** -- CRC32C header and data digest negotiation and verification

## Quick Start

```
go get github.com/rkujawa/uiscsi
```

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/rkujawa/uiscsi"
)

func main() {
    ctx := context.Background()

    // Discover available targets.
    targets, err := uiscsi.Discover(ctx, "192.168.1.100:3260")
    if err != nil {
        log.Fatal(err)
    }
    for _, t := range targets {
        fmt.Println(t.Name)
    }

    // Connect to a target.
    sess, err := uiscsi.Dial(ctx, "192.168.1.100:3260",
        uiscsi.WithTarget("iqn.2026-03.com.example:storage"),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer sess.Close()

    // Read the first block.
    data, err := sess.ReadBlocks(ctx, 0, 0, 1, 512)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Read %d bytes from LBA 0\n", len(data))
}
```

## Examples

Complete example programs are in the `examples/` directory:

- **[examples/discover-read/](examples/discover-read/)** -- Discover targets, login, query capacity, read blocks
- **[examples/write-verify/](examples/write-verify/)** -- Write blocks and verify with readback
- **[examples/raw-cdb/](examples/raw-cdb/)** -- Send custom SCSI commands via raw CDB pass-through
- **[examples/error-handling/](examples/error-handling/)** -- Typed error handling and recovery patterns

## API Reference

Full documentation is available on [pkg.go.dev](https://pkg.go.dev/github.com/rkujawa/uiscsi).

Key types and functions:

| Function/Type | Description |
|---------------|-------------|
| `Dial` | Connect to a target and return a Session |
| `Discover` | Enumerate available iSCSI targets |
| `Session.ReadBlocks` | Read blocks from a LUN |
| `Session.WriteBlocks` | Write blocks to a LUN |
| `Session.Execute` | Raw CDB pass-through |
| `Session.Inquiry` | SCSI INQUIRY command |
| `Session.ReadCapacity` | Query LUN capacity |
| `WithTarget` | Set target IQN |
| `WithCHAP` | Enable CHAP authentication |
| `WithLogger` | Inject slog.Logger |
| `SCSIError` | SCSI command failure with sense data |
| `TransportError` | iSCSI transport/connection failure |
| `AuthError` | Authentication failure |

## Requirements

- Go 1.25 or later
- No external dependencies (stdlib only)
