# Phase 9: lio-e2e-tests - Context

**Gathered:** 2026-04-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Build `test/lio/` helper package for configfs-based LIO iSCSI target setup/teardown, implement E2E tests covering 7 scenarios against a real kernel iSCSI target, and drop the dead gotgt integration stubs. Local execution only — CI workflow deferred to a future phase.

</domain>

<decisions>
## Implementation Decisions

### Helper API design
- **D-01:** Simple function API integrated with `testing.T`: `lio.Setup(t, lio.Config{...})` returns an explicit cleanup func
- **D-02:** Cleanup func returned to caller (not `t.Cleanup()`) — allows tests to inspect state after target removal or control teardown timing

### Configfs operations
- **D-03:** All LIO configuration via direct configfs manipulation (`os.MkdirAll`, `os.WriteFile`, `os.Symlink`, `os.Remove`) — no targetcli dependency
- **D-04:** Use `fileio` backstore with `/dev/shm/` tmpfs backing files as ramdisk substitute (kernel 6.19 removed `rd_mcp`)
- **D-05:** Bind to `127.0.0.1:<ephemeral_port>` to avoid conflicts with existing targets

### E2E test scope
- **D-06:** All 7 scenarios in scope:
  1. Basic connectivity — Discover + Dial + Inquiry + ReadCapacity + Close
  2. Data integrity — Write blocks, read back, verify byte-for-byte match
  3. CHAP authentication — CHAP and mutual CHAP against LIO ACL credentials
  4. CRC32C digests — Header and data digest negotiation and verification
  5. Multi-LUN — Multiple LUNs with different sizes, enumerate via ReportLuns
  6. Task management — AbortTask, LUNReset against real target
  7. Error recovery — Connection drop mid-session, verify ERL 0 reconnect behavior

### Build tag and module structure
- **D-07:** `//go:build e2e` on everything — both `test/lio/` helper package and all E2E test files. Nothing compiles unless `-tags e2e` is passed explicitly.

### Cleanup strategy
- **D-08:** All test targets use a fixed IQN prefix: `iqn.2026-04.com.uiscsi.e2e:`
- **D-09:** Cleanup func scans configfs for the prefix and tears down anything matching. TestMain also sweeps on entry to catch orphans from previous crashed runs.

### Gotgt removal
- **D-10:** Delete `test/integration/gotgt_test.go` — dead code, all 6 tests are `t.Skip` stubs

### Claude's Discretion
- Config struct field names and layout
- How to detect and skip when not running as root / modules not loaded
- Ephemeral port allocation strategy (`:0` listener trick vs fixed offset)
- Whether to split E2E tests across multiple files by scenario or keep in one file
- Fileio backstore size (likely 64MB per LUN is sufficient)
- Whether connection drop test kills TCP conn or uses iptables/firewall

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Public API (what E2E tests will exercise)
- `uiscsi.go` — Dial(), Discover() with default port 3260
- `session.go` — ReadBlocks(), WriteBlocks(), Inquiry(), ReadCapacity(), ReportLuns(), AbortTask(), LUNReset(), Close()
- `options.go` — WithTarget(), WithCHAP(), WithMutualCHAP(), WithInitiatorName(), WithHeaderDigest(), WithDataDigest()
- `types.go` — DecodeLUN(), DeviceTypeName(), Target, Portal, Capacity, InquiryData
- `errors.go` — SCSIError, TransportError, AuthError

### Existing test infrastructure (patterns to follow)
- `test/target.go` — MockTarget pattern (handler registration, accept loop, serve loop)
- `test/conformance/` — Conformance test patterns (setupTarget helper, context.WithTimeout, table-driven)

### Configfs reference paths (from research)
- `/sys/kernel/config/target/iscsi/` — iSCSI target root
- `/sys/kernel/config/target/core/fileio_*/` — fileio backstore HBAs

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `test/target.go` MockTarget pattern — not directly reusable but establishes the test helper convention
- `test/conformance/*_test.go` — test structure patterns (setup helper, context timeout, cleanup)
- Public API already supports all needed options: CHAP, mutual CHAP, digests, initiator name

### Established Patterns
- `//go:build integration` tag used by gotgt stubs (we'll use `//go:build e2e` instead)
- External test packages (`package conformance_test`) for public API testing
- `t.Cleanup()` + `defer` for resource management in tests

### Integration Points
- E2E tests import `github.com/rkujawa/uiscsi` public API only
- `test/lio/` helper is a new internal test package, imported by E2E tests
- Kernel modules: `target_core_mod`, `iscsi_target_mod`, `target_core_file` must be loaded

</code_context>

<specifics>
## Specific Ideas

- Helper should fail fast with clear skip message when root/modules unavailable — not cryptic permission errors
- TestMain sweeps orphaned targets on entry using the IQN prefix — handles crashed previous runs gracefully
- Ephemeral port avoids conflicts with any real iSCSI target running on the dev machine

</specifics>

<deferred>
## Deferred Ideas

- CI workflow for E2E tests — future phase (needs investigation of self-hosted runners or alternative)
- uiscsi-ls E2E tests — not a priority for this phase
- Performance/stress testing — separate concern

</deferred>

---

*Phase: 09-lio-e2e-tests*
*Context gathered: 2026-04-02*
