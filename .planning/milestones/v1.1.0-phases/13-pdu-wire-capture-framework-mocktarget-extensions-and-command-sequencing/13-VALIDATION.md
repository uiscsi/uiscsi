---
phase: 13
slug: pdu-wire-capture-framework-mocktarget-extensions-and-command-sequencing
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-04
---

# Phase 13 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) |
| **Config file** | none — existing test infrastructure |
| **Quick run command** | `go test -race -run TestCapture ./test/...` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test -race -run TestCapture ./test/...`
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 13-01-01 | 01 | 1 | CMDSEQ-01 | unit | `go test -race -run TestCapture ./test/...` | ❌ W0 | ⬜ pending |
| 13-01-02 | 01 | 1 | CMDSEQ-02 | unit | `go test -race -run TestCapture ./test/...` | ❌ W0 | ⬜ pending |
| 13-01-03 | 01 | 1 | CMDSEQ-03 | unit | `go test -race -run TestCapture ./test/...` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] PDU capture/assertion helper — new test infrastructure for field-level PDU queries
- [ ] MockTarget extensions — stateful session tracking, handler routing, command window control

*Existing test infrastructure (test/target.go, test/lio/) covers base protocol but not wire-level field assertions.*

---

## Manual-Only Verifications

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
