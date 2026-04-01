---
phase: 04
slug: write-path
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-01
---

# Phase 04 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) |
| **Config file** | none — Go test needs no config |
| **Quick run command** | `go test ./internal/session/...` |
| **Full suite command** | `go test -race -count=1 ./...` |
| **Estimated runtime** | ~75 seconds (session tests include keepalive timeouts) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/session/...`
- **After every plan wave:** Run `go test -race -count=1 ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 75 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 04-01-01 | 01 | 1 | WRITE-03 | unit | `go test ./internal/session/ -run TestImmediate` | ❌ W0 | ⬜ pending |
| 04-01-02 | 01 | 1 | WRITE-03 | unit | `go test ./internal/session/ -run TestSubmitWrite` | ❌ W0 | ⬜ pending |
| 04-01-03 | 01 | 1 | WRITE-03 | unit | `go test ./internal/session/ -run TestWriteResult` | ❌ W0 | ⬜ pending |
| 04-02-01 | 02 | 2 | WRITE-01, WRITE-02, WRITE-04, WRITE-05 | unit | `go test ./internal/session/ -run TestR2T` | ❌ W0 | ⬜ pending |
| 04-02-02 | 02 | 2 | WRITE-01, WRITE-04 | unit | `go test ./internal/session/ -run TestUnsolicited` | ❌ W0 | ⬜ pending |
| 04-03-01 | 03 | 3 | ALL | integration | `go test -race ./internal/session/ -run TestWriteMatrix` | ❌ W0 | ⬜ pending |
| 04-03-02 | 03 | 3 | WRITE-05 | integration | `go test -race ./internal/session/ -run TestBurst` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/session/dataout_test.go` — stubs for WRITE-01 through WRITE-05
- [ ] Existing test infrastructure covers framework needs (go test, testing/synctest)

*Existing infrastructure covers all phase requirements.*

---

## Manual-Only Verifications

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 75s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
