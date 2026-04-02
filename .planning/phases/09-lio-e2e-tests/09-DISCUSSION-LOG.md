# Phase 9: lio-e2e-tests - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-02
**Phase:** 09-lio-e2e-tests
**Areas discussed:** Helper API design, E2E test scope, Build tag and module structure, Cleanup strategy

---

## Helper API Design

| Option | Description | Selected |
|--------|-------------|----------|
| Struct-based config | `lio.Target{...}` then `target.Create()` / `target.Destroy()` | |
| Builder pattern | `lio.NewTarget(...).WithLUN(...).Create()` | |
| Simple functions | `lio.Setup(t, lio.Config{...})` returns cleanup func | ✓ |

**User's choice:** Simple functions with testing.T integration

### Cleanup func vs t.Cleanup

| Option | Description | Selected |
|--------|-------------|----------|
| t.Cleanup() only | Automatic teardown, no way to forget | |
| Return cleanup func | Caller controls timing, can inspect state after removal | ✓ |

**User's choice:** Return explicit cleanup func

---

## E2E Test Scope

| Option | Description | Selected |
|--------|-------------|----------|
| All 7 scenarios | Basic, data integrity, CHAP, digests, multi-LUN, TMF, error recovery | ✓ |
| Trim some | Subset for first pass | |

**User's choice:** All 7 scenarios

---

## Build Tag and Module Structure

| Option | Description | Selected |
|--------|-------------|----------|
| `//go:build e2e` on everything | Both helper and tests gated | ✓ |
| `//go:build linux` on helper, `e2e` on tests | Helper compiles on any Linux | |
| `//go:build e2e,linux` on both | Redundant double tag | |

**User's choice:** `//go:build e2e` on everything

---

## Cleanup Strategy

| Option | Description | Selected |
|--------|-------------|----------|
| Prefix-based cleanup | Fixed IQN prefix, TestMain sweep on entry | ✓ |
| PID-based tracking | Track test PID in temp file | |
| Unique random suffix | Random per run, sweep all matching | |

**User's choice:** Prefix-based with TestMain sweep

---

## Claude's Discretion

- Config struct field names and layout
- Root/module detection and skip strategy
- Ephemeral port allocation
- Test file organization
- Fileio backstore size
- Connection drop mechanism
