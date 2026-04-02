# Phase 8: lsscsi-discovery-utility - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-02
**Phase:** 08-lsscsi-discovery-utility
**Areas discussed:** Output format, CLI interface design, Probe depth, Binary placement

---

## Output Format

| Option | Description | Selected |
|--------|-------------|----------|
| lsscsi-style columns | Fixed-width columnar output, one line per LUN | |
| JSON output | Machine-parseable for scripting | |
| Both | Default columns, --json flag for JSON | ✓ |

**User's choice:** Both — default columnar, --json for machine output

---

## CLI Interface Design

### Portal addressing

| Option | Description | Selected |
|--------|-------------|----------|
| Positional address | `uiscsi-ls 192.168.1.100:3260` | |
| Flag-based address | `uiscsi-ls --portal 192.168.1.100:3260` | ✓ |
| Positional with optional port | `uiscsi-ls 192.168.1.100` | |

**User's choice:** Flag-based with `--portal`

### CHAP authentication

| Option | Description | Selected |
|--------|-------------|----------|
| Flags only | `--chap-user` and `--chap-secret` | |
| Env vars only | `ISCSI_CHAP_USER` / `ISCSI_CHAP_SECRET` | |
| Both | Flags with env var fallback, flags take precedence | ✓ |

**User's choice:** Both — flags with env var fallback for security

### Multiple portals

| Option | Description | Selected |
|--------|-------------|----------|
| Single portal only | Run multiple times for multiple portals | |
| Multiple portals | `--portal` repeated | ✓ |

**User's choice:** Multiple portals via repeated `--portal` flag

---

## Probe Depth

| Option | Description | Selected |
|--------|-------------|----------|
| Discovery only | Just list target IQNs and portals | |
| Full probe | Connect, ReportLuns, Inquiry + ReadCapacity per LUN | ✓ |
| Tiered | Default discovery, --probe for full | |

**User's choice:** Always full probe

### Discover-only flag

| Option | Description | Selected |
|--------|-------------|----------|
| Yes | `--discover-only` skips login/probe | |
| No | Always full probe, keep it simple | ✓ |

**User's choice:** No — keep it simple

---

## Binary Placement

| Option | Description | Selected |
|--------|-------------|----------|
| `cmd/uiscsi-ls/` | Inside existing module | |
| Separate module | Own go.mod, imports uiscsi | ✓ |
| `cmd/uiscsi-discover/` | Inside existing module, different name | |

**User's choice:** Separate module

### Binary name

| Option | Description | Selected |
|--------|-------------|----------|
| `uiscsi-ls` | Short, echoes lsscsi | ✓ |
| `uiscsi-discover` | Descriptive | |

**User's choice:** `uiscsi-ls`

---

## Claude's Discretion

- Column widths and alignment
- JSON structure
- Error handling for unreachable targets during multi-portal scan
- Exit codes
- Flag parsing library
