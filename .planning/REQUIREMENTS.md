# Requirements: uiscsi

**Defined:** 2026-03-31
**Core Value:** Full RFC 7143 compliance as a composable Go library

## v1 Requirements

Requirements for initial release. Each maps to roadmap phases.

### PDU Foundation

- [x] **PDU-01**: Binary PDU encoder/decoder for all iSCSI PDU types (BHS + AHS + data segment + padding)
- [x] **PDU-02**: RFC 1982 serial number arithmetic for all sequence number comparisons (CmdSN, StatSN, ExpCmdSN, MaxCmdSN, DataSN, R2TSN)
- [x] **PDU-03**: CRC32C (Castagnoli) computation for header and data digests
- [x] **PDU-04**: PDU padding to 4-byte boundaries per RFC 7143

### Transport

- [x] **XPORT-01**: TCP connection management with configurable timeouts and context cancellation
- [x] **XPORT-02**: PDU framing over TCP (read full BHS, then AHS + data segment based on lengths)
- [x] **XPORT-03**: Dedicated read/write goroutine pumps per connection (no concurrent TCP writes)
- [x] **XPORT-04**: ITT-based PDU routing/correlation (initiator task tag maps responses to outstanding commands)

### Login and Negotiation

- [ ] **LOGIN-01**: Full login phase state machine (security negotiation, operational negotiation, leading connection, normal connection)
- [ ] **LOGIN-02**: Text key-value negotiation engine for all RFC 7143 Section 13 mandatory keys
- [ ] **LOGIN-03**: AuthMethod=None authentication
- [x] **LOGIN-04**: CHAP authentication (one-way: target authenticates initiator)
- [x] **LOGIN-05**: Mutual CHAP authentication (bidirectional: both sides authenticate)
- [ ] **LOGIN-06**: Operational parameter negotiation (HeaderDigest, DataDigest, MaxRecvDataSegmentLength, MaxBurstLength, FirstBurstLength, InitialR2T, ImmediateData, MaxOutstandingR2T, DataPDUInOrder, DataSequenceInOrder, DefaultTime2Wait, DefaultTime2Retain, MaxConnections, ErrorRecoveryLevel)

### Session and Command Windowing

- [ ] **SESS-01**: Session state machine per RFC 7143 connection/session model
- [ ] **SESS-02**: CmdSN/ExpCmdSN/MaxCmdSN command windowing and flow control
- [ ] **SESS-03**: StatSN/ExpStatSN tracking per connection
- [ ] **SESS-04**: SCSI Command PDU generation with proper CDB encapsulation
- [ ] **SESS-05**: NOP-Out/NOP-In keepalive (initiator-originated and target-initiated response)

### Read Path

- [ ] **READ-01**: Data-In PDU handling with sequence number validation and data offset tracking
- [ ] **READ-02**: Multi-PDU read reassembly (gathering Data-In PDUs into complete read response)
- [ ] **READ-03**: Status delivery via Data-In with S-bit or separate SCSI Response PDU

### Write Path

- [ ] **WRITE-01**: R2T (Ready to Transfer) handling with R2TSN tracking and MaxOutstandingR2T compliance
- [ ] **WRITE-02**: Solicited Data-Out PDU generation in response to R2T
- [ ] **WRITE-03**: Immediate data support (write data piggybacked on SCSI Command PDU, bounded by FirstBurstLength)
- [ ] **WRITE-04**: Unsolicited Data-Out when InitialR2T=No (before first R2T, bounded by FirstBurstLength)
- [ ] **WRITE-05**: MaxBurstLength enforcement for solicited data sequences

### Integrity

- [ ] **INTEG-01**: Header digest negotiation and CRC32C verification on received PDUs
- [ ] **INTEG-02**: Data digest negotiation and CRC32C verification on received PDUs
- [ ] **INTEG-03**: Digest generation on outgoing PDUs when negotiated

### Task Management

- [ ] **TMF-01**: ABORT TASK — abort a specific outstanding task by ITT
- [ ] **TMF-02**: ABORT TASK SET — abort all tasks from this initiator on a LUN
- [ ] **TMF-03**: LUN RESET — reset a specific logical unit
- [ ] **TMF-04**: TARGET WARM RESET — reset target (sessions preserved)
- [ ] **TMF-05**: TARGET COLD RESET — reset target (sessions dropped)
- [ ] **TMF-06**: CLEAR TASK SET — clear all tasks on a LUN from all initiators

### Error Recovery

- [ ] **ERL-01**: Error Recovery Level 0 — session-level recovery (detect failure, reconnect, reinstate session, retry commands)
- [ ] **ERL-02**: Error Recovery Level 1 — within-connection recovery (SNACK for data/status retransmission without dropping connection)
- [ ] **ERL-03**: Error Recovery Level 2 — connection-level recovery (connection allegiance reassignment, task reassignment within session)

### Events and Logout

- [ ] **EVT-01**: Async message handling (SCSI async event, target-requested logout, connection/session drop notification, vendor-specific)
- [ ] **EVT-02**: Logout (normal session/connection teardown)
- [ ] **EVT-03**: Logout for connection recovery (remove connection for recovery)

### Discovery

- [ ] **DISC-01**: SendTargets discovery (discovery session type, text request/response for target enumeration)
- [ ] **DISC-02**: Target and LUN enumeration from discovery results

### SCSI Commands — Core

- [ ] **SCSI-01**: TEST UNIT READY
- [ ] **SCSI-02**: INQUIRY (standard data)
- [ ] **SCSI-03**: INQUIRY VPD pages (0x00 supported pages, 0x80 serial number, 0x83 device identification)
- [ ] **SCSI-04**: READ CAPACITY (10) and READ CAPACITY (16)
- [ ] **SCSI-05**: READ (10) and READ (16)
- [ ] **SCSI-06**: WRITE (10) and WRITE (16)
- [ ] **SCSI-07**: REQUEST SENSE
- [ ] **SCSI-08**: REPORT LUNS
- [ ] **SCSI-09**: MODE SENSE (6) and MODE SENSE (10)
- [ ] **SCSI-10**: Structured sense data parsing (fixed and descriptor formats, sense key, ASC/ASCQ classification)

### SCSI Commands — Extended

- [ ] **SCSI-11**: SYNCHRONIZE CACHE (10) and SYNCHRONIZE CACHE (16)
- [ ] **SCSI-12**: WRITE SAME (10) and WRITE SAME (16)
- [ ] **SCSI-13**: UNMAP (thin provisioning / TRIM)
- [ ] **SCSI-14**: VERIFY (10) and VERIFY (16)
- [ ] **SCSI-15**: PERSISTENT RESERVE IN (read reservations/keys)
- [ ] **SCSI-16**: PERSISTENT RESERVE OUT (register, reserve, release, clear, preempt)
- [ ] **SCSI-17**: COMPARE AND WRITE (atomic compare-and-swap at block level)
- [ ] **SCSI-18**: Extended VPD page parsing (0xB0 block limits, 0xB1 block characteristics, 0xB2 logical block provisioning)
- [ ] **SCSI-19**: START STOP UNIT

### API

- [ ] **API-01**: Low-level raw CDB pass-through (user builds CDB bytes, library handles iSCSI transport and response)
- [ ] **API-02**: High-level typed Go functions (ReadBlocks, WriteBlocks, Inquiry, ReadCapacity, TestUnitReady, etc.) with structured return types
- [ ] **API-03**: context.Context integration for cancellation and timeouts on all operations
- [ ] **API-04**: io.Reader/io.Writer interfaces where natural (block-level sequential I/O)
- [ ] **API-05**: Structured error types with sense data, iSCSI status, and response classification

### Observability

- [ ] **OBS-01**: Connection-level statistics (latency, throughput, error counts, retry counts)
- [ ] **OBS-02**: Structured logging via log/slog with configurable levels
- [ ] **OBS-03**: Hooks/callbacks for monitoring connection state transitions

### Testing

- [ ] **TEST-01**: IOL-inspired conformance test suite covering full feature phase
- [ ] **TEST-02**: Integration test infrastructure with automated target setup (no manual SAN configuration)
- [x] **TEST-03**: Table-driven unit tests for PDU encoding/decoding
- [ ] **TEST-04**: Parameterized tests for negotiation parameter matrix
- [ ] **TEST-05**: Error injection tests for recovery level verification

### Documentation and Examples

- [ ] **DOC-01**: Comprehensive API documentation with godoc
- [ ] **DOC-02**: Example: basic discovery, login, read blocks, logout
- [ ] **DOC-03**: Example: write blocks with verification
- [ ] **DOC-04**: Example: raw CDB pass-through for custom SCSI commands
- [ ] **DOC-05**: Example: error handling and recovery

## v2 Requirements

Deferred to future release. Tracked but not in current roadmap.

### Multi-Connection

- **MC-01**: Multiple connections per session (MC/S) with connection allegiance and task routing
- **MC-02**: Connection-level failover within MC/S sessions

### Protocol Extensions

- **PEXT-01**: RFC 7144 task management (QUERY TASK, QUERY TASK SET, I_T NEXUS RESET, QUERY ASYNC EVENT)
- **PEXT-02**: iSCSIProtocolLevel negotiation (RFC 7144)

### Additional SCSI Commands

- **SCSI2-01**: MODE SELECT (6) and MODE SELECT (10)
- **SCSI2-02**: READ (6) and WRITE (6) for legacy target compatibility
- **SCSI2-03**: PREFETCH (10) and PREFETCH (16)
- **SCSI2-04**: SANITIZE
- **SCSI2-05**: EXTENDED COPY / RECEIVE COPY RESULTS (third-party copy)
- **SCSI2-06**: READ DEFECT DATA (10, 12)
- **SCSI2-07**: PREVENT ALLOW MEDIUM REMOVAL

### Discovery

- **DISC2-01**: iSNS discovery client

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| iSER (RDMA transport) | Different transport layer; requires RDMA hardware; undermines pure-userspace value |
| Kernel block device emulation (/dev/sdX) | Defeats pure-userspace purpose; requires NBD/TCMU/FUSE kernel modules |
| Boot from iSCSI | Firmware/bootloader concern, not a library feature |
| IPsec integration | Network-layer concern; OS configures IPsec, library uses whatever TCP connection it gets |
| Automatic LUN scanning/device management | Opinionated policy doesn't belong in a library; provide building blocks instead |
| Built-in retry/reconnection policy | Application-specific; provide ERL mechanisms and hooks, optional default helper |

## Traceability

Which phases cover which requirements. Updated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| PDU-01 | Phase 1 | Complete |
| PDU-02 | Phase 1 | Complete |
| PDU-03 | Phase 1 | Complete |
| PDU-04 | Phase 1 | Complete |
| XPORT-01 | Phase 1 | Complete |
| XPORT-02 | Phase 1 | Complete |
| XPORT-03 | Phase 1 | Complete |
| XPORT-04 | Phase 1 | Complete |
| LOGIN-01 | Phase 2 | Pending |
| LOGIN-02 | Phase 2 | Pending |
| LOGIN-03 | Phase 2 | Pending |
| LOGIN-04 | Phase 2 | Complete |
| LOGIN-05 | Phase 2 | Complete |
| LOGIN-06 | Phase 2 | Pending |
| SESS-01 | Phase 3 | Pending |
| SESS-02 | Phase 3 | Pending |
| SESS-03 | Phase 3 | Pending |
| SESS-04 | Phase 3 | Pending |
| SESS-05 | Phase 3 | Pending |
| READ-01 | Phase 3 | Pending |
| READ-02 | Phase 3 | Pending |
| READ-03 | Phase 3 | Pending |
| WRITE-01 | Phase 4 | Pending |
| WRITE-02 | Phase 4 | Pending |
| WRITE-03 | Phase 4 | Pending |
| WRITE-04 | Phase 4 | Pending |
| WRITE-05 | Phase 4 | Pending |
| INTEG-01 | Phase 2 | Pending |
| INTEG-02 | Phase 2 | Pending |
| INTEG-03 | Phase 2 | Pending |
| TMF-01 | Phase 6 | Pending |
| TMF-02 | Phase 6 | Pending |
| TMF-03 | Phase 6 | Pending |
| TMF-04 | Phase 6 | Pending |
| TMF-05 | Phase 6 | Pending |
| TMF-06 | Phase 6 | Pending |
| ERL-01 | Phase 6 | Pending |
| ERL-02 | Phase 6 | Pending |
| ERL-03 | Phase 6 | Pending |
| EVT-01 | Phase 3 | Pending |
| EVT-02 | Phase 3 | Pending |
| EVT-03 | Phase 3 | Pending |
| DISC-01 | Phase 3 | Pending |
| DISC-02 | Phase 3 | Pending |
| SCSI-01 | Phase 5 | Pending |
| SCSI-02 | Phase 5 | Pending |
| SCSI-03 | Phase 5 | Pending |
| SCSI-04 | Phase 5 | Pending |
| SCSI-05 | Phase 5 | Pending |
| SCSI-06 | Phase 5 | Pending |
| SCSI-07 | Phase 5 | Pending |
| SCSI-08 | Phase 5 | Pending |
| SCSI-09 | Phase 5 | Pending |
| SCSI-10 | Phase 5 | Pending |
| SCSI-11 | Phase 5 | Pending |
| SCSI-12 | Phase 5 | Pending |
| SCSI-13 | Phase 5 | Pending |
| SCSI-14 | Phase 5 | Pending |
| SCSI-15 | Phase 5 | Pending |
| SCSI-16 | Phase 5 | Pending |
| SCSI-17 | Phase 5 | Pending |
| SCSI-18 | Phase 5 | Pending |
| SCSI-19 | Phase 5 | Pending |
| API-01 | Phase 7 | Pending |
| API-02 | Phase 7 | Pending |
| API-03 | Phase 7 | Pending |
| API-04 | Phase 7 | Pending |
| API-05 | Phase 7 | Pending |
| OBS-01 | Phase 7 | Pending |
| OBS-02 | Phase 7 | Pending |
| OBS-03 | Phase 7 | Pending |
| TEST-01 | Phase 7 | Pending |
| TEST-02 | Phase 7 | Pending |
| TEST-03 | Phase 1 | Complete |
| TEST-04 | Phase 2 | Pending |
| TEST-05 | Phase 6 | Pending |
| DOC-01 | Phase 7 | Pending |
| DOC-02 | Phase 7 | Pending |
| DOC-03 | Phase 7 | Pending |
| DOC-04 | Phase 7 | Pending |
| DOC-05 | Phase 7 | Pending |

**Coverage:**
- v1 requirements: 81 total
- Mapped to phases: 81
- Unmapped: 0

| Phase | Count | Categories |
|-------|-------|------------|
| Phase 1 | 9 | PDU (4), XPORT (4), TEST (1) |
| Phase 2 | 10 | LOGIN (6), INTEG (3), TEST (1) |
| Phase 3 | 13 | SESS (5), READ (3), EVT (3), DISC (2) |
| Phase 4 | 5 | WRITE (5) |
| Phase 5 | 19 | SCSI (19) |
| Phase 6 | 10 | ERL (3), TMF (6), TEST (1) |
| Phase 7 | 15 | API (5), OBS (3), TEST (2), DOC (5) |

---
*Requirements defined: 2026-03-31*
*Last updated: 2026-03-31 after roadmap creation*
