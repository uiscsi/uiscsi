# Phase 8: lsscsi-discovery-utility - Context

**Gathered:** 2026-04-02
**Status:** Ready for planning

<domain>
## Phase Boundary

Build a standalone CLI tool (`uiscsi-ls`) that performs iSCSI target discovery on specified portals and presents LUN information in a format similar to Linux `lsscsi`. The tool lives in its own Go module and imports `github.com/rkujawa/uiscsi` as a dependency.

</domain>

<decisions>
## Implementation Decisions

### Output format
- **D-01:** Default output is fixed-width columnar (lsscsi-style), one line per LUN showing: target IQN, portal, LUN number, device type, vendor, product, revision, capacity
- **D-02:** `--json` flag switches output to machine-parseable JSON for scripting

### CLI interface design
- **D-03:** Portal address specified via `--portal` flag (not positional argument)
- **D-04:** Multiple portals supported by repeating the flag: `--portal 10.0.0.1:3260 --portal 10.0.0.2:3260`
- **D-05:** CHAP authentication via `--chap-user` and `--chap-secret` flags with environment variable fallback (`ISCSI_CHAP_USER`, `ISCSI_CHAP_SECRET`). Flags take precedence over env vars.
- **D-06:** Default iSCSI port 3260 if port omitted from portal address

### Probe depth
- **D-07:** Always performs full probe: SendTargets discovery → connect to each target → ReportLuns → Inquiry + ReadCapacity per LUN
- **D-08:** No `--discover-only` flag — keep the tool simple, always full probe

### Binary placement
- **D-09:** Separate Go module (`uiscsi-ls`), imports `github.com/rkujawa/uiscsi` as external dependency
- **D-10:** Binary name: `uiscsi-ls`

### Claude's Discretion
- Column widths and alignment strategy for columnar output
- JSON structure (flat vs nested)
- Error handling for unreachable targets during multi-portal scan (skip and report vs fail fast)
- Exit codes
- Flag parsing library (stdlib `flag` vs other)

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Public API (what the CLI will call)
- `uiscsi.go` — Discover() for SendTargets, Dial() for target connection
- `session.go` — ReportLuns(), Inquiry(), ReadCapacity(), TestUnitReady(), Close()
- `options.go` — WithTarget(), WithCHAP(), functional options pattern
- `types.go` — Target, Portal, InquiryData, Capacity type definitions
- `errors.go` — SCSIError, TransportError, AuthError for error display

### Reference implementations
- Linux `lsscsi` output format — columnar layout inspiration

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `uiscsi.Discover()` — handles full SendTargets session lifecycle, returns []Target with portals
- `uiscsi.Dial()` with `WithTarget()` — connects and logs in to a specific target
- `Session.ReportLuns()` — returns []uint64 LUN list
- `Session.Inquiry()` — returns *InquiryData with DeviceType, VendorID, ProductID, Revision
- `Session.ReadCapacity()` — returns *Capacity with LBA, BlockSize, LogicalBlocks
- `examples/discover-read/main.go` — existing example showing Discover → Dial → ReadCapacity → ReadBlocks flow

### Established Patterns
- Functional options for configuration (WithTarget, WithCHAP, WithLogger)
- Context-based cancellation on all API calls
- Typed error hierarchy for error classification and display

### Integration Points
- Imports `github.com/rkujawa/uiscsi` as external module dependency
- No integration with internal packages — public API only

</code_context>

<specifics>
## Specific Ideas

- Output should feel like `lsscsi` — compact, one line per LUN, human-scannable
- Secrets should not appear in process list — env var fallback for CHAP credentials

</specifics>

<deferred>
## Deferred Ideas

None — discussion stayed within phase scope

</deferred>

---

*Phase: 08-lsscsi-discovery-utility*
*Context gathered: 2026-04-02*
