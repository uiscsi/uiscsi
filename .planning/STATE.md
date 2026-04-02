---
gsd_state_version: 1.0
milestone: v1.0
milestone_name: milestone
status: executing
stopped_at: Phase 8 context gathered
last_updated: "2026-04-02T12:14:39.270Z"
last_activity: 2026-04-02
progress:
  total_phases: 9
  completed_phases: 8
  total_plans: 25
  completed_plans: 25
  percent: 92
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-03-31)

**Core value:** Full RFC 7143 compliance as a composable Go library
**Current focus:** Phase 07 — public-api-observability-and-release

## Current Position

Phase: 07
Plan: Not started
Status: Ready to execute
Last activity: 2026-04-02

Progress: [█████████░] 92%

## Performance Metrics

**Velocity:**

- Total plans completed: 0
- Average duration: -
- Total execution time: 0 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| - | - | - | - |

**Recent Trend:**

- Last 5 plans: -
- Trend: -

*Updated after each plan completion*
| Phase 01 P01 | 3min | 2 tasks | 7 files |
| Phase 01 P02 | 6min | 2 tasks | 11 files |
| Phase 01 P03 | 4min | 2 tasks | 9 files |
| Phase 02 P02 | 2min | 1 tasks | 2 files |
| Phase 02 P03 | 4min | 2 tasks | 2 files |
| Phase 03 P01 | 12min | 2 tasks | 11 files |
| Phase 03 P02 | 7min | 2 tasks | 6 files |
| Phase 03 P03 | 4min | 2 tasks | 3 files |
| Phase 04 P01 | 5min | 3 tasks | 5 files |
| Phase 04 P03 | 9min | 2 tasks | 1 files |
| Phase 05 P01 | 6min | 2 tasks | 11 files |
| Phase 05 P02 | 4min | 2 tasks | 5 files |
| Phase 06 P02 | 10min | 2 tasks | 9 files |
| Phase 06.1 P01 | 4min | 2 tasks | 6 files |
| Phase 06.1 P02 | 4min | 2 tasks | 7 files |
| Phase 06.1 P03 | 7min | 3 tasks | 7 files |
| Phase 07 P01 | 6min | 2 tasks | 8 files |
<<<<<<< Updated upstream
| Phase 07 P03 | 2min | 2 tasks | 6 files |
=======
| Phase 07 P02 | 7min | 3 tasks | 7 files |
>>>>>>> Stashed changes

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

-

- [Phase 01]: int32 cast trick for RFC 1982 serial comparison
- [Phase 01]: Package-level crc32cTable for one-time CRC32C init
- [Phase 01]: Double-modulo padding formula (4-(n%4))%4 to avoid returning 4 for aligned inputs
- [Phase 01]: Typed PDU per opcode with embedded Header base struct (D-01/D-03 compliance)
- [Phase 01]: 3-byte manual encoding for DataSegmentLength to avoid TotalAHSLength corruption
- [Phase 01]: Login PDU byte 1 reuses Final bit position for Transit bit per RFC 7143 Section 11.12
- [Phase 01]: io.ReadFull exclusively for TCP reads (Pitfall 6), single WritePump goroutine for write serialization (Pitfall 7)
- [Phase 01]: Size-class buffer pooling (4KB/64KB/16MB) with copy-out ownership model for transport I/O
- [Phase 01]: Router wraps ITT 0xFFFFFFFE->0x00000000, never allocating reserved 0xFFFFFFFF per RFC 7143
- [Phase 02]: Package-private CHAP functions consumed only by login state machine
- [Phase 02]: Constant-time comparison for mutual CHAP response verification
- [Phase 02]: Synchronous PDU exchange via raw net.Conn during login (not pumps)
- [Phase 02]: buildInitiatorKeys in login.go for login-specific key proposal construction
- [Phase 02]: Mock target uses loopback TCP for realistic login integration testing
- [Phase 03]: Buffered Data-In reassembly (bytes.Buffer) instead of io.Pipe to avoid deadlock
- [Phase 03]: Per-task goroutine drains Router channel preventing slow reader from blocking
- [Phase 03]: Router refactored with routerEntry struct for persistent multi-PDU registrations
- [Phase 03]: Refactored handleUnsolicited into opcode-based dispatch to dedicated handlers
- [Phase 03]: Logout() drains tasks before CmdSN acquire; Close() attempts graceful logout with 5s timeout
- [Phase 03]: Persistent Router registration for SendTargets multi-PDU continuation
- [Phase 04]: io.Reader on Command for write data -- callers use bytes.NewReader for []byte, enables streaming
- [Phase 04]: Auto-set W-bit when cmd.Data \!= nil -- callers don't need to set both Data and Write
- [Phase 04]: Immediate data bounded by min(FirstBurstLength, MaxRecvDataSegmentLength) per RFC 7143
- [Phase 04]: Matrix test uses 2048B payload with FirstBurstLength=1024, MaxRecvDSL=512 to exercise all four write mode paths
- [Phase 05]: ~70 ASC/ASCQ entries in lookup table covering common SPC-4 Annex D codes
- [Phase 05]: checkResult helper centralizes status check + sense parse + data read for all parse functions
- [Phase 05]: CDB builder pattern: plain functions return session.Command with packed CDB bytes
- [Phase 05]: VPD 0x83 page length from bytes 2-3 as BigEndian.Uint16 (unlike 0x00/0x80 single-byte)
- [Phase 05]: Association field is bits 5-4 of VPD 0x83 descriptor byte 1 (2-bit field)
- [Phase 06]: Wait for old dispatchLoop done before replacing channels during reconnect (race fix)
- [Phase 06]: Task stores original Command for ERL 0 retry; reuse resultCh for transparent caller recovery
- [Phase 06.1]: DigestError uses pointer receiver for errors.As compatibility
- [Phase 06.1]: PDU stringer format: TypeName{key:value} with hex tags, decimal sequence numbers, CDB truncated to 8 bytes
- [Phase 06.1]: Direction constants (HookSend/HookReceive) in transport package to avoid session->transport circular dependency
- [Phase 06.1]: pduHookBridge returns nil when no hooks configured for zero-cost hot path
- [Phase 06.1]: Push-based MetricEvent callback (no concrete stats struct) per D-11 design
- [Phase 06.1]: DigestError unconditionally connection-fatal (no reconnect) per RFC 7143 Section 7.3
- [Phase 06.1]: Login logger injected via WithLoginLogger option, defaults to slog.Default()
- [Phase 07]: All public types are value types in root package -- no internal type leakage
- [Phase 07]: WithPDUHook adapter concatenates BHS+DataSegment into []byte to avoid exposing transport.RawPDU

- [Phase 07]: Godoc examples have no Output markers since they connect to non-existent addresses
- [Phase 07]: MockTarget uses opcode-keyed handler dispatch for composable test targets
- [Phase 07]: Conformance tests use external test package (conformance_test) for public API-only validation
- [Phase 07]: Integration tests behind //go:build integration tag per D-07 tiered approach

### Roadmap Evolution

- Phase 06.1 inserted after Phase 6: Observability and Debugging Infrastructure (URGENT) — fill all debugging gaps before E2E testing in Phase 7
- Phase 8 added: lsscsi-style discovery utility — standalone CLI using uiscsi library

### Pending Todos

None yet.

### Blockers/Concerns

- Verify gostor/gotgt compatibility with Go 1.25 early in Phase 1
- Verify Go CRC32C hardware acceleration and TCP networking on NetBSD 10.1

## Session Continuity

<<<<<<< Updated upstream
Last session: 2026-04-02T12:14:39.265Z
Stopped at: Phase 8 context gathered
=======
Last session: 2026-04-02T08:55:48.530Z
Stopped at: Completed 07-02-PLAN.md
>>>>>>> Stashed changes
Resume file: .planning/phases/08-lsscsi-discovery-utility/08-CONTEXT.md
