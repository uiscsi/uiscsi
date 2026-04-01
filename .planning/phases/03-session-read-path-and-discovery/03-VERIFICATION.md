---
phase: 03-session-read-path-and-discovery
verified: 2026-04-01T08:00:00Z
status: passed
score: 13/13 must-haves verified
re_verification: false
---

# Phase 03: Session Read Path and Discovery Verification Report

**Phase Goal:** Session lifecycle, SCSI read command path, and target discovery
**Verified:** 2026-04-01T08:00:00Z
**Status:** PASSED
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Session can be created from a Conn and NegotiatedParams | VERIFIED | `NewSession(conn *transport.Conn, params login.NegotiatedParams, opts ...SessionOption)` in session.go:47; auto-starts readPumpLoop, writePumpLoop, dispatchLoop, keepaliveLoop goroutines |
| 2 | CmdSN window throttles Submit when full and unblocks when MaxCmdSN advances | VERIFIED | `cmdwindow.go` uses `serial.InWindow` for gating and `sync.Cond` for blocking/wakeup; 7 tests including `TestCmdWindowBlocks`, `TestCmdWindowContextCancel`, `TestCmdWindowWrapAround` all pass |
| 3 | SCSI read command returns correct data assembled from multiple Data-In PDUs via io.Reader | VERIFIED | `datain.go` accumulates into `bytes.Buffer`, delivers `bytes.NewReader` in Result; `TestSessionSubmitMultiPDURead` passes with 3 Data-In + SCSIResponse |
| 4 | StatSN/ExpStatSN tracks correctly from every response PDU | VERIFIED | `updateStatSN()` in session.go:201 called from taskLoop on both DataIn (S-bit) and SCSIResponse; `TestSessionStatSNTracking` passes |
| 5 | Status delivered via S-bit Data-In or separate SCSIResponse PDU | VERIFIED | `task.handleDataIn` handles `HasStatus` S-bit (datain.go:52); `task.handleSCSIResponse` handles separate response (datain.go:72); both test paths pass |
| 6 | Session sends periodic NOP-Out pings and detects timeout if NOP-In response is missing | VERIFIED | `keepaliveLoop` in keepalive.go uses ticker + router.Register; `TestKeepaliveTimeout` confirms timeout detection |
| 7 | Session responds to target-initiated NOP-In with NOP-Out echoing TTT | VERIFIED | `handleUnsolicitedNOPIn` in keepalive.go echoes `TargetTransferTag != 0xFFFFFFFF`; `TestUnsolicitedNOPInResponse` passes |
| 8 | Async messages from target are dispatched to the user-provided callback | VERIFIED | `handleAsyncMsg` in async.go switches on EventCode 0-4 and calls asyncHandler; `TestAsyncEventCallback` passes |
| 9 | Target-requested logout triggers auto-logout and notifies caller | VERIFIED | EventCode 1 in async.go:38 spawns `handleTargetRequestedLogout` goroutine with DefaultTime2Wait delay |
| 10 | Graceful logout tears down session cleanly with Logout PDU exchange | VERIFIED | `logout.go` sends `pdu.LogoutReq`, waits for `pdu.LogoutResp`; `TestLogoutGraceful` passes; `TestCloseWithLogout` confirms `Close()` triggers logout |
| 11 | SendTargets discovery enumerates available targets with their portal addresses | VERIFIED | `SendTargets()` in discovery.go sends TextReq with `SendTargets=All`, parses TextResp via `parseSendTargetsResponse`; `TestSendTargetsSingleResponse` passes |
| 12 | Multi-PDU text responses with C-bit continuation are handled correctly | VERIFIED | discovery.go:108-131 loops on `textResp.Continue` flag, sends continuation TextReq with target's TTT; `TestSendTargetsContinuation` passes |
| 13 | Discover convenience function performs full Dial+Login+SendTargets+Logout | VERIFIED | `Discover()` in discovery.go:149 does `transport.Dial` -> `login.Login(WithSessionType("Discovery"))` -> `NewSession` -> `SendTargets` -> `Logout`; `TestDiscoverIntegration` passes |

**Score:** 13/13 truths verified

---

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/session/types.go` | Result, Command, AsyncEvent, DiscoveryTarget, Portal, SessionOption types | VERIFIED | All 5 exported types + SessionOption + sessionConfig present |
| `internal/session/cmdwindow.go` | CmdSN/MaxCmdSN window gating with sync.Cond | VERIFIED | 120 lines; `serial.InWindow`, `serial.Incr`, `serial.GreaterThan` all used |
| `internal/session/datain.go` | Data-In reassembly with bytes.Buffer streaming | VERIFIED | Uses `bytes.Buffer` (deviation from planned `io.Pipe`; avoids deadlock); `handleDataIn`, `handleSCSIResponse` present |
| `internal/session/session.go` | Session struct, NewSession, Submit, dispatchLoop | VERIFIED | All exports present; 4 background goroutines started; `expStatSN` tracked under mutex |
| `internal/session/keepalive.go` | NOP-Out/NOP-In keepalive goroutine | VERIFIED | `keepaliveLoop`, `handleUnsolicitedNOPIn`; `0xFFFFFFFF` reserved TTT used correctly |
| `internal/session/async.go` | AsyncMsg dispatch and target-requested logout handling | VERIFIED | `handleAsyncMsg`, `handleTargetRequestedLogout`; event codes 0-4 handled |
| `internal/session/logout.go` | Logout PDU exchange, Close integration | VERIFIED | `logout()` private + `Logout()` public + `LogoutConnection()`; `Close()` in session.go attempts graceful logout |
| `internal/session/discovery.go` | SendTargets method, Discover function, response parsing | VERIFIED | `SendTargets()`, `Discover()`, `parseSendTargetsResponse()`, `parsePortal()` all present |

---

### Key Link Verification

Note: gsd-tools pattern matcher returned false negatives on all cross-package links (it searches file paths, not file contents). All links were manually verified by `grep` against actual file contents.

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `session.go` | `transport/conn.go` | `*transport.Conn` field and `NewSession` param | WIRED | session.go:22 `conn *transport.Conn`; session.go:47 `NewSession(conn *transport.Conn, ...)` |
| `session.go` | `login/params.go` | `login.NegotiatedParams` in NewSession and Params() | WIRED | session.go:23 `params login.NegotiatedParams`; session.go:81 `Params() login.NegotiatedParams` |
| `datain.go` | `pdu/target.go` | `*pdu.DataIn` and `*pdu.SCSIResponse` in handlers | WIRED | datain.go:39 `handleDataIn(din *pdu.DataIn)`; datain.go:72 `handleSCSIResponse(resp *pdu.SCSIResponse)` |
| `session.go` | `serial/serial.go` | `serial.InWindow` for CmdSN window check (in cmdwindow.go) | WIRED | cmdwindow.go:49 `serial.InWindow`; cmdwindow.go:74 `serial.Incr`; cmdwindow.go:103 `serial.LessThan` |
| `keepalive.go` | `transport/router.go` | `s.router.Register()` for NOP-Out ITT | WIRED | keepalive.go:49 `s.router.Register()` |
| `keepalive.go` | `session.go` | `s.writeCh` for NOP-Out PDUs | WIRED | keepalive.go uses `s.writeCh` |
| `async.go` | `types.go` | `AsyncEvent` struct | WIRED | async.go:25 `AsyncEvent{EventCode: async.AsyncEvent, ...}` |
| `logout.go` | `pdu/initiator.go` | `pdu.LogoutReq` PDU | WIRED | logout.go:25 `&pdu.LogoutReq{...}` |
| `discovery.go` | `login/textcodec.go` | `login.DecodeTextKV` for response parsing | WIRED | discovery.go:193 `login.DecodeTextKV(data)` |
| `discovery.go` | `login/login.go` | `login.Login(WithSessionType("Discovery"))` | WIRED | discovery.go:158,162 |
| `discovery.go` | `types.go` | `DiscoveryTarget` and `Portal` types | WIRED | discovery.go uses both types throughout |

---

### Data-Flow Trace (Level 4)

All artifacts render in-memory data constructed from decoded PDUs over `net.Pipe()` mock connections in tests. No external data sources exist that could be hollow.

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|--------------|--------|-------------------|--------|
| `datain.go` Result.Data | `t.buf` (bytes.Buffer) | PDU DataSegment bytes written in `handleDataIn` | Yes — verified by multi-PDU tests confirming correct byte concatenation | FLOWING |
| `session.go` expStatSN | `s.expStatSN` | `updateStatSN(statSN)` called on every DataIn/SCSIResponse PDU | Yes — `TestSessionStatSNTracking` explicitly verifies advancement | FLOWING |
| `discovery.go` targets | `parseSendTargetsResponse(data)` | `login.DecodeTextKV` on accumulated TextResp data segment | Yes — `TestSendTargetsContinuation` verifies multi-PDU accumulation | FLOWING |

---

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| All session tests pass with -race | `go test ./internal/session/ -race -count=1` | ok 71.998s | PASS |
| Transport tests pass with -race (RegisterPersistent wired) | `go test ./internal/transport/ -race -count=1` | ok 1.039s | PASS |
| Login tests pass with -race (CmdSN/ExpStatSN handoff) | `go test ./internal/login/ -race -count=1` | ok 1.261s | PASS |
| Full test suite passes | `go test ./... -race -count=1` | all 6 packages ok | PASS |
| go vet clean | `go vet ./internal/session/ ./internal/transport/ ./internal/login/` | no output | PASS |

---

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|------------|-------------|--------|----------|
| SESS-01 | 03-01 | Session state machine per RFC 7143 connection/session model | SATISFIED | `NewSession` wraps `transport.Conn` after login; session tracks loggedIn state; `Close()` handles teardown |
| SESS-02 | 03-01 | CmdSN/ExpCmdSN/MaxCmdSN command windowing and flow control | SATISFIED | `cmdwindow.go` with `serial.InWindow` gating; 7 tests including blocking/unblocking/wrap-around |
| SESS-03 | 03-01 | StatSN/ExpStatSN tracking per connection | SATISFIED | `updateStatSN` in session.go:201; called from taskLoop on every response PDU; `TestSessionStatSNTracking` |
| SESS-04 | 03-01 | SCSI Command PDU generation with proper CDB encapsulation | SATISFIED | `Submit()` builds `pdu.SCSICommand` with CDB, flags, CmdSN, ExpStatSN; `TestSessionSubmitRead` |
| SESS-05 | 03-02 | NOP-Out/NOP-In keepalive (initiator-originated and target-initiated response) | SATISFIED | `keepaliveLoop` for initiated pings; `handleUnsolicitedNOPIn` for target-initiated echoes; 4 keepalive tests |
| READ-01 | 03-01 | Data-In PDU handling with sequence number validation and data offset tracking | SATISFIED | `handleDataIn` validates `DataSN == nextDataSN` and `BufferOffset == nextOffset`; `TestTaskDataSNGap`, `TestTaskOffsetMismatch` |
| READ-02 | 03-01 | Multi-PDU read reassembly (gathering Data-In PDUs into complete read response) | SATISFIED | `bytes.Buffer` accumulation in `task`; `TestSessionSubmitMultiPDURead` with 3 Data-In PDUs |
| READ-03 | 03-01 | Status delivery via Data-In with S-bit or separate SCSI Response PDU | SATISFIED | S-bit path in `handleDataIn`; SCSIResponse path in `handleSCSIResponse`; both covered by tests |
| EVT-01 | 03-02 | Async message handling (SCSI async event, target-requested logout, connection/session drop, vendor-specific) | SATISFIED | `handleAsyncMsg` switch on EventCode 0-4; `asyncHandler` callback dispatch; `TestAsyncEventCallback` |
| EVT-02 | 03-02 | Logout (normal session/connection teardown) | SATISFIED | `Logout()` in logout.go exchanges LogoutReq/LogoutResp; `TestLogoutGraceful`; `TestCloseWithLogout` |
| EVT-03 | 03-02 | Logout for connection recovery (remove connection for recovery) | SATISFIED | `LogoutConnection(ctx, reasonCode uint8)` with reason code 2; `TestLogoutReasonCode2` |
| DISC-01 | 03-03 | SendTargets discovery (discovery session type, text request/response for target enumeration) | SATISFIED | `SendTargets()` sends TextReq `SendTargets=All`; handles C-bit continuation; `TestSendTargetsSingleResponse`, `TestSendTargetsContinuation` |
| DISC-02 | 03-03 | Target and LUN enumeration from discovery results | SATISFIED | `parseSendTargetsResponse` parses TargetName/TargetAddress into `DiscoveryTarget{Name, Portals}`; `TestParseSendTargetsResponse` with 8 sub-cases |

All 13 requirement IDs from PLAN frontmatter accounted for. No orphaned requirements found.

---

### Anti-Patterns Found

No anti-patterns detected. Scanned all session package `.go` files for:
- TODO/FIXME/XXX/HACK/PLACEHOLDER markers — none found
- Empty return stubs (`return null`, `return []`, `return {}`) — none that affect user-visible output
- Hardcoded empty data replacing real implementation — none

Notable deviation documented in SUMMARY (not a stub): `io.Pipe` replaced with `bytes.Buffer` in datain.go to prevent deadlock. This is a correctness improvement, not a regression.

---

### Human Verification Required

None. All behaviors are verified programmatically through tests that exercise the actual protocol logic over `net.Pipe()` mock connections.

---

### Gaps Summary

No gaps. All 13 observable truths verified, all 8 artifacts pass all levels (exists, substantive, wired, data-flowing), all key links confirmed wired via manual grep, all requirements satisfied, full test suite passes with -race flag.

---

_Verified: 2026-04-01T08:00:00Z_
_Verifier: Claude (gsd-verifier)_
