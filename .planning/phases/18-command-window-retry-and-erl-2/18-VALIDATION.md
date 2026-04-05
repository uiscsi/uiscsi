---
phase: 18
slug: command-window-retry-and-erl-2
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-05
---

# Phase 18 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) |
| **Config file** | none — existing infrastructure from Phase 13 |
| **Quick run command** | `go test ./test/conformance/ -run 'TestCmdWindow_\|TestRetry_\|TestERL2_' -race -count=1 -timeout 60s` |
| **Full suite command** | `go test ./... -race -count=1 -timeout 120s` |
| **Estimated runtime** | ~30 seconds |

---

## Sampling Rate

- **After every task commit:** Run quick run command
- **After every plan wave:** Run full suite command
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 18-01-01 | 01 | 1 | CMDSEQ-04 | — | N/A | conformance | `go test ./test/conformance/ -run TestCmdWindow_Zero -race` | ❌ W0 | ⬜ pending |
| 18-01-02 | 01 | 1 | CMDSEQ-05 | — | N/A | conformance | `go test ./test/conformance/ -run TestCmdWindow_Large -race` | ❌ W0 | ⬜ pending |
| 18-01-03 | 01 | 1 | CMDSEQ-06 | — | N/A | conformance | `go test ./test/conformance/ -run TestCmdWindow_SizeOne -race` | ❌ W0 | ⬜ pending |
| 18-01-04 | 01 | 1 | CMDSEQ-09 | — | N/A | conformance | `go test ./test/conformance/ -run TestCmdWindow_MaxCmdSNClose -race` | ❌ W0 | ⬜ pending |
| 18-02-01 | 02 | 1 | CMDSEQ-07 | — | N/A | conformance | `go test ./test/conformance/ -run TestRetry_ -race` | ❌ W0 | ⬜ pending |
| 18-02-02 | 02 | 1 | CMDSEQ-08 | — | N/A | conformance | `go test ./test/conformance/ -run TestRetry_StatSNGap -race` | ❌ W0 | ⬜ pending |
| 18-03-01 | 03 | 2 | SESS-07 | — | N/A | conformance | `go test ./test/conformance/ -run TestERL2_ConnReplace -race` | ❌ W0 | ⬜ pending |
| 18-03-02 | 03 | 2 | SESS-08 | — | N/A | conformance | `go test ./test/conformance/ -run TestERL2_TaskReassign -race` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

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
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
