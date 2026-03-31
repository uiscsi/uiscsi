# Phase 2: Connection and Login - Context

**Gathered:** 2026-03-31
**Status:** Ready for planning

<domain>
## Phase Boundary

Establish an authenticated iSCSI connection with full operational parameter negotiation. Covers the login state machine (security negotiation → operational negotiation → full feature phase transition), AuthMethod=None, CHAP, and mutual CHAP authentication, all RFC 7143 Section 13 mandatory key negotiation, and digest activation on the transport layer. Does not include session-level concerns (command windowing, keepalive, logout) — those belong in Phase 3.

</domain>

<decisions>
## Implementation Decisions

### Login API Shape
- **D-01:** Functional options pattern for login configuration — `conn.Login(ctx, WithTarget("iqn..."), WithCHAP(user, secret), WithHeaderDigest(CRC32C), ...)`
- **D-02:** Separate Dial + Login steps — keep `transport.Dial()` and `conn.Login()` as distinct operations. Caller controls connection lifecycle. No combined Connect() convenience in this phase.
- **D-03:** Login returns a session handle (or connection-level result) with access to negotiated parameters

### Negotiation Engine
- **D-04:** Declarative key registry — each RFC 7143 Section 13 key is a struct describing its type (BoolAnd, BoolOr, NumericMin, NumericMax, ListSelect), default value, valid range, and RFC reference. A generic engine processes all keys uniformly.
- **D-05:** Negotiated parameters stored in a typed `NegotiatedParams` struct with direct field access (HeaderDigest bool, DataDigest bool, MaxRecvDSL uint32, MaxBurstLen, FirstBurstLen, InitialR2T, ImmediateData, MaxOutR2T, etc.). Compile-time safe, no string map lookups at use sites.

### Credential Handling
- **D-06:** CHAP credentials provided via functional options — `WithCHAP(user, secret)` for one-way CHAP, `WithMutualCHAP(user, secret, targetSecret)` for bidirectional authentication. No callback interface or credential provider abstraction in v1.
- **D-07:** AuthMethod=None is the default when no CHAP options are provided

### Login Error Reporting
- **D-08:** Typed `LoginError` struct with StatusClass (uint8) and StatusDetail (uint8) fields mapping directly to RFC 7143 Section 11.13 login response status codes. Human-readable Message field. Callers inspect via `errors.As()`.
- **D-09:** StatusClass values: 0=success, 1=redirect, 2=initiator error, 3=target error — direct mapping from the spec

### Claude's Discretion
- Login state machine internal design (flat function vs explicit state type)
- How login PDU exchanges are sequenced internally (loop vs recursive steps)
- CHAP challenge/response crypto implementation details
- Text key-value encoding/decoding format details (key=value\0 pairs in data segment)
- Internal package organization for login code (internal/login/ vs internal/conn/)
- Whether NegotiatedParams is embedded in a connection/session type or returned standalone

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### iSCSI Login Protocol
- RFC 7143 Section 11.12 — Login Request PDU format (CSG/NSG, Transit bit, Continue bit, ISID, TSIH)
- RFC 7143 Section 11.13 — Login Response PDU format (status class/detail codes, redirect handling)
- RFC 7143 Section 6 — Login phase state machine (SecurityNegotiation, LoginOperationalNegotiation, FullFeaturePhase transitions)
- RFC 7143 Section 13 — Text key negotiation: all mandatory operational parameters, negotiation types (AND, OR, min, max, list), and default values

### Authentication
- RFC 7143 Section 12 — Security considerations, CHAP requirement
- RFC 1994 — PPP Challenge Handshake Authentication Protocol (CHAP) — defines the challenge/response algorithm used by iSCSI
- RFC 7143 Section 12.1 — Header and data digest negotiation (CRC32C)

### Project Research (from Phase 1)
- `.planning/research/STACK.md` — Technology choices, stdlib crypto/md5 and crypto/hmac for CHAP
- `.planning/research/ARCHITECTURE.md` — Component boundaries, login layer placement
- `.planning/research/PITFALLS.md` — Login-related pitfalls (if any)

### Existing Code
- `internal/pdu/initiator.go` — LoginReq PDU type with CSG/NSG bit packing
- `internal/pdu/target.go` — LoginResp PDU type with StatusClass/StatusDetail
- `internal/transport/conn.go` — Conn with SetDigests(), SetMaxRecvDSL() hooks
- `internal/transport/pump.go` — ReadPump/WritePump for PDU framing
- `internal/transport/router.go` — ITT-based response routing

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `pdu.LoginReq` / `pdu.LoginResp` — Already implemented with full BHS marshal/unmarshal, CSG/NSG bit packing, Transit/Continue flags, ISID/TSIH fields
- `transport.Conn` — Has Dial(), SetDigests(), SetMaxRecvDSL() — login outcome directly feeds these setters
- `transport.WritePump` / `transport.ReadPump` — Full-duplex PDU framing already working
- `transport.Router` — ITT-based dispatch for correlating login responses to requests
- `digest.HeaderDigest` / `digest.DataDigest` — CRC32C computation ready for activation post-negotiation

### Established Patterns
- Typed PDU per opcode with embedded Header — login code should use LoginReq/LoginResp directly
- io.ReadFull for all TCP reads, single WritePump for write serialization
- Copy-out ownership model for PDU data segments
- Table-driven tests with t.Run subtests
- Internal packages under internal/ for implementation hiding

### Integration Points
- Login code consumes transport.Conn (Dial already done) and produces negotiated session state
- After login, Conn.SetDigests() and Conn.SetMaxRecvDSL() are called with negotiated values
- NegotiatedParams feeds Phase 3 (session layer command windowing) and Phase 4 (write path burst lengths)
- LoginReq.Data / LoginResp.Data carry text key=value\0 negotiation pairs

</code_context>

<specifics>
## Specific Ideas

- Follow The Bronx Method: standards compliance non-negotiable, minimal ceremony
- Declarative key registry should be easily testable with parameterized tests covering the full negotiation matrix (TEST-04 requirement)
- CHAP uses crypto/md5 and crypto/hmac from stdlib — no external dependency needed
- LoginError should feel natural to Go developers familiar with errors.As() pattern

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 02-connection-and-login*
*Context gathered: 2026-03-31*
