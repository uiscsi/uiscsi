# Milestones

## v1.1.0 Full Test Compliance and Coverage (Shipped: 2026-04-05)

**Phases completed:** 7 phases, 19 plans, 34 tasks

**Key accomplishments:**

- PDU capture Recorder with typed decode via DecodeBHS, MockTarget HandleSCSIFunc with atomic call counter, and SessionState for stateful CmdSN/MaxCmdSN tracking
- Three wire-level CmdSN conformance tests proving sequential increment for SCSI commands and Immediate delivery (no CmdSN advance) for NOP-Out and TMF
- MockTarget extended with NegotiationConfig, multi-PDU Data-In, R2T sequence generation, and Data-Out receive for Phase 14 conformance tests
- Commit:
- 4 R2T wire conformance tests validating TTT echo, DataSN progression, per-burst reset, F-bit placement, and per-command ITT/TTT isolation
- Extracted 5 reusable write-test helpers into helpers_test.go to eliminate setup boilerplate for SCSI command wire conformance tests
- All 7 SCSI Command PDU wire conformance tests (SCSI-01 through SCSI-07) covering ImmediateData/InitialR2T/FirstBurstLength matrix with W-bit, F-bit, EDTL, and DataSegmentLength assertions
- HandleSCSIWithStatus helper and 6 ERR conformance tests covering BUSY, RESERVATION CONFLICT, CHECK CONDITION sense parsing, and SNACK Reject task cancellation with retry
- Data/R2T SNACK on DataSN gap (Type=0, BegRun=1, RunLength=1) and DataACK SNACK on A-bit (Type=2, BegRun=2, RunLength=0, TTT=0x00000042) verified at wire level
- MockTarget async injection with SendAsyncMsg/HandleText, LUN echo fix, Parameter3 deadline, renegotiation handler, plus SESS-03/SESS-04 NOP-Out wire field conformance tests
- SESS-05 ExpStatSN NOP-Out confirmation, SESS-01 async logout within Parameter3, and SESS-06 clean logout exchange -- all validated on wire via PDU capture
- Four async event conformance tests (ASYNC-01 through ASYNC-04) validating logout timing, connection drop, session drop, and TextReq renegotiation with wire-level PDU assertions
- 4 command window conformance tests (zero/large/size-1/MaxCmdSN close) with cmdWindow bug fixes for RFC 7143 flow control compliance
- Reject-reissue and SNACK timer tail loss conformance tests proving ERL=1 command retry and status recovery behavior
- ERL dispatch in triggerReconnect with conformance tests proving Logout(reasonCode=2) and TMF TASK REASSIGN(Function=14) on wire after ERL 2 connection replacement
- Same-connection retry on Reject at ERL>=1 using original ITT, CDB, CmdSN per RFC 7143 Section 6.2.1
- 6 TMF wire-level conformance tests verifying CmdSN immediate handling, LUN encoding, RefCmdSN, and AbortTaskSet behavior with non-blocking goroutine stall pattern
- 6 Text Request conformance tests validating opcode, F-bit, ITT uniqueness, TTT handling (initial and continuation), CmdSN/ExpStatSN, and negotiation reset per RFC 7143

---

## v1.0 v1.0 (Shipped: 2026-04-03)

**Phases completed:** 12 phases, 38 plans, 77 tasks

**Key accomplishments:**

- RFC 1982 serial arithmetic with wrap-around, CRC32C digest with padding-inclusive DataDigest, and 4-byte PadLen helper -- all stdlib-only, all passing under -race
- Complete iSCSI PDU codec with all 18 opcode types (8 initiator, 10 target), BHS marshal/unmarshal, AHS support, and 30+ round-trip tests passing under -race
- PDU framing over TCP with io.ReadFull, concurrent read/write pump goroutines, ITT-based response routing skipping reserved 0xFFFFFFFF, and sync.Pool buffer management -- 27 tests passing under -race
- Text key-value codec, declarative negotiation engine for all 14 RFC 7143 Section 13 mandatory keys, typed NegotiatedParams, and LoginError with status codes
- CHAP authentication with MD5 response computation, hex/base64 binary encoding, and mutual CHAP verification using constant-time comparison
- Login state machine with functional options API, CHAP/mutual-CHAP auth, digest negotiation, and mock target test harness with 10 integration tests
- Session layer with CmdSN windowing, async SCSI command dispatch via Submit+Channel, and multi-PDU Data-In reassembly with DataSN/BufferOffset validation
- NOP-Out/NOP-In keepalive with timeout detection, async message dispatch to user callbacks, and graceful Logout PDU exchange per RFC 7143
- SendTargets discovery with C-bit continuation, Discover convenience function (Dial+Login+SendTargets+Logout), and IPv4/IPv6 portal parsing
- Command type uses io.Reader for write data with auto W-bit detection and immediate data reading from reader
- R2T-driven solicited Data-Out with MaxBurstLength enforcement and unsolicited Data-Out when InitialR2T=No, using on-demand io.Reader consumption
- Parameterized 2x2 ImmediateData x InitialR2T matrix test plus multi-R2T sequence, small data, and burst boundary edge case tests
- Mock target responds to Logout PDUs — session test suite 142s → 7s with zero production code changes
- internal/scsi/ package with sense parsing (fixed+descriptor), 14 SenseKey values, CommandError, and 8 CDB builders for TUR/INQUIRY/RC10/RC16/REQUEST SENSE/REPORT LUNS/MODE SENSE 6+10
- READ/WRITE 10/16 CDB builders with FUA/DPO options and 6 VPD page parsers (0x00, 0x80, 0x83, 0xB0, 0xB1, 0xB2) for device identification and capability discovery
- Cache, provisioning, reservations, verify, compare-and-write, and start-stop-unit CDB builders with parameter data serialization for UNMAP and PR OUT
- All six RFC 7143 task management functions with auto-cleanup plus faultConn deterministic error injection utility
- Automatic session reinstatement with same ISID+TSIH after connection failure, transparent in-flight command retry with exponential backoff
- SNACK-based PDU retransmission for within-connection recovery plus ERL 2 connection replacement with Logout reasonCode=2 and TMF TASK REASSIGN
- DigestError type with CRC32C verification in ReadRawPDU plus compact String() methods on all 18 PDU types for wire-level debugging
- PDU hook callbacks and metrics events with injected logger, eliminating global slog in transport pumps
- Structured slog lifecycle logging for session/login stages, DigestError connection-fatal behavior per D-03, and ITT/CmdSN-enriched error messages
- Complete public API surface with Dial/Discover, 20+ Session methods, typed error hierarchy, streaming I/O, and observability options
- Mock iSCSI target with handler-based PDU dispatch and 20 IOL-inspired conformance tests covering login, SCSI read/write/inquiry, error recovery, and task management
- Godoc testable examples for 7 API functions, four standalone example programs, and README with quick start
- SCSI device type table, columnar tabwriter formatter, and JSON formatter with SI capacity display for lsscsi-style discovery output
- Full probe pipeline (Discover->Dial->ReportLuns->Inquiry->ReadCapacity) with CLI flag parsing, signal handling, and exit codes
- LIO configfs helper package with Setup/Teardown/SweepOrphans and basic connectivity E2E test against real kernel iSCSI target
- 6 E2E test files covering data integrity, CHAP auth, CRC32C digests, multi-LUN enumeration, TMF LUNReset, and ERL 0 connection recovery against real LIO kernel target
- WithOperationalOverrides login option enabling 1MB multi-R2T data transfer and 2x2 ImmediateData x InitialR2T negotiation matrix E2E tests against real LIO target
- Header-only and data-only CRC32C digest modes tested with write+read cycles, SCSI error handling verified with out-of-range LBA producing ILLEGAL_REQUEST sense data
- ABORT TASK/TARGET WARM RESET TMF tests with PDU hook ITT capture, plus ERL 1/2 best-effort negotiation tests against real LIO target
- Fixed OpReject PDU handling (both dispatch paths) and SCSI Response SenseLength prefix stripping per RFC 7143 Section 11.4.7.2
- Fixed negotiation matrix and AbortTask TMF tests to handle Reject errors and accept all valid RFC 7143 response codes
- MaxRecvDSL enforcement prevents memory exhaustion DoS, CHAP returns errors instead of panicking, non-auth login errors correctly classified as TransportError
- Residual overflow detection, CDB validation, safe backoff, improved error messages, PDU encoding validation, and AHS type checking
- Goroutine-safe writeCh access, blocking SNACK delivery with timeout, and correct ITT lifecycle during ERL 2 connection replacement
- Configurable digest byte order, context-aware callbacks, and instrumented mock target

---
