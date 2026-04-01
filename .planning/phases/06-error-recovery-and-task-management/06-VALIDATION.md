---
phase: 6
slug: error-recovery-and-task-management
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-01
---

# Phase 6 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` (Go 1.25) |
| **Config file** | None needed -- `go test` auto-discovers |
| **Quick run command** | `go test ./internal/session/ -run TestTMF -count=1` |
| **Full suite command** | `go test -race ./...` |
| **Estimated runtime** | ~15 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/session/ -count=1 -race`
- **After every plan wave:** Run `go test -race ./...`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 06-01-01 | 01 | 1 | TMF-01 | unit | `go test ./internal/session/ -run TestAbortTask -count=1` | Wave 0 | pending |
| 06-01-02 | 01 | 1 | TMF-02 | unit | `go test ./internal/session/ -run TestAbortTaskSet -count=1` | Wave 0 | pending |
| 06-01-03 | 01 | 1 | TMF-03 | unit | `go test ./internal/session/ -run TestLUNReset -count=1` | Wave 0 | pending |
| 06-01-04 | 01 | 1 | TMF-04 | unit | `go test ./internal/session/ -run TestTargetWarmReset -count=1` | Wave 0 | pending |
| 06-01-05 | 01 | 1 | TMF-05 | unit | `go test ./internal/session/ -run TestTargetColdReset -count=1` | Wave 0 | pending |
| 06-01-06 | 01 | 1 | TMF-06 | unit | `go test ./internal/session/ -run TestClearTaskSet -count=1` | Wave 0 | pending |
| 06-02-01 | 02 | 2 | ERL-01 | integration | `go test ./internal/session/ -run TestERL0Reconnect -count=1 -race` | Wave 0 | pending |
| 06-02-02 | 02 | 2 | ERL-02 | unit | `go test ./internal/session/ -run TestSNACK -count=1` | Wave 0 | pending |
| 06-02-03 | 02 | 2 | ERL-03 | integration | `go test ./internal/session/ -run TestERL2ConnReplace -count=1 -race` | Wave 0 | pending |
| 06-03-01 | 03 | 1 | TEST-05 | unit+integration | `go test ./internal/transport/ -run TestFaultConn -count=1 && go test ./internal/session/ -run TestErrorInjection -count=1 -race` | Wave 0 | pending |

*Status: pending / green / red / flaky*

---

## Wave 0 Requirements

- [ ] `internal/session/tmf_test.go` -- stubs for TMF-01 through TMF-06
- [ ] `internal/session/recovery_test.go` -- stubs for ERL-01
- [ ] `internal/session/snack_test.go` -- stubs for ERL-02
- [ ] `internal/session/connreplace_test.go` -- stubs for ERL-03
- [ ] `internal/transport/faultconn.go` -- faultConn test utility
- [ ] `internal/transport/faultconn_test.go` -- faultConn self-tests for TEST-05

---

## Manual-Only Verifications

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have automated verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
