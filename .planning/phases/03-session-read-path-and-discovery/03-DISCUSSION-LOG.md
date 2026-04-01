# Phase 3: Session, Read Path, and Discovery - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md -- this log preserves the alternatives considered.

**Date:** 2026-04-01
**Phase:** 03-session-read-path-and-discovery
**Areas discussed:** Session API shape, Command dispatch model, Discovery integration, Keepalive and async events

---

## Session API Shape

| Option | Description | Selected |
|--------|-------------|----------|
| Session wraps Conn | Login returns NegotiatedParams, caller passes Conn + params to NewSession(). Session owns Conn lifecycle from that point. | ✓ |
| Login returns Session directly | Login() returns *Session that internally wraps Conn. Simpler but couples login and session. | |
| You decide | Claude picks based on existing Dial/Login separation. | |

**User's choice:** Session wraps Conn
**Notes:** Consistent with D-02 from Phase 2 (separate Dial + Login steps). Three-step flow: Dial -> Login -> NewSession.

| Option | Description | Selected |
|--------|-------------|----------|
| Read-only accessor | Session.Params() returns NegotiatedParams. Callers can inspect but not modify. | ✓ |
| Embed in Session | Public field, direct access but mutable. | |
| You decide | | |

**User's choice:** Read-only accessor

| Option | Description | Selected |
|--------|-------------|----------|
| Auto-start on NewSession | Pumps start immediately, session ready for commands. | ✓ |
| Explicit Start() | Caller calls session.Start() to begin pumps. | |
| You decide | | |

**User's choice:** Auto-start on NewSession

---

## Command Dispatch Model

| Option | Description | Selected |
|--------|-------------|----------|
| Synchronous Send+Wait | ExecuteCommand blocks until complete. Simple API. | |
| Async Submit+Channel | Submit returns chan Result. Multiple in flight. | ✓ |
| Both layers | Low-level async + sync wrapper. | |
| You decide | | |

**User's choice:** Async Submit+Channel

| Option | Description | Selected |
|--------|-------------|----------|
| Assembled buffer | Reassemble all Data-In PDUs into single []byte. | |
| Streaming io.Reader | Data-In as io.Reader, streams as PDUs arrive. | ✓ |
| You decide | | |

**User's choice:** io.Reader (streaming)
**Notes:** User asked about impact on higher layers, specifically tape device drivers. Claude explained io.Reader is strictly more flexible -- tape's sequential streaming model maps naturally. Higher-level APIs consume Reader internally.

---

## Discovery Integration

| Option | Description | Selected |
|--------|-------------|----------|
| Standalone function | Discover(ctx, addr, opts...) does everything in one call. | |
| Session method | session.SendTargets() on existing discovery session. | |
| Both | Standalone convenience + session method for power users. | ✓ |

**User's choice:** Both

| Option | Description | Selected |
|--------|-------------|----------|
| Structured DiscoveryTarget | Typed struct with Name and Portals. | ✓ |
| Raw text key-values | Return []KeyValue, caller parses. | |
| You decide | | |

**User's choice:** Structured DiscoveryTarget

---

## Keepalive and Async Events

| Option | Description | Selected |
|--------|-------------|----------|
| Automatic background | Goroutine sends NOP-Out at configurable interval. | ✓ |
| Caller-driven | session.Ping(ctx) manual. | |
| Both (configurable) | WithKeepalive enables auto, otherwise manual Ping(). | |

**User's choice:** Automatic background

| Option | Description | Selected |
|--------|-------------|----------|
| Callback function | WithAsyncHandler(func(AsyncEvent)) option. | ✓ |
| Channel | AsyncEvents() <-chan AsyncEvent. | |
| You decide | | |

**User's choice:** Callback function

| Option | Description | Selected |
|--------|-------------|----------|
| Auto-logout + notify | Session auto-logouts per RFC, then calls handler. | ✓ |
| Notify only | Call handler, let caller decide. | |
| You decide | | |

**User's choice:** Auto-logout + notify

---

## Claude's Discretion

- Internal session state machine design
- CmdSN window tracking data structure
- Data-In reassembly buffer management
- Result type structure
- Internal package organization
- NOP-Out TTT handling
- Logout PDU exchange sequencing
- SendTargets response parsing implementation

## Deferred Ideas

None -- discussion stayed within phase scope
