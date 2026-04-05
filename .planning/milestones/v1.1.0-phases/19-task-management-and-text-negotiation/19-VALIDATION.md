---
phase: 19
slug: task-management-and-text-negotiation
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-05
---

# Phase 19 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) |
| **Config file** | none — existing infrastructure |
| **Quick run command** | `go test ./test/conformance/ -run 'TestTMF\|TestText' -race -count=1 -timeout 60s` |
| **Full suite command** | `go test ./... -race -count=1 -timeout 120s` |
| **Estimated runtime** | ~40 seconds |

---

## Sampling Rate

- **After every task commit:** Run quick run command
- **After every plan wave:** Run full suite command
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 40 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 19-01-01 | 01 | 1 | TMF-01,02,03 | conformance | `go test ./test/conformance/ -run TestTMF -race -count=1` | ❌ W0 | ⬜ pending |
| 19-01-02 | 01 | 1 | TMF-04,05,06 | conformance | `go test ./test/conformance/ -run TestTMF_AbortTaskSet -race -count=1` | ❌ W0 | ⬜ pending |
| 19-02-01 | 02 | 1 | TEXT-01,02,03 | conformance | `go test ./test/conformance/ -run TestText -race -count=1` | ❌ W0 | ⬜ pending |
| 19-02-02 | 02 | 1 | TEXT-04,05,06 | conformance | `go test ./test/conformance/ -run TestText -race -count=1` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers all phase requirements:
- `test/target.go` — MockTarget with HandleTMF(), HandleText()
- `test/pducapture/capture.go` — PDU capture recorder
- `internal/session/tmf.go` — TMF production code
- `internal/session/discovery.go` — Text/SendTargets production code

---

## Manual-Only Verifications

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 40s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
