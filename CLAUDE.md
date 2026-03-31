<!-- GSD:project-start source:PROJECT.md -->
## Project

**uiscsi**

A pure-userspace iSCSI initiator library written in Go. It handles TCP connection, iSCSI login negotiation, and SCSI CDB transport over iSCSI PDUs entirely in userspace — no kernel SCSI stack, no iscsi-initiator-utils, no open-iscsi. Go applications import the library and talk directly to iSCSI targets.

**Core Value:** Full RFC 7143 compliance as a composable Go library — the spec is non-negotiable, everything else is secondary.

### Constraints

- **Language:** Go 1.25 — use modern features (range-over-func, enhanced generics, etc.) where they improve clarity
- **Dependencies:** Minimal external dependencies (Bronx Method: every dependency must justify its existence)
- **Standard:** RFC 7143 compliance — the spec drives implementation, not convenience
- **Testing:** Must be fully testable without manual infrastructure setup (no "plug in a SAN to run tests")
- **API style:** Go idiomatic — context.Context for cancellation, io.Reader/Writer where natural, structured errors
- **Quality:** High test coverage, clean interfaces, no dead code, no speculative abstractions
<!-- GSD:project-end -->

<!-- GSD:stack-start source:research/STACK.md -->
## Technology Stack

## Recommended Stack
### Core Technologies
| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| Go | 1.25 | Language runtime | Project constraint. Released August 2025. Includes `testing/synctest` for concurrent test determinism, container-aware GOMAXPROCS, DWARF5 debug info. NetBSD supported via pkgsrc. |
| `encoding/binary` | stdlib | PDU serialization/deserialization | iSCSI PDUs are fixed-layout binary structures with big-endian (network byte order) fields. `binary.BigEndian.PutUint32()` / `Uint32()` maps directly to PDU header fields. No external dependency needed -- this is the standard Go pattern for binary network protocols. |
| `hash/crc32` | stdlib | Header and data digest (CRC32C) | Provides `crc32.Castagnoli` (polynomial 0x82f63b78) which is the exact CRC variant required by RFC 7143. Hardware-accelerated on amd64/arm64 via SSE4.2/NEON. Zero external dependencies. |
| `net` | stdlib | TCP connection management | iSCSI runs over TCP. Standard `net.Dial`, `net.Conn` with `context.Context` deadlines. No need for anything beyond stdlib networking. |
| `crypto/md5`, `crypto/hmac` | stdlib | CHAP authentication | CHAP (RFC 1994) uses MD5-HMAC. Go stdlib covers this completely. |
| `log/slog` | stdlib | Structured logging | Standard since Go 1.21. Use for connection lifecycle, negotiation traces, error diagnostics. Library consumers can plug their own `slog.Handler`. No third-party logging dependency. |
| `context` | stdlib | Cancellation and timeouts | iSCSI operations need timeout/cancellation (login negotiation, command timeouts, session teardown). `context.Context` is idiomatic Go. |
### Supporting Libraries
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `gostor/gotgt` | v0.1.0+ (HEAD) | Integration test target | Use as the iSCSI target for integration tests. Pure Go, no kernel dependencies. ~276 GitHub stars, actively maintained (last commit March 2026). Import `pkg/port/iscsit` to embed a target in test processes. |
| `testing/synctest` | stdlib (Go 1.25) | Deterministic concurrent tests | Use for testing iSCSI state machines, timeout handling, reconnection logic. Graduated from experimental in Go 1.25. Virtualizes time in "bubbles" so concurrent tests are deterministic and fast. |
### Development Tools
| Tool | Purpose | Notes |
|------|---------|-------|
| `go test -race` | Race condition detection | Critical for concurrent PDU dispatch, session state machines, R2T handling. Run in CI on every commit. |
| `go vet` | Static analysis | Catches struct alignment issues relevant to binary encoding. |
| `golangci-lint` | Comprehensive linting | Use `sloglint` plugin for slog consistency. Configure with minimal ruleset (errcheck, govet, staticcheck, unused). |
| `gotgt` (as binary) | Manual integration testing | Run `gotgt daemon` locally to test against a real iSCSI target. Useful for development beyond unit tests. |
| `wireshark` / `tshark` | Protocol debugging | Wireshark has excellent iSCSI dissector. Capture on loopback when testing against gotgt for PDU-level debugging. |
| `libiscsi` `iscsi-test-cu` | Conformance reference | Not a Go tool, but the gold standard iSCSI/SCSI conformance test suite (C). Study test structure to inform Go test design. Do not depend on it at runtime. |
## Installation
# Initialize module
# No external runtime dependencies needed -- stdlib only.
# The entire iSCSI protocol implementation uses:
#   encoding/binary, hash/crc32, net, crypto/md5, crypto/hmac,
#   context, log/slog, sync, errors, fmt, io, bytes
# Test dependency (iSCSI target for integration tests)
# Development tools
## Alternatives Considered
| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| `encoding/binary` (manual) | `encoding/binary.Read/Write` with structs | Struct-based `binary.Read` works for simple fixed-layout headers, but iSCSI PDUs have bitfield packing (e.g., opcode + flags in first byte, AHS chains, variable-length data segments). Manual byte manipulation with `BigEndian.PutUint32()` gives precise control over bit-level layout. Use struct-based Read/Write only for simple sub-structures where the layout maps cleanly. |
| `hash/crc32` (stdlib) | `klauspost/crc32` | Only if benchmarking shows stdlib CRC32C is a bottleneck (unlikely -- stdlib already uses hardware acceleration). The klauspost fork was created before stdlib had SSE4.2 support; stdlib has caught up. |
| `gostor/gotgt` for test target | Embedded minimal target in test code | If gotgt proves too heavy or unreliable for tests, write a minimal iSCSI target that handles only the PDUs needed for test scenarios. Start with gotgt; fall back to custom if needed. |
| `log/slog` | `zerolog`, `zap` | Only if you need the library to use a specific third-party logger. Since this is a library (not an application), `slog` is correct -- callers inject their own handler. Adding zap/zerolog as a dependency to a library is an anti-pattern. |
| `testing/synctest` | `testify` assertions | `testify` adds a dependency for syntactic sugar. Use stdlib `testing` package. `synctest` is specifically for concurrent code testing which is central to iSCSI (async PDU dispatch, timeouts, reconnection). |
| Manual TCP (`net.Conn`) | gRPC, HTTP/2 | iSCSI is a raw TCP protocol with its own framing. HTTP-based transports are irrelevant. |
## What NOT to Use
| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `testify` | Adds external dependency for no real benefit in a stdlib-only library. Assertion-style tests obscure failure context compared to Go's `t.Errorf` with descriptive messages. | stdlib `testing` package with table-driven tests |
| `protobuf` / `flatbuffers` | iSCSI PDUs have a fixed binary format defined by RFC 7143. Serialization frameworks add complexity and don't map to the wire format. | `encoding/binary` with manual byte manipulation |
| `kubernetes-csi/csi-lib-iscsi` | Wraps `iscsiadm` CLI (open-iscsi userspace tools). Not a protocol implementation -- it shells out to system commands. | Build protocol from scratch per RFC 7143 |
| `longhorn/go-iscsi-helper` | Same problem: wraps system `tgtadm`/`iscsiadm` commands. Not a protocol library. | Build protocol from scratch per RFC 7143 |
| `u-root/iscsinl` | Uses Linux netlink to communicate with the kernel iSCSI module. Linux-only, kernel-dependent -- opposite of pure userspace. | Pure TCP implementation over `net.Conn` |
| `dell/gobrick` | CSI driver abstraction layer, not protocol implementation. | Build protocol from scratch per RFC 7143 |
| `encoding/gob` | Go-specific encoding format. iSCSI wire format is language-agnostic and defined by RFC. | `encoding/binary` |
| Third-party logging libs as direct dependency | Library should not dictate logging framework to consumers. | `log/slog` with injectable `slog.Handler` |
## Existing iSCSI Implementations to Study
| Implementation | Language | Type | Study Value |
|----------------|----------|------|-------------|
| **libiscsi** (sahlberg/libiscsi) | C | Initiator | Gold standard pure-userspace initiator. Async + sync API layers. Includes `iscsi-test-cu` conformance test suite with comprehensive SCSI/iSCSI tests. Study its API design (async core + sync wrapper), PDU encoding patterns, and test organization. |
| **open-iscsi** | C | Initiator | Linux kernel + userspace hybrid. Study its state machine design and negotiation logic, but NOT its architecture (kernel-coupled). |
| **gotgt** (gostor/gotgt) | Go | Target | Go iSCSI target. Study its PDU parsing code in `pkg/port/iscsit/` and SCSI command handling in `pkg/scsi/`. Directly useful since it is the same language and the complementary side of the protocol. |
| **RFC 7143** | N/A | Specification | The primary "implementation" to study. 295 pages. Defines every PDU format, state machine, negotiation rule, and error recovery procedure. |
## Stack Patterns by Variant
- Use stdlib `testing` with table-driven tests
- Use `bytes.Buffer` as mock `net.Conn` for PDU serialization tests
- Use `testing/synctest` for state machine timeout tests
- Embed `gotgt` as in-process iSCSI target
- Use `net.Pipe()` or loopback TCP for transport
- Consider `t.Parallel()` for test independence
- Use `slog` at debug level to trace PDU exchanges
- Capture with `tshark -i lo -Y iscsi` when testing against gotgt over loopback
## Version Compatibility
| Component | Compatible With | Notes |
|-----------|-----------------|-------|
| Go 1.25 | NetBSD 10.1 (amd64) | Available via pkgsrc. NetBSD is a supported GOOS. |
| Go 1.25 | `testing/synctest` | Graduated from experimental (was GOEXPERIMENT in 1.24). No build tag needed. |
| Go 1.25 | `hash/crc32` Castagnoli | Hardware-accelerated CRC32C on amd64 (SSE4.2). Available since Go 1.6+; well-tested. |
| `gostor/gotgt` | Go 1.25 | No version-pinned releases; use latest HEAD. Check `go.mod` compatibility before pinning. |
## Sources
- [Go 1.25 Release Notes](https://go.dev/doc/go1.25) -- Go 1.25 features including synctest graduation, container-aware GOMAXPROCS (HIGH confidence)
- [hash/crc32 package docs](https://pkg.go.dev/hash/crc32) -- Castagnoli polynomial support, hardware acceleration (HIGH confidence)
- [encoding/binary package docs](https://pkg.go.dev/encoding/binary) -- BigEndian byte order, struct encoding patterns (HIGH confidence)
- [log/slog package docs](https://pkg.go.dev/log/slog) -- Structured logging API, Handler interface (HIGH confidence)
- [testing/synctest package docs](https://pkg.go.dev/testing/synctest) -- Concurrent test "bubble" mechanism (HIGH confidence)
- [gostor/gotgt GitHub](https://github.com/gostor/gotgt) -- Go iSCSI target framework, ~276 stars, active development (MEDIUM confidence -- no stable releases)
- [sahlberg/libiscsi GitHub](https://github.com/sahlberg/libiscsi) -- C userspace iSCSI initiator, conformance test suite reference (HIGH confidence)
- [RFC 7143](https://datatracker.ietf.org/doc/html/rfc7143) -- iSCSI protocol specification, 295 pages (HIGH confidence)
- [Go on NetBSD wiki](https://go.dev/wiki/NetBSD) -- NetBSD platform support status (MEDIUM confidence)
- [SNIA: Testing iSCSI/SCSI Protocol Compliance Using Libiscsi](https://www.snia.org/educational-library/testing-iscsi-scsi-protocol-compliance-using-libiscsi-2013) -- Conformance test methodology reference (HIGH confidence)
<!-- GSD:stack-end -->

<!-- GSD:conventions-start source:CONVENTIONS.md -->
## Conventions

Conventions not yet established. Will populate as patterns emerge during development.
<!-- GSD:conventions-end -->

<!-- GSD:architecture-start source:ARCHITECTURE.md -->
## Architecture

Architecture not yet mapped. Follow existing patterns found in the codebase.
<!-- GSD:architecture-end -->

<!-- GSD:workflow-start source:GSD defaults -->
## GSD Workflow Enforcement

Before using Edit, Write, or other file-changing tools, start work through a GSD command so planning artifacts and execution context stay in sync.

Use these entry points:
- `/gsd:quick` for small fixes, doc updates, and ad-hoc tasks
- `/gsd:debug` for investigation and bug fixing
- `/gsd:execute-phase` for planned phase work

Do not make direct repo edits outside a GSD workflow unless the user explicitly asks to bypass it.
<!-- GSD:workflow-end -->



<!-- GSD:profile-start -->
## Developer Profile

> Profile not yet configured. Run `/gsd:profile-user` to generate your developer profile.
> This section is managed by `generate-claude-profile` -- do not edit manually.
<!-- GSD:profile-end -->
