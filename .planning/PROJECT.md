# uiscsi

## What This Is

A pure-userspace iSCSI initiator library written in Go. It handles TCP connection, iSCSI login negotiation, and SCSI CDB transport over iSCSI PDUs entirely in userspace — no kernel SCSI stack, no iscsi-initiator-utils, no open-iscsi. Go applications import the library and talk directly to iSCSI targets.

## Core Value

Full RFC 7143 compliance as a composable Go library — the spec is non-negotiable, everything else is secondary.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] SendTargets discovery to enumerate available targets and LUNs
- [ ] Full iSCSI login phase: leading login, authentication, operational negotiation
- [ ] Authentication: None, CHAP, and mutual CHAP
- [ ] iSCSI full feature phase: SCSI command PDUs, data-in/out, R2T handling
- [ ] Error recovery level 0 (session-level reconnection)
- [ ] Error recovery level 1 (within-connection PDU retransmission)
- [ ] Error recovery level 2 (connection-level recovery within session)
- [ ] Header and data digest negotiation and CRC32C verification
- [ ] Task management: ABORT TASK, ABORT TASK SET, LUN RESET, TARGET WARM/COLD RESET
- [ ] Async event/message handling from target
- [ ] Low-level API: raw CDB pass-through (user builds CDB bytes, library transports)
- [ ] High-level API: typed Go functions (ReadBlocks, WriteBlocks, Inquiry, TestUnitReady, etc.)
- [ ] IOL-inspired integration test suite covering full feature phase conformance
- [ ] Comprehensive examples for developer onboarding
- [ ] iSCSI text negotiation for all mandatory keys (RFC 7143 Section 13)

### Out of Scope

- iSER/RDMA (iSCSI Extensions for RDMA) — niche use case, significant complexity, separate transport layer
- Multiple connections per session (MC/S) — rarely used in practice, defer to future version
- iSNS discovery — SendTargets sufficient for v1, iSNS adds protocol dependency
- Kernel integration / block device emulation — defeats the purpose of pure userspace
- Boot from iSCSI — requires kernel involvement by nature

## Context

- **Domain:** iSCSI is defined by RFC 7143 (which consolidated earlier RFCs 3720, 3980, 4850, 5048). The spec is extensive with detailed PDU formats, state machines, negotiation rules, and error recovery procedures.
- **Testing reference:** The UNH IOL iSCSI Initiator Full Feature Phase test suite (iol.unh.edu) provides a structured conformance testing framework. The test approach should be inspired by this structure.
- **Test infrastructure:** Whether to use gotgt (Go iSCSI target), an embedded minimal target, or another approach for integration tests needs research.
- **Existing landscape:** open-iscsi and libiscsi exist but are C-based and kernel-coupled. There's no mature pure-Go userspace initiator.
- **Philosophy:** Follows The Bronx Method — standards compliance is non-negotiable, constraint-aware design (build for real hardware and real users, not imagined scale), direct communication (clear errors, no jargon), minimal ceremony (no unnecessary abstractions or configuration layers), care in execution (correctness over convenience).

## Constraints

- **Language:** Go 1.25 — use modern features (range-over-func, enhanced generics, etc.) where they improve clarity
- **Dependencies:** Minimal external dependencies (Bronx Method: every dependency must justify its existence)
- **Standard:** RFC 7143 compliance — the spec drives implementation, not convenience
- **Testing:** Must be fully testable without manual infrastructure setup (no "plug in a SAN to run tests")
- **API style:** Go idiomatic — context.Context for cancellation, io.Reader/Writer where natural, structured errors
- **Quality:** High test coverage, clean interfaces, no dead code, no speculative abstractions

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Pure userspace, no kernel | Universal portability, embeddable in any Go app, containers, constrained environments | -- Pending |
| Single connection per session for v1 | MC/S rarely used, simplifies state machine significantly | -- Pending |
| Two-layer API (raw CDB + typed helpers) | Maximum flexibility for power users, convenience for common operations | -- Pending |
| All three error recovery levels in v1 | Full spec compliance is core value — can't claim RFC 7143 without ERL 0-2 | -- Pending |
| Full auth stack from day one | None + CHAP + mutual CHAP — CHAP is mandatory per spec for real deployments | -- Pending |
| Bronx Method philosophy | Standards non-negotiable, minimal ceremony, constraint-aware, direct communication | -- Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd:transition`):
1. Requirements invalidated? -> Move to Out of Scope with reason
2. Requirements validated? -> Move to Validated with phase reference
3. New requirements emerged? -> Add to Active
4. Decisions to log? -> Add to Key Decisions
5. "What This Is" still accurate? -> Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-03-31 after initialization*
