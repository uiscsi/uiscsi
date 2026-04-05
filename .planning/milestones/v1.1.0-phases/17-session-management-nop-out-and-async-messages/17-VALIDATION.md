---
phase: 17
slug: session-management-nop-out-and-async-messages
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-05
---

# Phase 17 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) |
| **Config file** | none — existing infrastructure from Phase 13 |
| **Quick run command** | `go test ./test/conformance/ -run 'TestSession_\|TestNOPOut_\|TestAsync_' -race -count=1 -timeout 60s` |
| **Full suite command** | `go test ./... -race -count=1 -timeout 120s` |
| **Estimated runtime** | ~20 seconds |

---

## Sampling Rate

- **After every task commit:** Run quick run command
- **After every plan wave:** Run full suite command
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 20 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 17-01-01 | 01 | 1 | SESS-03 | — | N/A | conformance | `go test ./test/conformance/ -run TestNOPOut_PingResponse -race` | ❌ W0 | ⬜ pending |
| 17-01-02 | 01 | 1 | SESS-04 | — | N/A | conformance | `go test ./test/conformance/ -run TestNOPOut_PingRequest -race` | ❌ W0 | ⬜ pending |
| 17-01-03 | 01 | 1 | SESS-05 | — | N/A | conformance | `go test ./test/conformance/ -run TestNOPOut_ExpStatSN -race` | ❌ W0 | ⬜ pending |
| 17-02-01 | 02 | 1 | SESS-01 | — | N/A | conformance | `go test ./test/conformance/ -run TestSession_LogoutAfterAsync -race` | ❌ W0 | ⬜ pending |
| 17-02-02 | 02 | 1 | SESS-06 | — | N/A | conformance | `go test ./test/conformance/ -run TestSession_CleanLogout -race` | ❌ W0 | ⬜ pending |
| 17-03-01 | 03 | 1 | ASYNC-01 | — | N/A | conformance | `go test ./test/conformance/ -run TestAsync_LogoutRequest -race` | ❌ W0 | ⬜ pending |
| 17-03-02 | 03 | 1 | ASYNC-02 | — | N/A | conformance | `go test ./test/conformance/ -run TestAsync_ConnDrop -race` | ❌ W0 | ⬜ pending |
| 17-03-03 | 03 | 1 | ASYNC-03 | — | N/A | conformance | `go test ./test/conformance/ -run TestAsync_SessionDrop -race` | ❌ W0 | ⬜ pending |
| 17-03-04 | 03 | 1 | ASYNC-04 | — | N/A | N/A | `go test ./test/conformance/ -run TestAsync_Renegotiation -race` | ❌ W0 | ⬜ pending |

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
- [ ] Feedback latency < 20s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
