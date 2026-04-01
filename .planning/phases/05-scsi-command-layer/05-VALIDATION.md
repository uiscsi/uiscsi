---
phase: 05
slug: scsi-command-layer
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-01
---

# Phase 05 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib) |
| **Config file** | none — standard Go testing |
| **Quick run command** | `go test ./internal/scsi/ -count=1 -race` |
| **Full suite command** | `go test ./... -count=1 -race` |
| **Estimated runtime** | ~10 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/scsi/ -count=1 -race`
- **After every plan wave:** Run `go test ./... -count=1 -race`
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 15 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 05-01-01 | 01 | 1 | SCSI-01,02,03,04,07,08,09 | unit | `go test ./internal/scsi/ -run TestCDB -race` | ❌ W0 | ⬜ pending |
| 05-01-02 | 01 | 1 | SCSI-10 | unit | `go test ./internal/scsi/ -run TestSense -race` | ❌ W0 | ⬜ pending |
| 05-01-03 | 01 | 1 | SCSI-03,18 | unit | `go test ./internal/scsi/ -run TestVPD -race` | ❌ W0 | ⬜ pending |
| 05-02-01 | 02 | 2 | SCSI-05,06 | unit | `go test ./internal/scsi/ -run TestRead -race` | ❌ W0 | ⬜ pending |
| 05-02-02 | 02 | 2 | SCSI-11,12,13,14 | unit | `go test ./internal/scsi/ -run "TestSync\|TestWriteSame\|TestUnmap\|TestVerify" -race` | ❌ W0 | ⬜ pending |
| 05-03-01 | 03 | 3 | SCSI-15,16 | unit | `go test ./internal/scsi/ -run TestReserv -race` | ❌ W0 | ⬜ pending |
| 05-03-02 | 03 | 3 | SCSI-17,19 | unit | `go test ./internal/scsi/ -run "TestCompare\|TestStartStop" -race` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/scsi/` directory created
- [ ] Test files created alongside source files (Go convention)

*Existing go test infrastructure covers all phase requirements.*

---

## Manual-Only Verifications

*All phase behaviors have automated verification. SCSI CDB building and response parsing are pure data transformations — fully testable with golden bytes.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 15s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
