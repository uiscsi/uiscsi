---
phase: 01-pdu-codec-and-transport
plan: 03
subsystem: protocol
tags: [iscsi, transport, tcp, framing, pdu, pump, router, itt, sync-pool, rfc7143]

# Dependency graph
requires:
  - phase: 01-pdu-codec-and-transport plan 01
    provides: PadLen helper for 4-byte alignment, CRC32C digest functions
  - phase: 01-pdu-codec-and-transport plan 02
    provides: BHSLength constant, PDU types, DecodeBHS dispatcher, opcode constants
provides:
  - TCP connection wrapper with Dial(ctx), Close, deadline management
  - PDU framing over TCP (ReadRawPDU/WriteRawPDU) using io.ReadFull exclusively
  - Concurrent read/write pump goroutines for full-duplex PDU transport
  - ITT-based response routing with reserved 0xFFFFFFFF handling
  - sync.Pool buffer management with size-class pooling (4KB/64KB/16MB)
  - Unsolicited PDU channel for target-initiated messages (NOP-In, AsyncMsg)
affects: [02-login-negotiation, 03-full-feature-phase, 04-error-recovery]

# Tech tracking
tech-stack:
  added: [log/slog, net, context, sync]
  patterns: [pump-goroutines, itt-routing, buffer-pooling, io-readfull-framing]

key-files:
  created:
    - internal/transport/pool.go
    - internal/transport/framer.go
    - internal/transport/conn.go
    - internal/transport/router.go
    - internal/transport/pump.go
    - internal/transport/framer_test.go
    - internal/transport/conn_test.go
    - internal/transport/router_test.go
    - internal/transport/pump_test.go
  modified: []

key-decisions:
  - "io.ReadFull exclusively for all TCP reads (Pitfall 6) -- never raw conn.Read"
  - "Single WritePump goroutine serializes all writes to prevent TCP interleaving (Pitfall 7)"
  - "Size-class buffer pooling (4KB/64KB/16MB) with oversized fallback to direct allocation"
  - "Copy-out ownership: ReadRawPDU copies data segment to caller-owned memory, returns pool buffers"
  - "Router skips reserved ITT 0xFFFFFFFF and wraps around from 0xFFFFFFFE to 0x00000000"

patterns-established:
  - "Pump pattern: dedicated goroutines for read/write with channel-based PDU dispatch"
  - "ITT routing: Register returns channel, Dispatch delivers, Unregister for cleanup"
  - "net.Pipe() for transport-layer tests (no real TCP needed)"
  - "Unsolicited channel for target-initiated PDUs separate from ITT routing"

requirements-completed: [XPORT-01, XPORT-02, XPORT-03, XPORT-04, TEST-03]

# Metrics
duration: 4min
completed: 2026-03-31
---

# Phase 01 Plan 03: TCP Transport Layer Summary

**PDU framing over TCP with io.ReadFull, concurrent read/write pump goroutines, ITT-based response routing skipping reserved 0xFFFFFFFF, and sync.Pool buffer management -- 27 tests passing under -race**

## Performance

- **Duration:** 4 min
- **Started:** 2026-03-31T20:44:06Z
- **Completed:** 2026-03-31T20:47:49Z
- **Tasks:** 2
- **Files modified:** 9

## Accomplishments
- Complete PDU framing with ReadRawPDU/WriteRawPDU using io.ReadFull for correct TCP stream handling
- Concurrent read/write pump goroutines enabling full-duplex iSCSI transport with no data corruption under -race
- ITT-based response router that correctly skips reserved 0xFFFFFFFF and routes unsolicited PDUs to separate channel
- sync.Pool buffer management with 3 size classes reducing GC pressure during transport I/O

## Task Commits

Each task was committed atomically:

1. **Task 1: PDU framing, buffer pool, and connection wrapper** - `28ea821` (feat)
2. **Task 2: Read/write pump goroutines and ITT-based PDU router** - `6b7558d` (feat)

## Files Created/Modified
- `internal/transport/pool.go` - sync.Pool buffer management with size-class pooling (BHS, small/medium/large data)
- `internal/transport/framer.go` - ReadRawPDU/WriteRawPDU: PDU framing with BHS + AHS + digests + data + padding
- `internal/transport/conn.go` - TCP connection wrapper with Dial(ctx), Close, deadlines, digest/MaxRecvDSL config
- `internal/transport/router.go` - ITT-based PDU dispatch with Register/Dispatch/Unregister, reserved ITT skip
- `internal/transport/pump.go` - WritePump and ReadPump goroutines for concurrent PDU transport
- `internal/transport/framer_test.go` - 15 tests: back-to-back PDUs, AHS, digests, padding variants, truncation, round-trips
- `internal/transport/conn_test.go` - 5 tests: dial success/cancel, close, BHS pool, data buffer pool
- `internal/transport/router_test.go` - 7 tests: monotonic ITTs, 0xFFFFFFFF skip, dispatch, unregister, concurrent
- `internal/transport/pump_test.go` - 6 tests: basic write, dispatch, unsolicited, concurrent writers, shutdown, full round-trip

## Decisions Made
- Used io.ReadFull for all TCP reads per Pitfall 6 -- raw Read may return partial data on TCP
- Single WritePump goroutine owns all writes per Pitfall 7 -- prevents TCP byte interleaving from concurrent senders
- Size-class buffer pooling (4KB/64KB/16MB) with direct allocation fallback for oversized buffers
- Copy-out ownership model: ReadRawPDU copies data segment into fresh caller-owned slice, pool buffers returned immediately
- Router wraps ITT from 0xFFFFFFFE to 0x00000000, never allocating reserved 0xFFFFFFFF per RFC 7143
- Used log/slog for transport-layer warnings (dropped unsolicited PDU, unknown ITT) -- library consumers can configure handler

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- Transport layer ready for login negotiation (Phase 2): Conn.Dial connects, pumps frame PDUs, router dispatches responses
- WritePump channel is the single entry point for sending PDUs -- login negotiation sends LoginReq via this channel
- ReadPump + Router deliver LoginResp to the correct waiter by ITT
- Unsolicited channel ready for NOP-In pings and async messages from target
- Digest flags on Conn configurable after login negotiation completes
- MaxRecvDataSegmentLength enforcement ready at transport layer
- Full Phase 1 test suite passes: go test -race ./internal/...

## Self-Check: PASSED

All 9 files verified present. Both commit hashes (28ea821, 6b7558d) found in git log.

---
*Phase: 01-pdu-codec-and-transport*
*Completed: 2026-03-31*
