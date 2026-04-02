---
phase: 10
slug: e2e-test-coverage-expansion-unh-iol-compliance-gaps
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-02
---

# Phase 10 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` (Go 1.25) with `e2e` build tag |
| **Config file** | None (go test with build tags) |
| **Quick run command** | `sudo go test -tags e2e -v -count=1 ./test/e2e/ -run TestName` |
| **Full suite command** | `sudo go test -tags e2e -v -count=1 -timeout 300s ./test/e2e/` |
| **Estimated runtime** | ~60 seconds |

---

## Sampling Rate

- **After every task commit:** Run `sudo go test -tags e2e -v -count=1 ./test/e2e/ -run TestName`
- **After every plan wave:** Run `sudo go test -tags e2e -v -count=1 -timeout 300s ./test/e2e/`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 10-01-01 | 01 | 1 | E2E-11 | e2e | `sudo go test -tags e2e -v -count=1 ./test/e2e/ -run TestLargeWrite` | ❌ W0 | ⬜ pending |
| 10-02-01 | 02 | 1 | E2E-12 | e2e | `sudo go test -tags e2e -v -count=1 ./test/e2e/ -run TestNegotiation` | ❌ W0 | ⬜ pending |
| 10-03-01 | 03 | 2 | E2E-13 | e2e | `sudo go test -tags e2e -v -count=1 ./test/e2e/ -run TestERL1` | ❌ W0 | ⬜ pending |
| 10-03-02 | 03 | 2 | E2E-14 | e2e | `sudo go test -tags e2e -v -count=1 ./test/e2e/ -run TestERL2` | ❌ W0 | ⬜ pending |
| 10-04-01 | 04 | 2 | E2E-15 | e2e | `sudo go test -tags e2e -v -count=1 ./test/e2e/ -run TestTMF_AbortTask` | ❌ W0 | ⬜ pending |
| 10-04-02 | 04 | 2 | E2E-16 | e2e | `sudo go test -tags e2e -v -count=1 ./test/e2e/ -run TestTMF_TargetWarmReset` | ❌ W0 | ⬜ pending |
| 10-05-01 | 05 | 1 | E2E-17 | e2e | `sudo go test -tags e2e -v -count=1 ./test/e2e/ -run TestDigest_HeaderOnly` | ❌ W0 | ⬜ pending |
| 10-05-02 | 05 | 1 | E2E-18 | e2e | `sudo go test -tags e2e -v -count=1 ./test/e2e/ -run TestDigest_DataOnly` | ❌ W0 | ⬜ pending |
| 10-06-01 | 06 | 1 | E2E-19 | e2e | `sudo go test -tags e2e -v -count=1 ./test/e2e/ -run TestSCSIError` | ❌ W0 | ⬜ pending |
| 10-06-02 | 06 | 1 | E2E-20 | e2e | `go test -tags e2e -v -count=1 ./test/e2e/ -run TestBasicConnectivity` (non-root) | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `test/e2e/largewrite_test.go` — stubs for E2E-11
- [ ] `test/e2e/negotiation_test.go` — stubs for E2E-12
- [ ] `test/e2e/erl_test.go` — stubs for E2E-13, E2E-14
- [ ] `test/e2e/tmf_test.go` — stubs for E2E-15, E2E-16
- [ ] `test/e2e/digest_test.go` — stubs for E2E-17, E2E-18
- [ ] `test/e2e/scsierror_test.go` — stubs for E2E-19

*Existing infrastructure: `test/lio/lio.go` helper, `options.go` WithPDUHook/WithDigestConfig, E2E-20 skip logic already present.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| ERL 1 SNACK recovery | E2E-13 | LIO may not support ERL 1; test may t.Skip | If t.Skip triggered, verify kernel configfs `param/ErrorRecoveryLevel` write attempt was made |
| ERL 2 connection replace | E2E-14 | LIO may not support ERL 2; test may t.Skip | If t.Skip triggered, verify negotiation was attempted and target rejection documented |

*If ERL tests pass against LIO, these become fully automated.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
