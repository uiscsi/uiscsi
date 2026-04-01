# Phase 7: Public API, Observability, and Release - Research

**Researched:** 2026-04-01
**Domain:** Go public API design, iSCSI conformance testing, library documentation
**Confidence:** HIGH

## Summary

Phase 7 wraps the mature internal packages (`internal/session`, `internal/scsi`, `internal/login`, `internal/transport`) into a clean public API at the top-level `github.com/rkujawa/uiscsi` package. The internal API surface is complete and well-structured: 19+ CDB builders, session lifecycle management, discovery, TMF, logout, and observability hooks are all implemented and tested. The public API is primarily a thin wrapper layer that re-exports types and orchestrates the TCP-connect -> login -> session -> command flow.

OBS-01, OBS-02, OBS-03 are already complete from Phase 06.1 -- the observability hooks (`WithPDUHook`, `WithMetricsHook`, `MetricEvent`, slog integration, state callbacks) need only to be re-exported through public option functions, not reimplemented.

**Primary recommendation:** Build the public API as a thin orchestration layer in the root package. Use public wrapper types (`Session`, `Target`, `SCSIError`, `TransportError`, `AuthError`) that wrap internal types. The test infrastructure should be a two-tier approach: custom mock target in `test/` for PDU-level conformance, gotgt for full-stack integration (when available).

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions
- **D-01:** Top-level `github.com/rkujawa/uiscsi` package exports the public API. Consumers write `uiscsi.Dial()`, `uiscsi.ReadBlocks()`, etc.
- **D-02:** Public wrapper types in the uiscsi package (`Session`, `Result`, `Target`, `SenseInfo`, etc.) wrap internal types. Internal types stay internal -- consumers never import `internal/`.
- **D-03:** Two-step connection flow: `uiscsi.Discover(ctx, addr)` returns targets, then `uiscsi.Dial(ctx, addr, opts...)` creates a session. Discovery is optional -- Dial works directly with a target name.
- **D-04:** Primary block I/O uses `[]byte` -- `ReadBlocks` returns `[]byte`, `WriteBlocks` takes `[]byte`. Separate streaming functions (`StreamRead`/`StreamWrite` or similar) return `io.Reader` / take `io.Reader` for large transfers.
- **D-05:** Raw CDB pass-through is a method on Session: `sess.Execute(ctx, lun, cdb, opts...)` takes raw CDB bytes and returns raw response + status. Options like `WithDataIn(allocLen)` control transfer direction.
- **D-06:** Typed error hierarchy: `SCSIError` (wraps sense data + status), `TransportError` (wraps iSCSI/TCP errors), `AuthError` (login failures). All implement `error`. Consumers use `errors.As()` to extract detail.
- **D-07:** Tiered test approach: custom mock target (in Go) for PDU-level conformance tests, gotgt embedded target for full-stack integration tests.
- **D-08:** IOL structure-inspired conformance suite: organize by IOL test categories (login, full-feature, error recovery, task management), write Go-idiomatic table-driven tests, use IOL test names/numbers as comments for traceability.
- **D-09:** Test infrastructure lives in top-level `test/` directory. `test/target.go` for mock target, `test/conformance/` for IOL-inspired tests, `test/integration/` for gotgt-based E2E.
- **D-10:** Four runnable example programs in `examples/` directory (DOC-02 through DOC-05): discover+login+read, write+verify, raw CDB pass-through, error handling and recovery. Each is a standalone `main()` program.
- **D-11:** Godoc testable examples (`func ExampleDial()`, `func ExampleSession_ReadBlocks()`, etc.) for API reference discoverability, in addition to the `examples/` programs.
- **D-12:** README.md with overview, quick start, feature list, links to examples, API reference (godoc link), requirements, and license.

### Claude's Discretion
- Exact method signatures for high-level functions (parameter order, option names)
- Which internal types need public equivalents vs which stay opaque
- Mock target implementation detail (handler registration pattern, PDU sequence control)
- Streaming API naming (StreamRead vs ReadStream vs ReadTo)
- Godoc example selection (which functions get Example tests)
- README sections beyond the agreed structure

### Deferred Ideas (OUT OF SCOPE)
None -- discussion stayed within phase scope.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| API-01 | Low-level raw CDB pass-through | D-05: `sess.Execute(ctx, lun, cdb, opts...)` wraps `session.Submit()` with raw CDB bytes. Internal `session.Command` already supports arbitrary CDB. |
| API-02 | High-level typed Go functions (ReadBlocks, WriteBlocks, Inquiry, etc.) | All 19+ CDB builders in `internal/scsi` are ready to wrap. Each public method calls the internal builder, submits via session, and parses the result. |
| API-03 | context.Context integration for cancellation/timeouts | Already implemented internally -- `session.Submit()` takes `context.Context`. Public API passes through. |
| API-04 | io.Reader/io.Writer interfaces where natural | D-04: Primary API uses `[]byte`, streaming variants use `io.Reader`. Internal already uses `io.Reader` for write data and `io.Reader` for read results. |
| API-05 | Structured error types with sense data, iSCSI status, response classification | D-06: `SCSIError`, `TransportError`, `AuthError`. Internal has `scsi.CommandError`, `login.LoginError`, `digest.DigestError` to wrap. |
| OBS-01 | Connection-level statistics | Complete from Phase 06.1 -- `WithMetricsHook`/`MetricEvent`. Re-export as public option. |
| OBS-02 | Structured logging via log/slog | Complete from Phase 06.1 -- `WithLogger`. Re-export as public option. |
| OBS-03 | Hooks/callbacks for monitoring state transitions | Complete from Phase 06.1 -- `WithPDUHook`, `WithAsyncHandler`. Re-export as public options. |
| TEST-01 | IOL-inspired conformance test suite | D-07/D-08: Mock target + IOL-structured table-driven tests in `test/conformance/`. |
| TEST-02 | Integration test infrastructure with automated target setup | D-07/D-09: Mock target in `test/target.go`, gotgt integration in `test/integration/`. |
| DOC-01 | Comprehensive API documentation with godoc | D-11: Godoc comments on all public types/functions + testable examples. |
| DOC-02 | Example: basic discovery, login, read blocks, logout | D-10: `examples/discover-read/main.go` |
| DOC-03 | Example: write blocks with verification | D-10: `examples/write-verify/main.go` |
| DOC-04 | Example: raw CDB pass-through | D-10: `examples/raw-cdb/main.go` |
| DOC-05 | Example: error handling and recovery | D-10: `examples/error-handling/main.go` |
</phase_requirements>

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| Go stdlib | 1.25.5 | Everything | Project constraint. Zero external dependencies for library code. |
| `testing/synctest` | stdlib (1.25) | Deterministic concurrent tests | Mock target PDU exchanges need deterministic timing. |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `gostor/gotgt` | HEAD | Full-stack integration test target | E2E integration tests in `test/integration/`. Not available on target machine; tests must be build-tag gated. |

**Installation:**
```bash
# No runtime dependencies. Test dependency only:
go get github.com/gostor/gotgt@latest
```

**Version verification:** Go 1.25.5 confirmed on target system. gotgt not installed; integration tests must be skippable.

## Architecture Patterns

### Recommended Project Structure
```
github.com/rkujawa/uiscsi/
  uiscsi.go          # Dial(), Discover(), Option types, package doc
  session.go         # Session type, all methods (ReadBlocks, WriteBlocks, Execute, etc.)
  errors.go          # SCSIError, TransportError, AuthError
  types.go           # Target, Result, InquiryData, Capacity, SenseInfo, etc. (public wrappers)
  options.go         # WithTarget(), WithCHAP(), WithLogger(), etc. (functional options)
  stream.go          # StreamRead(), StreamWrite() (io.Reader/io.Writer variants)
  example_test.go    # Godoc testable examples (ExampleDial, ExampleSession_ReadBlocks, etc.)
  test/
    target.go        # Mock iSCSI target for testing (PDU-level)
    target_test.go   # Tests for the mock target itself
    conformance/
      login_test.go       # IOL login phase tests
      fullfeature_test.go # IOL full-feature phase tests
      error_test.go       # IOL error recovery tests
      task_test.go        # IOL task management tests
    integration/
      gotgt_test.go       # Full-stack E2E with gotgt (build-tag gated)
  examples/
    discover-read/main.go
    write-verify/main.go
    raw-cdb/main.go
    error-handling/main.go
```

### Pattern 1: Thin Public Wrapper Over Internal Types

**What:** Public `Session` type holds `*session.Session` and delegates. Public types mirror internal types with different names and no internal package dependencies in their signatures.

**When to use:** Every public API method.

**Example:**
```go
// session.go
package uiscsi

import (
    "context"
    "github.com/rkujawa/uiscsi/internal/scsi"
    "github.com/rkujawa/uiscsi/internal/session"
)

// Session represents an active iSCSI session to a target.
type Session struct {
    s *session.Session
}

// ReadBlocks reads blocks from the given LUN starting at lba.
// It returns the raw block data as a byte slice.
func (s *Session) ReadBlocks(ctx context.Context, lun uint64, lba uint64, blocks uint32, blockSize uint32) ([]byte, error) {
    cmd := scsi.Read16(lun, lba, blocks, blockSize)
    resultCh, err := s.s.Submit(ctx, cmd)
    if err != nil {
        return nil, wrapTransportError(err)
    }
    result := <-resultCh
    data, err := scsi.CheckResult(result) // needs to be exported or duplicated
    if err != nil {
        return nil, wrapSCSIError(err)
    }
    return data, nil
}
```

### Pattern 2: Functional Options Composition

**What:** Public options wrap internal session and login options into a single `Option` type for `Dial()`.

**When to use:** `Dial()` configuration.

**Example:**
```go
// options.go
package uiscsi

import (
    "log/slog"
    "time"

    "github.com/rkujawa/uiscsi/internal/login"
    "github.com/rkujawa/uiscsi/internal/session"
)

// Option configures a Dial or Discover call.
type Option func(*dialConfig)

type dialConfig struct {
    loginOpts   []login.LoginOption
    sessionOpts []session.SessionOption
}

// WithTarget sets the target IQN.
func WithTarget(iqn string) Option {
    return func(c *dialConfig) {
        c.loginOpts = append(c.loginOpts, login.WithTarget(iqn))
    }
}

// WithCHAP enables CHAP authentication.
func WithCHAP(user, secret string) Option {
    return func(c *dialConfig) {
        c.loginOpts = append(c.loginOpts, login.WithCHAP(user, secret))
    }
}

// WithLogger sets the structured logger for the session.
func WithLogger(l *slog.Logger) Option {
    return func(c *dialConfig) {
        c.sessionOpts = append(c.sessionOpts, session.WithLogger(l))
        c.loginOpts = append(c.loginOpts, login.WithLoginLogger(l))
    }
}
```

### Pattern 3: Typed Error Hierarchy

**What:** Three concrete error types wrapping internal errors, all implementing `error`. Consumers use `errors.As()`.

**When to use:** All error returns from public API.

**Example:**
```go
// errors.go
package uiscsi

import (
    "fmt"
    "github.com/rkujawa/uiscsi/internal/scsi"
    "github.com/rkujawa/uiscsi/internal/login"
)

// SCSIError represents a SCSI command failure with status and sense data.
type SCSIError struct {
    Status   uint8
    SenseKey uint8
    ASC      uint8
    ASCQ     uint8
    Message  string
}

func (e *SCSIError) Error() string {
    return fmt.Sprintf("scsi: status 0x%02X: %s", e.Status, e.Message)
}

// TransportError represents an iSCSI transport or connection failure.
type TransportError struct {
    Op  string // "dial", "submit", "read", etc.
    Err error  // underlying error
}

func (e *TransportError) Error() string { return fmt.Sprintf("iscsi %s: %s", e.Op, e.Err) }
func (e *TransportError) Unwrap() error { return e.Err }

// AuthError represents an authentication failure during login.
type AuthError struct {
    StatusClass  uint8
    StatusDetail uint8
    Message      string
}

func (e *AuthError) Error() string {
    return fmt.Sprintf("iscsi auth: %s", e.Message)
}
```

### Pattern 4: Mock Target for Conformance Tests

**What:** An in-process TCP server that speaks enough iSCSI to exercise the initiator. Uses handler registration pattern from existing `session_test.go` mock goroutines.

**When to use:** `test/target.go` for all conformance tests.

**Example:**
```go
// test/target.go
package test

import (
    "net"
    "github.com/rkujawa/uiscsi/internal/pdu"
    "github.com/rkujawa/uiscsi/internal/transport"
)

// MockTarget is an in-process iSCSI target for testing.
type MockTarget struct {
    listener net.Listener
    handlers map[pdu.OpCode]PDUHandler
}

// PDUHandler processes a received PDU and returns zero or more response PDUs.
type PDUHandler func(req *transport.RawPDU) []*transport.RawPDU

// NewMockTarget starts a mock target listening on a random loopback port.
func NewMockTarget() (*MockTarget, error) { ... }

// Addr returns the listener address (for Dial).
func (t *MockTarget) Addr() string { ... }

// Handle registers a handler for a specific opcode.
func (t *MockTarget) Handle(op pdu.OpCode, h PDUHandler) { ... }

// Close shuts down the mock target.
func (t *MockTarget) Close() error { ... }
```

### Pattern 5: Raw CDB Pass-Through

**What:** `Execute()` method takes raw CDB bytes and direction options, wraps into `session.Command`, submits, returns raw response.

**When to use:** API-01 requirement.

**Example:**
```go
// ExecuteOption configures a raw CDB execution.
type ExecuteOption func(*executeConfig)

type executeConfig struct {
    dataIn  uint32     // expected data-in length (read)
    dataOut io.Reader  // data-out payload (write)
}

// WithDataIn indicates the command expects data-in of the given length.
func WithDataIn(allocLen uint32) ExecuteOption {
    return func(c *executeConfig) { c.dataIn = allocLen }
}

// WithDataOut provides write data for the command.
func WithDataOut(r io.Reader, length uint32) ExecuteOption {
    return func(c *executeConfig) { c.dataOut = r }
}

// Execute sends a raw SCSI CDB and returns the raw response.
func (s *Session) Execute(ctx context.Context, lun uint64, cdb []byte, opts ...ExecuteOption) (*RawResult, error) {
    cfg := &executeConfig{}
    for _, o := range opts {
        o(cfg)
    }
    var cmd session.Command
    copy(cmd.CDB[:], cdb)
    cmd.LUN = lun
    if cfg.dataIn > 0 {
        cmd.Read = true
        cmd.ExpectedDataTransferLen = cfg.dataIn
    }
    if cfg.dataOut != nil {
        cmd.Write = true
        cmd.Data = cfg.dataOut
    }
    resultCh, err := s.s.Submit(ctx, cmd)
    if err != nil {
        return nil, wrapTransportError(err)
    }
    result := <-resultCh
    return convertRawResult(result), nil
}
```

### Anti-Patterns to Avoid
- **Leaking internal types through public API:** Never return `session.Result`, `session.Command`, `login.NegotiatedParams` directly. Always wrap.
- **Exposing session.Submit's channel-based API:** The public API should be synchronous (blocking). The channel-based async pattern is an internal implementation detail.
- **Making the public API stateful beyond Session:** `ReadBlocks` should not require the caller to track sequence numbers, ITTs, or CmdSN. All protocol state is encapsulated in Session.
- **Blocking without context:** Every public method that does I/O must accept `context.Context` as its first argument.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| CDB construction | Manual byte packing in public API | Internal `scsi.*` CDB builders | Already implemented, tested, handles edge cases (VPD page lengths, service action opcodes) |
| Session lifecycle | Custom TCP + login + session orchestration | Chain: `transport.Dial` -> `login.Login` -> `session.NewSession` | Complex multi-step process with error handling at each stage |
| Sense data parsing | Manual sense byte interpretation | `scsi.ParseSense` + `scsi.CommandError` | Fixed/descriptor format handling, ASC/ASCQ lookup table with ~70 entries |
| iSCSI text encoding | Custom key-value serializer | `login.EncodeTextKV` / `login.DecodeTextKV` | Null-delimited format with edge cases |
| Error classification | Manual status code checking | Internal `scsi.checkResult` pattern | Centralizes status check + sense parse + data read |

## Common Pitfalls

### Pitfall 1: Synchronous vs Asynchronous API Mismatch
**What goes wrong:** Public API returns a channel like internal Submit does, forcing consumers to deal with channel semantics.
**Why it happens:** Directly exposing the internal `<-chan Result` pattern.
**How to avoid:** Public methods block and return `(data, error)`. Internally they call Submit then immediately `<-resultCh`. Advanced users who need async can use goroutines.
**Warning signs:** Public method signatures that return channels.

### Pitfall 2: Result.Data io.Reader Consumed Multiple Times
**What goes wrong:** `Result.Data` is an `io.Reader` -- once read, it is empty. If public API reads it to return `[]byte`, any subsequent read gets nothing.
**Why it happens:** io.Reader is single-use.
**How to avoid:** `ReadBlocks` reads the io.Reader once via `io.ReadAll` and returns `[]byte`. The streaming variant `StreamRead` returns the io.Reader directly (single use, documented).
**Warning signs:** Tests that read Result.Data twice.

### Pitfall 3: Error Type Wrapping Breaks errors.As/errors.Is
**What goes wrong:** Wrapping errors in public types without implementing `Unwrap()` breaks the error chain.
**Why it happens:** Forgetting Unwrap on wrapper types.
**How to avoid:** `TransportError` must implement `Unwrap() error`. `SCSIError` wraps sense info directly (no underlying error to unwrap). `AuthError` should wrap `login.LoginError`.
**Warning signs:** `errors.As(err, &target)` returns false when it should match.

### Pitfall 4: Block Size Not Known at ReadBlocks Call
**What goes wrong:** `ReadBlocks` needs block size to calculate `ExpectedDataTransferLength`, but the caller may not know it yet.
**Why it happens:** Block size comes from ReadCapacity, which the caller may not have issued.
**How to avoid:** Two approaches: (a) require block size as parameter (transparent, no magic), or (b) cache block size on Session after first ReadCapacity. Recommendation: require it as parameter -- matches D-04 "typed functions" and avoids hidden state.
**Warning signs:** Methods that silently fail when block size is wrong.

### Pitfall 5: Discover() and Dial() Error Handling Asymmetry
**What goes wrong:** `Discover()` succeeds but `Dial()` fails, or vice versa, and the consumer does not know which step failed.
**Why it happens:** Both wrap multiple internal operations (TCP connect, login, session creation).
**How to avoid:** Use typed errors: `TransportError{Op: "dial"}` for TCP failures, `AuthError` for login failures.
**Warning signs:** Generic "connection failed" errors without indication of which step failed.

### Pitfall 6: Mock Target Race Conditions
**What goes wrong:** Test mock target has races between accepting connections and sending responses.
**Why it happens:** Mock target runs in a goroutine, test proceeds before target is ready.
**How to avoid:** Mock target signals readiness (e.g., blocks until listener is bound). Use `testing/synctest` for deterministic PDU exchange timing.
**Warning signs:** Flaky tests that pass individually but fail in parallel.

### Pitfall 7: Export Internal Types by Accident
**What goes wrong:** A public function signature references an internal type, causing a compilation error for consumers.
**Why it happens:** Go enforces that exported functions cannot return/accept unexported types from internal packages.
**How to avoid:** Every type in the public API must be defined in the root `uiscsi` package or be a standard library type. Run `go vet` which catches this.
**Warning signs:** `go build` errors mentioning "use of internal package not allowed".

### Pitfall 8: Godoc Examples That Don't Compile
**What goes wrong:** Example functions in `example_test.go` have compilation errors or don't produce expected output.
**Why it happens:** Examples reference types/functions that changed during development.
**How to avoid:** Examples must be in `_test.go` files with `package uiscsi_test` (external test package). Run `go test` which compiles all examples. Use `// Output:` comments for verified output.
**Warning signs:** `go test` failures in example compilation.

## Code Examples

### Dial Flow (Core Integration Point)
```go
// uiscsi.go
package uiscsi

import (
    "context"
    "fmt"

    "github.com/rkujawa/uiscsi/internal/login"
    "github.com/rkujawa/uiscsi/internal/session"
    "github.com/rkujawa/uiscsi/internal/transport"
)

// Dial connects to an iSCSI target, performs login negotiation, and
// returns a Session ready for SCSI commands.
func Dial(ctx context.Context, addr string, opts ...Option) (*Session, error) {
    cfg := &dialConfig{}
    for _, o := range opts {
        o(cfg)
    }

    tc, err := transport.Dial(ctx, addr)
    if err != nil {
        return nil, &TransportError{Op: "dial", Err: err}
    }

    params, err := login.Login(ctx, tc, cfg.loginOpts...)
    if err != nil {
        tc.Close()
        var le *login.LoginError
        if errors.As(err, &le) {
            if le.StatusClass == 2 && le.StatusDetail == 1 {
                return nil, &AuthError{
                    StatusClass: le.StatusClass,
                    StatusDetail: le.StatusDetail,
                    Message: le.Message,
                }
            }
        }
        return nil, &TransportError{Op: "login", Err: err}
    }

    // Add reconnect info for ERL 0 recovery.
    allSessionOpts := append(cfg.sessionOpts,
        session.WithReconnectInfo(addr, cfg.loginOpts...))

    s := session.NewSession(tc, *params, allSessionOpts...)
    return &Session{s: s}, nil
}
```

### Public Type Mapping (Internal -> Public)
```
Internal Type                     -> Public Type
session.Result                    -> uiscsi.Result (copy fields, read Data into []byte)
session.DiscoveryTarget           -> uiscsi.Target
session.Portal                    -> uiscsi.Portal
session.AsyncEvent                -> uiscsi.AsyncEvent
session.TMFResult                 -> uiscsi.TMFResult
scsi.InquiryResponse              -> uiscsi.InquiryData
scsi.ReadCapacity10Response       -> uiscsi.Capacity
scsi.ReadCapacity16Response       -> uiscsi.Capacity (unified)
scsi.SenseData                    -> uiscsi.SenseInfo
scsi.CommandError                 -> uiscsi.SCSIError
login.LoginError                  -> uiscsi.AuthError
session.MetricEvent               -> uiscsi.MetricEvent (re-export)
session.MetricEventType           -> uiscsi.MetricEventType (re-export)
session.PDUDirection              -> uiscsi.PDUDirection (re-export)
```

### Godoc Testable Example Pattern
```go
// example_test.go
package uiscsi_test

import (
    "context"
    "fmt"
    "github.com/rkujawa/uiscsi"
)

func ExampleDial() {
    ctx := context.Background()
    sess, err := uiscsi.Dial(ctx, "192.168.1.100:3260",
        uiscsi.WithTarget("iqn.2026-03.com.example:storage"),
    )
    if err != nil {
        fmt.Println("dial failed:", err)
        return
    }
    defer sess.Close()
    fmt.Println("connected")
}
```

### Mock Target Handler Registration
```go
// test/target.go -- handler pattern based on session_test.go mock goroutines
func (t *MockTarget) HandleLogin() {
    t.Handle(pdu.OpLoginReq, func(req *transport.RawPDU) []*transport.RawPDU {
        // Parse LoginReq, build LoginResp with AuthMethod=None, transit to FFP
        resp := buildLoginResp(req) // helper that builds a valid LoginResp
        return []*transport.RawPDU{resp}
    })
}

func (t *MockTarget) HandleSCSI(status uint8, data []byte) {
    t.Handle(pdu.OpSCSICommand, func(req *transport.RawPDU) []*transport.RawPDU {
        // Parse SCSI Command, return DataIn (if read) + SCSIResponse
        return buildSCSIResponse(req, status, data)
    })
}
```

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `GOEXPERIMENT=synctest` | `testing/synctest` in stdlib | Go 1.25 (Aug 2025) | No build tag needed for concurrent test determinism |
| Manual `go vet` + separate linters | `golangci-lint` with `sloglint` | Ongoing | Comprehensive static analysis in one tool |
| `testify` assertions | stdlib `testing` with table-driven tests | Project convention | Zero test dependencies |

## Open Questions

1. **gotgt Compatibility with Go 1.25 on NetBSD**
   - What we know: gotgt is a pure Go project, should compile. It is not installed on the target machine.
   - What's unclear: Whether gotgt's iSCSI target implementation handles all PDU sequences our conformance tests need.
   - Recommendation: Build-tag gate gotgt integration tests (`//go:build integration`). Mock target handles all conformance testing. gotgt is bonus coverage.

2. **How Deep Should Mock Target Be?**
   - What we know: Existing `session_test.go` already has mock target goroutines that handle login, SCSI commands, Data-In, R2T, etc.
   - What's unclear: Whether to extract these into `test/target.go` or build fresh.
   - Recommendation: Extract and generalize the existing mock patterns. They are proven and handle the PDU sequences correctly.

3. **Block Size Parameter Strategy**
   - What we know: ReadBlocks/WriteBlocks need block size. Two options: parameter vs cached on Session.
   - What's unclear: Whether caching block size is acceptable API design for a library.
   - Recommendation: Require block size as parameter. Simpler, no hidden state. Provide a `ReadCapacity` method so callers can query it. This matches the "composable library" philosophy.

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go | Everything | Yes | 1.25.5 | -- |
| gotgt | Integration E2E tests | No | -- | Mock target for conformance; build-tag gate gotgt tests |
| golangci-lint | Linting | No | -- | `go vet` for basic analysis |
| tgtd | Conformance reference | No | -- | Mock target |

**Missing dependencies with no fallback:**
- None. All critical functionality uses stdlib.

**Missing dependencies with fallback:**
- gotgt: Gate with build tags, use mock target as primary test infrastructure.
- golangci-lint: Use `go vet` during development.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` (1.25.5) |
| Config file | None needed (Go convention) |
| Quick run command | `go test ./...` |
| Full suite command | `go test -race -count=1 ./...` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| API-01 | Raw CDB pass-through | unit | `go test -run TestSession_Execute -v ./` | No -- Wave 0 |
| API-02 | High-level typed functions | unit | `go test -run TestSession_ReadBlocks -v ./` | No -- Wave 0 |
| API-03 | context.Context integration | unit | `go test -run TestSession_ContextCancel -v ./` | No -- Wave 0 |
| API-04 | io.Reader/io.Writer streaming | unit | `go test -run TestSession_StreamRead -v ./` | No -- Wave 0 |
| API-05 | Structured error types | unit | `go test -run TestErrors -v ./` | No -- Wave 0 |
| OBS-01 | Connection statistics | unit | `go test -run TestMetrics -v ./internal/session/` | Yes (metrics_test.go) |
| OBS-02 | Structured slog logging | unit | `go test -run TestLogging -v ./internal/session/` | Yes (session_test.go) |
| OBS-03 | State transition callbacks | unit | `go test -run TestPDUHook -v ./internal/session/` | Yes (metrics_test.go) |
| TEST-01 | IOL-inspired conformance suite | integration | `go test -run TestConformance -v ./test/conformance/` | No -- Wave 0 |
| TEST-02 | Automated test infrastructure | integration | `go test -v ./test/...` | No -- Wave 0 |
| DOC-01 | Godoc API documentation | build | `go doc ./...` (compiles all docs) | No -- Wave 0 |
| DOC-02 | Example: discover+login+read | build | `go build ./examples/discover-read/` | No -- Wave 0 |
| DOC-03 | Example: write+verify | build | `go build ./examples/write-verify/` | No -- Wave 0 |
| DOC-04 | Example: raw CDB pass-through | build | `go build ./examples/raw-cdb/` | No -- Wave 0 |
| DOC-05 | Example: error handling | build | `go build ./examples/error-handling/` | No -- Wave 0 |

### Sampling Rate
- **Per task commit:** `go test -race ./...`
- **Per wave merge:** `go test -race -count=1 ./...`
- **Phase gate:** Full suite green before `/gsd:verify-work`

### Wave 0 Gaps
- [ ] Root package test file (`uiscsi_test.go`) -- covers API-01 through API-05
- [ ] `test/target.go` -- mock target infrastructure for TEST-01, TEST-02
- [ ] `test/conformance/` directory -- IOL-inspired test files
- [ ] `examples/` directory with four subdirectories -- DOC-02 through DOC-05

## Sources

### Primary (HIGH confidence)
- Codebase: `internal/session/types.go`, `session.go`, `discovery.go`, `logout.go`, `tmf.go`, `metrics.go` -- full internal API surface
- Codebase: `internal/scsi/*.go` -- all 19+ CDB builders and parsers
- Codebase: `internal/login/login.go`, `params.go`, `errors.go` -- login flow and option types
- Codebase: `internal/transport/conn.go` -- Dial function, Conn type
- Codebase: `internal/session/session_test.go` -- existing mock target patterns (newTestSession, writeDataInPDU)
- [Go 1.25 Release Notes](https://go.dev/doc/go1.25) -- testing/synctest graduation
- [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) -- idiomatic Go API patterns

### Secondary (MEDIUM confidence)
- `.planning/research/e2e-target-infrastructure.md` -- gotgt evaluation, tgtd IPC protocol analysis
- `.planning/research/e2e-testing-approaches.md` -- IOL test structure, libiscsi iscsi-test-cu analysis

### Tertiary (LOW confidence)
- gotgt availability on NetBSD -- not tested, assumed compilable as pure Go

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH -- stdlib only, well-understood
- Architecture: HIGH -- internal API is complete and well-structured, wrapping is straightforward
- Pitfalls: HIGH -- derived from direct codebase analysis and Go API design experience
- Test infrastructure: MEDIUM -- mock target needs to be built; gotgt integration untested on target platform

**Research date:** 2026-04-01
**Valid until:** 2026-05-01 (stable domain, no fast-moving dependencies)
