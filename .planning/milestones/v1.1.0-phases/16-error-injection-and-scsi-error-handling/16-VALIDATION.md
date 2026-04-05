---
phase: 16
slug: error-injection-and-scsi-error-handling
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-05
---

# Phase 16 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing (stdlib), Go 1.25 |
| **Config file** | None -- standard `go test` |
| **Quick run command** | `go test ./test/conformance/ -run 'TestError_\|TestSNACK_' -race -count=1 -v` |
| **Full suite command** | `go test ./... -race -count=1` |
| **Estimated runtime** | ~15 seconds (quick), ~30 seconds (full) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./test/conformance/ -run 'TestError_|TestSNACK_' -race -count=1`
- **After every plan wave:** Run `go test ./... -race -count=1`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 16-01-01 | 01 | 1 | — | — | N/A | unit | `go build ./test/...` | ❌ W0 | ⬜ pending |
| 16-01-02 | 01 | 1 | ERR-01 | — | N/A | conformance | `go test ./test/conformance/ -run TestError_CRCErrorSense -race -count=1` | ❌ W0 | ⬜ pending |
| 16-01-03 | 01 | 1 | ERR-02 | — | N/A | conformance | `go test ./test/conformance/ -run TestError_SNACKReject -race -count=1` | ❌ W0 | ⬜ pending |
| 16-01-04 | 01 | 1 | ERR-03 | — | N/A | conformance | `go test ./test/conformance/ -run TestError_UnexpectedUnsolicited -race -count=1` | ❌ W0 | ⬜ pending |
| 16-01-05 | 01 | 1 | ERR-04 | — | N/A | conformance | `go test ./test/conformance/ -run TestError_NotEnoughUnsolicited -race -count=1` | ❌ W0 | ⬜ pending |
| 16-01-06 | 01 | 1 | ERR-05 | — | N/A | conformance | `go test ./test/conformance/ -run TestError_BUSY -race -count=1` | ❌ W0 | ⬜ pending |
| 16-01-07 | 01 | 1 | ERR-06 | — | N/A | conformance | `go test ./test/conformance/ -run TestError_ReservationConflict -race -count=1` | ❌ W0 | ⬜ pending |
| 16-02-01 | 02 | 1 | SNACK-01 | — | N/A | conformance | `go test ./test/conformance/ -run TestSNACK_DataSNGap -race -count=1` | ❌ W0 | ⬜ pending |
| 16-02-02 | 02 | 1 | SNACK-02 | — | N/A | conformance | `go test ./test/conformance/ -run TestSNACK_DataACKWireFields -race -count=1` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `test/conformance/error_test.go` — covers ERR-01 through ERR-06
- [ ] `test/conformance/snack_test.go` — covers SNACK-01, SNACK-02
- [ ] `HandleSCSIWithStatus` helper in test/target.go

*Existing infrastructure covers test framework (go test) and PDU capture (pducapture).*

---

## Manual-Only Verifications

All phase behaviors have automated verification.

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
