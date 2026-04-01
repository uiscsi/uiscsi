# Phase 3: Session, Read Path, and Discovery - Context

**Gathered:** 2026-04-01
**Status:** Ready for planning

<domain>
## Phase Boundary

Build the session layer on top of authenticated connections: CmdSN/MaxCmdSN command windowing, SCSI command dispatch with Data-In reassembly, NOP-Out/NOP-In keepalive, SendTargets discovery, graceful logout, and async message handling. Does not include the write path (Data-Out, R2T handling) or error recovery levels 1-2 -- those belong in Phase 4.

</domain>

<decisions>
## Implementation Decisions

### Session API Shape
- **D-01:** Session wraps Conn -- `NewSession(conn, params, opts...)` takes ownership of the transport.Conn and NegotiatedParams. Caller does Dial -> Login -> NewSession as three distinct steps, consistent with D-02 from Phase 2.
- **D-02:** NegotiatedParams exposed via read-only accessor `session.Params()` -- callers can inspect negotiated values (MaxBurstLength, digests, etc.) but not modify them.
- **D-03:** NewSession auto-starts ReadPump/WritePump -- session is ready for commands immediately after creation. No separate Start() call needed.

### Command Dispatch Model
- **D-04:** Async Submit+Channel model -- `session.Submit(ctx, cmd)` returns a `<-chan Result` (or similar). Callers can have multiple commands in flight up to the CmdSN window. CmdSN/MaxCmdSN windowing handled internally.
- **D-05:** Data-In delivered as `io.Reader` -- streaming assembly of multi-PDU reads. Higher-level APIs (ReadBlocks, Inquiry in future phases) consume the Reader internally and return typed results. Lower memory pressure for large transfers, Go-idiomatic.

### Discovery Integration
- **D-06:** Both standalone function and Session method -- `Discover(ctx, addr, opts...)` convenience function (Dial+Login+SendTargets+Logout in one call) and `session.SendTargets(ctx)` for power users with existing discovery sessions.
- **D-07:** Structured `DiscoveryTarget` return type -- `type DiscoveryTarget struct { Name string; Portals []Portal }` with `Portal` containing Address and Port. Parsed from text key-value response.

### Keepalive and Async Events
- **D-08:** Automatic background keepalive -- Session runs a goroutine sending NOP-Out at configurable interval (default 30s). Timeout triggers session error. Also auto-responds to unsolicited target NOP-In. No caller action needed.
- **D-09:** Async events via callback -- `WithAsyncHandler(func(AsyncEvent))` functional option. Called on dedicated goroutine when target sends AsyncMsg PDU.
- **D-10:** Target-requested logout triggers auto-logout + notify -- Session initiates graceful logout automatically per RFC 7143 Time2Wait+Time2Retain, then calls async handler to inform caller.

### Claude's Discretion
- Internal session state machine design
- CmdSN window tracking data structure (ring buffer, sync.Cond, semaphore, etc.)
- Data-In reassembly buffer management
- How the Result type is structured (struct with Reader + Status + SenseData, etc.)
- Internal package organization (internal/session/ vs extending internal/login/)
- NOP-Out TTT handling for unsolicited vs solicited NOP exchanges
- Logout PDU exchange sequencing
- SendTargets text response parsing implementation

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### iSCSI Session and Command Protocol
- RFC 7143 Section 4 -- Session types, session establishment, ISID/TSIH semantics
- RFC 7143 Section 3.2.2 -- Command numbering and acknowledging (CmdSN, MaxCmdSN, ExpCmdSN)
- RFC 7143 Section 4.2 -- Command ordering and numbering rules, command window
- RFC 7143 Section 11.2 -- SCSI Command PDU format (CDB, expected data transfer length, flags)
- RFC 7143 Section 11.3 -- SCSI Response PDU format (status, sense data, residual counts)
- RFC 7143 Section 11.7 -- Data-In PDU format (DataSN, offset, S-bit, residual handling)

### Keepalive and Async
- RFC 7143 Section 11.18 -- NOP-Out PDU format (ping, TTT)
- RFC 7143 Section 11.19 -- NOP-In PDU format (response to NOP-Out, unsolicited from target)
- RFC 7143 Section 11.9 -- Async Message PDU format (event codes, parameter handling)

### Discovery and Logout
- RFC 7143 Section 4.3 -- SendTargets discovery mechanism
- RFC 7143 Section 11.10 -- Text Request PDU (SendTargets=All)
- RFC 7143 Section 11.11 -- Text Response PDU
- RFC 7143 Section 11.14 -- Logout Request PDU (reason codes, connection vs session)
- RFC 7143 Section 11.15 -- Logout Response PDU (time2wait, time2retain)

### Existing Code
- `internal/transport/conn.go` -- Conn with ReadPump/WritePump, SetDigests, SetMaxRecvDSL
- `internal/transport/pump.go` -- ReadPump/WritePump full-duplex PDU framing
- `internal/transport/router.go` -- ITT-based response routing
- `internal/login/login.go` -- Login() function, NegotiatedParams, functional options pattern
- `internal/login/params.go` -- NegotiatedParams struct with all operational parameters
- `internal/login/textcodec.go` -- EncodeTextKV/DecodeTextKV for SendTargets text format
- `internal/pdu/initiator.go` -- SCSICommand, NOPOut, TextReq, LogoutReq PDU types
- `internal/pdu/target.go` -- SCSIResponse, DataIn, NOPIn, TextResp, LogoutResp, AsyncMsg PDU types
- `internal/serial/serial.go` -- InWindow, Incr for sequence number arithmetic

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `transport.Conn` with full-duplex pumps -- session layer sits on top, receives PDUs via Router
- `transport.Router` -- ITT-based dispatch, already handles PDU correlation. Session needs to register handlers for unsolicited PDUs (NOP-In, AsyncMsg)
- `login.NegotiatedParams` -- All negotiated operational parameters available (MaxBurstLength, InitialR2T, ImmediateData, MaxOutR2T, etc.)
- `login.EncodeTextKV/DecodeTextKV` -- Reuse for SendTargets text request/response encoding
- `serial.InWindow/Incr` -- Sequence number arithmetic for CmdSN/DataSN validation
- All relevant PDU types already implemented: SCSICommand, SCSIResponse, DataIn, NOPOut, NOPIn, TextReq, TextResp, LogoutReq, LogoutResp, AsyncMsg

### Established Patterns
- Functional options pattern for configuration (established in Phase 2 Login API)
- Typed PDU per opcode with embedded Header base struct
- io.ReadFull for all TCP reads, single WritePump goroutine for write serialization
- Table-driven tests with t.Run subtests, no testify
- Mock target goroutine pattern for integration testing (from login_test.go)

### Integration Points
- Session receives `*transport.Conn` after Login -- takes ownership, starts pumps
- NegotiatedParams feeds session configuration: MaxCmdSN window size, burst lengths, digest state
- Router.Register for unsolicited PDU handling (NOP-In, AsyncMsg)
- Login's text codec reused for SendTargets discovery data segment encoding/decoding

</code_context>

<specifics>
## Specific Ideas

- io.Reader for Data-In was chosen specifically for tape device compatibility -- sequential streaming is the natural model for variable-block tape reads
- The two-layer API (low-level raw CDB + high-level typed helpers) means this phase builds the low-level Submit+Channel layer; typed helpers come in a later phase
- Automatic keepalive with configurable interval matches how production iSCSI initiators behave (open-iscsi defaults to 5s, but 30s is more conservative for a library)

</specifics>

<deferred>
## Deferred Ideas

None -- discussion stayed within phase scope

</deferred>

---

*Phase: 03-session-read-path-and-discovery*
*Context gathered: 2026-04-01*
