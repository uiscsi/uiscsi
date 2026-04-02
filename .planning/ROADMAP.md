# Roadmap: uiscsi

## Overview

Build a pure-userspace iSCSI initiator library in Go from the bottom up, following the natural protocol dependency chain: PDU codec and transport framing first, then connection management and login negotiation, then session layer with read I/O, then the complex write path in isolation, then SCSI command builders and sense parsing, then error recovery and task management, and finally the public API surface with documentation. Each phase delivers a verifiable protocol layer that the next phase builds upon.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: PDU Codec and Transport** - Binary PDU encoding/decoding, CRC32C digests, serial number arithmetic, TCP framing, and PDU routing
- [x] **Phase 2: Connection and Login** - Connection state machine, read/write pumps, login negotiation, authentication, digest and operational parameter negotiation (completed 2026-03-31)
- [ ] **Phase 3: Session, Read Path, and Discovery** - Session state machine, command windowing, Data-In read path, keepalive, async events, logout, and SendTargets discovery
- [x] **Phase 4: Write Path** - R2T handling, Data-Out generation, immediate and unsolicited data, burst length enforcement (completed 2026-04-01)
- [ ] **Phase 5: SCSI Command Layer** - CDB builders and response parsers for all core and extended SCSI commands, structured sense data parsing
- [ ] **Phase 6: Error Recovery and Task Management** - ERL 0/1/2 recovery mechanisms and all six task management functions
- [ ] **Phase 7: Public API, Observability, and Release** - High-level and low-level APIs, observability hooks, integration test suite, documentation, and examples

## Phase Details

### Phase 1: PDU Codec and Transport
**Goal**: A Go application can encode, decode, and frame all iSCSI PDU types over TCP with correct padding, digest computation, and sequence number arithmetic
**Depends on**: Nothing (first phase)
**Requirements**: PDU-01, PDU-02, PDU-03, PDU-04, XPORT-01, XPORT-02, XPORT-03, XPORT-04, TEST-03
**Success Criteria** (what must be TRUE):
  1. All 24 iSCSI PDU opcodes can be round-trip encoded and decoded with byte-perfect fidelity (BHS + AHS + data segment + padding)
  2. CRC32C digests computed over arbitrary PDU headers and data segments match known test vectors from RFC 3720 Appendix B
  3. Serial number comparisons (CmdSN, StatSN, DataSN) correctly handle wrap-around at 2^32 boundaries per RFC 1982
  4. Two goroutines (read pump and write pump) can concurrently frame PDUs over a TCP connection without data corruption, verified under -race
  5. Table-driven unit tests cover all PDU types including edge cases (zero-length data, maximum AHS, padding boundaries)
**Plans:** 3 plans
Plans:
- [x] 01-01-PLAN.md — Go module init, serial arithmetic, CRC32C digest, padding helpers
- [x] 01-02-PLAN.md — All 24 PDU opcode types with BHS codec and round-trip tests
- [x] 01-03-PLAN.md — TCP transport: framing, read/write pumps, ITT router, buffer pool

### Phase 2: Connection and Login
**Goal**: A Go application can establish an authenticated iSCSI connection with full operational parameter negotiation, including digest settings
**Depends on**: Phase 1
**Requirements**: LOGIN-01, LOGIN-02, LOGIN-03, LOGIN-04, LOGIN-05, LOGIN-06, INTEG-01, INTEG-02, INTEG-03, TEST-04
**Success Criteria** (what must be TRUE):
  1. Login succeeds with AuthMethod=None against a test target and transitions to full feature phase
  2. Login succeeds with CHAP authentication (target authenticates initiator) and with mutual CHAP (bidirectional)
  3. All RFC 7143 Section 13 mandatory operational parameters are negotiated correctly (HeaderDigest, DataDigest, MaxRecvDataSegmentLength, MaxBurstLength, FirstBurstLength, ImmediateData, InitialR2T, etc.)
  4. When HeaderDigest=CRC32C or DataDigest=CRC32C is negotiated, received PDUs with incorrect digests are detected and rejected
  5. Parameterized tests cover the negotiation parameter matrix (boolean AND/OR, numerical min/max, string list semantics)
**Plans:** 3/3 plans complete
Plans:
- [x] 02-01-PLAN.md — Text codec, negotiation engine, NegotiatedParams, LoginError
- [x] 02-02-PLAN.md — CHAP authentication (one-way and mutual)
- [x] 02-03-PLAN.md — Login state machine, functional options, mock target tests, digest activation

### Phase 3: Session, Read Path, and Discovery
**Goal**: A Go application can open a session, discover targets, issue SCSI read commands, and receive data with correct sequencing and flow control
**Depends on**: Phase 2
**Requirements**: SESS-01, SESS-02, SESS-03, SESS-04, SESS-05, READ-01, READ-02, READ-03, EVT-01, EVT-02, EVT-03, DISC-01, DISC-02
**Success Criteria** (what must be TRUE):
  1. A session can be established and CmdSN/MaxCmdSN command windowing correctly throttles outstanding commands
  2. A SCSI read command returns correct data assembled from one or more Data-In PDUs with proper sequence number validation
  3. NOP-Out/NOP-In keepalive works in both directions (initiator-originated ping and response to target-initiated NOP-In)
  4. SendTargets discovery enumerates available targets and their portal addresses from a discovery session
  5. Graceful logout tears down the session cleanly, and async messages from the target (including target-requested logout) are handled
**Plans:** 3 plans
Plans:
- [x] 03-01-PLAN.md — Session core: types, CmdSN windowing, Submit, Data-In reassembly
- [x] 03-02-PLAN.md — Keepalive, async events, logout
- [ ] 03-03-PLAN.md — SendTargets discovery and Discover convenience function

### Phase 4: Write Path
**Goal**: A Go application can write data to an iSCSI target through all write path variants with correct R2T handling and burst length enforcement
**Depends on**: Phase 3
**Requirements**: WRITE-01, WRITE-02, WRITE-03, WRITE-04, WRITE-05
**Success Criteria** (what must be TRUE):
  1. Solicited writes work: target sends R2T, initiator responds with correct Data-Out PDUs respecting MaxBurstLength and R2TSN tracking
  2. Immediate data works: write data piggybacks on SCSI Command PDU when ImmediateData=Yes, bounded by FirstBurstLength
  3. Unsolicited Data-Out works: when InitialR2T=No, initiator sends data before first R2T, bounded by FirstBurstLength
  4. All four ImmediateData x InitialR2T combinations produce correct wire behavior, verified by parameterized tests
  5. MaxOutstandingR2T is respected and MaxBurstLength is enforced for all solicited data sequences
**Plans:** 4 plans (3 complete + 1 gap closure)
Plans:
- [x] 04-01-PLAN.md — Command type io.Reader migration, Submit write detection, immediate data
- [x] 04-02-PLAN.md — Data-Out engine: R2T handling, solicited/unsolicited Data-Out, MaxBurstLength
- [x] 04-03-PLAN.md — Parameterized 2x2 ImmediateData x InitialR2T matrix and edge case tests
- [x] 04-04-PLAN.md — Gap closure: configurable close timeout for fast test teardown

### Phase 5: SCSI Command Layer
**Goal**: A Go application can issue all core and extended SCSI commands with structured CDB building and response parsing, including sense data interpretation
**Depends on**: Phase 3 (read path for verification), Phase 4 (write path for write commands)
**Requirements**: SCSI-01, SCSI-02, SCSI-03, SCSI-04, SCSI-05, SCSI-06, SCSI-07, SCSI-08, SCSI-09, SCSI-10, SCSI-11, SCSI-12, SCSI-13, SCSI-14, SCSI-15, SCSI-16, SCSI-17, SCSI-18, SCSI-19
**Success Criteria** (what must be TRUE):
  1. Core SCSI commands (TEST UNIT READY, INQUIRY, READ CAPACITY, READ/WRITE 10/16, REQUEST SENSE, REPORT LUNS, MODE SENSE) produce correctly formatted CDBs and parse responses into typed Go structs
  2. INQUIRY VPD pages (0x00, 0x80, 0x83) and extended VPD pages (0xB0, 0xB1, 0xB2) are parsed into structured data
  3. Sense data in both fixed and descriptor formats is parsed with correct sense key, ASC/ASCQ classification
  4. Extended SCSI commands (SYNCHRONIZE CACHE, WRITE SAME, UNMAP, VERIFY, PERSISTENT RESERVE IN/OUT, COMPARE AND WRITE, START STOP UNIT) produce valid CDBs
  5. All CDB builders can be verified with round-trip tests independent of a live iSCSI target
**Plans:** 2/3 plans executed
Plans:
- [x] 05-01-PLAN.md — Foundation types, sense parsing, core commands (TUR, INQUIRY, READ CAPACITY, REQUEST SENSE, REPORT LUNS, MODE SENSE)
- [x] 05-02-PLAN.md — READ/WRITE 10/16 CDB builders and VPD page parsers (0x00, 0x80, 0x83, 0xB0, 0xB1, 0xB2)
- [ ] 05-03-PLAN.md — Extended commands (SYNC CACHE, WRITE SAME, UNMAP, VERIFY, PR IN/OUT, COMPARE AND WRITE, START STOP UNIT)

### Phase 6: Error Recovery and Task Management
**Goal**: A Go application can recover from connection failures at all three error recovery levels and manage outstanding tasks
**Depends on**: Phase 4, Phase 5
**Requirements**: ERL-01, ERL-02, ERL-03, TMF-01, TMF-02, TMF-03, TMF-04, TMF-05, TMF-06, TEST-05
**Success Criteria** (what must be TRUE):
  1. ERL 0: after a connection drop, the session is reinstated with correct ISID handling, and in-flight commands are retried
  2. ERL 1: SNACK mechanism requests retransmission of missing Data-In or status PDUs without dropping the connection
  3. ERL 2: a failed connection within a session can be replaced and tasks reassigned to the new connection
  4. All six task management functions (ABORT TASK, ABORT TASK SET, LUN RESET, TARGET WARM RESET, TARGET COLD RESET, CLEAR TASK SET) send correct TMF requests and process responses
  5. Error injection tests verify recovery behavior under simulated connection failures, timeout scenarios, and digest errors
**Plans:** 3 plans
Plans:
- [x] 06-01-PLAN.md — faultConn test utility, TMF types and constants, all six TMF methods with auto-cleanup
- [x] 06-02-PLAN.md — ERL 0: WithTSIH login option, reconnect FSM, session reinstatement, in-flight command retry
- [ ] 06-03-PLAN.md — ERL 1 SNACK-based retransmission + ERL 2 connection replacement with task reassignment

### Phase 06.1: Observability and Debugging Infrastructure (INSERTED)

**Goal:** Fill all observability gaps so E2E tests can diagnose protocol compliance issues from logs alone — digest verification, PDU tracing, state machine logging, enriched errors, and connection metrics
**Requirements**: OBS-01, OBS-02, OBS-03
**Depends on:** Phase 6
**Success Criteria** (what must be TRUE):
  1. Received CRC32C digests are verified against computed values; structured errors returned on mismatch
  2. All 18 PDU types have String() methods producing human-readable dump output
  3. PDU send/receive hooks (middleware pattern) allow traffic logging in tests
  4. Errors include CmdSN, StatSN, ITT, and data offsets where applicable
  5. Login stage transitions, task lifecycle, and command window changes are logged via slog at Debug level
  6. Connection-level metrics (PDU counts by type, bytes in/out, command latency) are available to consumers
  7. Full PDU exchanges are traceable at slog Debug level
**Plans:** 3/3 plans complete
Plans:
- [x] 06.1-01-PLAN.md — DigestError type, digest verification in ReadRawPDU, PDU String() methods
- [x] 06.1-02-PLAN.md — PDU hooks, metrics events, pump logger injection
- [x] 06.1-03-PLAN.md — Structured slog lifecycle logging, enriched errors

### Phase 7: Public API, Observability, and Release
**Goal**: Library consumers can use a clean, Go-idiomatic API with both high-level convenience and low-level control, backed by observability and comprehensive documentation
**Depends on**: Phase 5, Phase 6
**Requirements**: API-01, API-02, API-03, API-04, API-05, OBS-01, OBS-02, OBS-03, TEST-01, TEST-02, DOC-01, DOC-02, DOC-03, DOC-04, DOC-05
**Success Criteria** (what must be TRUE):
  1. A developer can perform discovery, login, read blocks, write blocks, and logout using high-level typed functions (ReadBlocks, WriteBlocks, Inquiry, etc.) with structured return types
  2. A developer can send arbitrary SCSI commands via raw CDB pass-through for commands the library does not wrap
  3. All operations accept context.Context for cancellation and timeouts, and block I/O exposes io.Reader/io.Writer where natural
  4. Connection statistics (latency, throughput, error counts), structured slog logging, and state transition callbacks are available to consumers
  5. IOL-inspired conformance test suite runs against automated test infrastructure with no manual SAN setup, and godoc plus four worked examples cover discovery, read, write, raw CDB, and error handling
**Plans:** 3 plans
Plans:
- [x] 07-01-PLAN.md — Public API surface: types, errors, options, Dial/Discover, Session methods, streaming I/O
- [ ] 07-02-PLAN.md — Mock target infrastructure and IOL-inspired conformance test suite
- [x] 07-03-PLAN.md — Documentation: godoc examples, example programs, README

### Phase 8: lsscsi-style discovery utility

**Goal:** Build a standalone CLI tool (`uiscsi-ls`) that performs iSCSI target discovery on specified portals and presents LUN information in lsscsi-style columnar format or JSON, using the uiscsi library as its backend.
**Requirements**: CLI-01, CLI-02, CLI-03, CLI-04, CLI-05, CLI-06
**Depends on:** Phase 7
**Success Criteria** (what must be TRUE):
  1. `uiscsi-ls --portal <addr>` discovers targets, connects to each, probes all LUNs, and displays results in lsscsi-style columnar format
  2. `--json` flag produces machine-parseable nested JSON output
  3. CHAP credentials resolve from flags with env var fallback
  4. Multiple portals can be specified via repeated `--portal` flags
  5. Unreachable portals are skipped with errors to stderr; remaining portals still probed
**Plans:** 2 plans

Plans:
- [x] 08-01-PLAN.md — Go module setup, result types, device type table, formatters with tests
- [x] 08-02-PLAN.md — Probe pipeline, CLI main.go with flag parsing, signal handling, exit codes

## Progress

**Execution Order:**
Phases execute in numeric order: 1 -> 2 -> 3 -> 4 -> 5 -> 6 -> 7

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. PDU Codec and Transport | 3/3 | Complete | - |
| 2. Connection and Login | 3/3 | Complete   | 2026-03-31 |
| 3. Session, Read Path, and Discovery | 0/3 | Planning complete | - |
| 4. Write Path | 3/4 | Gap closure | 2026-04-01 |
| 5. SCSI Command Layer | 2/3 | In Progress|  |
| 6. Error Recovery and Task Management | 0/3 | Planning complete | - |
| 7. Public API, Observability, and Release | 0/3 | Planning complete | - |
| 8. lsscsi-style discovery utility | 0/2 | Planning complete | - |
