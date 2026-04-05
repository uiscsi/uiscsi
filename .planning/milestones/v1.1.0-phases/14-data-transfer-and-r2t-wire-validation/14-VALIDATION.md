---
phase: 14
slug: data-transfer-and-r2t-wire-validation
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-05
---

# Phase 14 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing (stdlib), Go 1.25 |
| **Config file** | None needed -- `go test` with package path |
| **Quick run command** | `go test ./test/conformance/ -run "TestDataOut\|TestDataIn\|TestR2T" -count=1 -timeout 30s` |
| **Full suite command** | `go test ./test/conformance/ -count=1 -timeout 120s -race` |
| **Estimated runtime** | ~30 seconds (quick), ~120 seconds (full with race) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./test/conformance/ -run "TestDataOut|TestDataIn|TestR2T" -count=1 -timeout 30s`
- **After every plan wave:** Run `go test ./test/conformance/ -count=1 -timeout 120s -race`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 14-01-01 | 01 | 1 | — | — | N/A | unit | `go test ./test/ -run TestMockTarget -count=1` | ❌ W0 | ⬜ pending |
| 14-02-01 | 02 | 2 | DATA-01 | — | N/A | E2E conformance | `go test ./test/conformance/ -run TestDataOut_DataSN -count=1` | ❌ W0 | ⬜ pending |
| 14-02-02 | 02 | 2 | DATA-02 | — | N/A | E2E conformance | `go test ./test/conformance/ -run TestDataOut_UnsolicitedFirstBurst -count=1` | ❌ W0 | ⬜ pending |
| 14-02-03 | 02 | 2 | DATA-03 | — | N/A | E2E conformance | `go test ./test/conformance/ -run TestDataOut_NoUnsolicited -count=1` | ❌ W0 | ⬜ pending |
| 14-02-04 | 02 | 2 | DATA-04 | — | N/A | E2E conformance | `go test ./test/conformance/ -run TestDataOut_FirstBurstLimit -count=1` | ❌ W0 | ⬜ pending |
| 14-02-05 | 02 | 2 | DATA-05 | — | N/A | E2E conformance | `go test ./test/conformance/ -run TestDataOut_TTTEcho -count=1` | ❌ W0 | ⬜ pending |
| 14-02-06 | 02 | 2 | DATA-08 | — | N/A | E2E conformance | `go test ./test/conformance/ -run TestDataOut_MaxRecvDSL -count=1` | ❌ W0 | ⬜ pending |
| 14-02-07 | 02 | 2 | DATA-10 | — | N/A | E2E conformance | `go test ./test/conformance/ -run TestDataOut_FBitUnsolicited -count=1` | ❌ W0 | ⬜ pending |
| 14-02-08 | 02 | 2 | DATA-11 | — | N/A | E2E conformance | `go test ./test/conformance/ -run TestDataOut_FBitSolicited -count=1` | ❌ W0 | ⬜ pending |
| 14-02-09 | 02 | 2 | DATA-12 | — | N/A | E2E conformance | `go test ./test/conformance/ -run TestDataOut_DataSNPerR2T -count=1` | ❌ W0 | ⬜ pending |
| 14-02-10 | 02 | 2 | DATA-13 | — | N/A | E2E conformance | `go test ./test/conformance/ -run TestDataOut_BufferOffset -count=1` | ❌ W0 | ⬜ pending |
| 14-03-01 | 03 | 2 | DATA-06 | — | N/A | E2E conformance | `go test ./test/conformance/ -run TestDataIn_StatusInFinal -count=1` | ❌ W0 | ⬜ pending |
| 14-03-02 | 03 | 2 | DATA-07 | — | N/A | E2E conformance | `go test ./test/conformance/ -run TestDataIn_ABitDataACK -count=1` | ❌ W0 | ⬜ pending |
| 14-03-03 | 03 | 2 | DATA-09 | — | N/A | E2E conformance | `go test ./test/conformance/ -run TestDataIn_ZeroLength -count=1` | ❌ W0 | ⬜ pending |
| 14-03-04 | 03 | 2 | DATA-14 | — | N/A | E2E conformance | `go test ./test/conformance/ -run TestDataIn_EDTL -count=1` | ❌ W0 | ⬜ pending |
| 14-04-01 | 04 | 2 | R2T-01 | — | N/A | E2E conformance | `go test ./test/conformance/ -run TestR2T_SinglePDU -count=1` | ❌ W0 | ⬜ pending |
| 14-04-02 | 04 | 2 | R2T-02 | — | N/A | E2E conformance | `go test ./test/conformance/ -run TestR2T_MultiPDU -count=1` | ❌ W0 | ⬜ pending |
| 14-04-03 | 04 | 2 | R2T-03 | — | N/A | E2E conformance | `go test ./test/conformance/ -run TestR2T_MultipleR2T -count=1` | ❌ W0 | ⬜ pending |
| 14-04-04 | 04 | 2 | R2T-04 | — | N/A | E2E conformance | `go test ./test/conformance/ -run TestR2T_ParallelCommand -count=1` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `test/conformance/dataout_test.go` — covers DATA-01 through DATA-05, DATA-08, DATA-10 through DATA-13
- [ ] `test/conformance/datain_test.go` — covers DATA-06, DATA-07, DATA-09, DATA-14
- [ ] `test/conformance/r2t_test.go` — covers R2T-01 through R2T-04
- [ ] MockTarget NegotiationConfig + Data-Out handler in `test/target.go`

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
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
