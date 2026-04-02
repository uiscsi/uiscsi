---
status: awaiting_human_verify
trigger: "LIO E2E test setup leaves orphaned configfs targets when Setup() fails partway"
created: 2026-04-02T00:00:00Z
updated: 2026-04-02T00:00:00Z
---

## Current Focus

hypothesis: Setup() calls t.Fatalf on failure after creating partial resources, but cleanup func hasn't been registered yet (Setup hasn't returned). Also backstores not disabled before removal.
test: Code review confirms the pattern - every t.Fatalf in Setup after step 1 leaves orphans
expecting: Fix by calling teardownState before t.Fatalf on error
next_action: Implement fixes for both root causes

## Symptoms

expected: When E2E tests fail, all LIO configfs resources should be fully cleaned up.
actual: After test failures, targetcli shows orphaned targets with disabled TPGs.
errors: Orphaned targets accumulate in configfs after test failures.
reproduction: Run `sudo go test -tags e2e -v -count=1 ./test/e2e/` — tests fail and leave orphaned LIO targets.
started: Phase 9 just implemented. First time running against real kernel.

## Eliminated

(none yet)

## Evidence

- timestamp: 2026-04-02T00:00:00Z
  checked: test/lio/lio.go Setup() function
  found: Every step (1-9) calls t.Fatalf on error. After step 1 creates backstores, any failure orphans them because the cleanup func is only returned at line 302 and never runs.
  implication: Root cause 1 confirmed - Setup must teardown partial state before calling t.Fatalf.

- timestamp: 2026-04-02T00:00:00Z
  checked: test/lio/lio.go teardownState() and sweep.go cleanOrphanBackstores()
  found: Neither function writes "0" to backstore enable file before os.Remove(). Research doc line 162 shows this step.
  implication: Root cause 2 confirmed - backstores should be disabled before removal.

- timestamp: 2026-04-02T00:00:00Z
  checked: test/lio/sweep.go SweepOrphans()
  found: SweepOrphans iterates IQN prefix targets but cleanOrphanBackstores is only called from teardownTarget(iqn). However cleanOrphanBackstores already scans all "e2e-" prefixed entries independent of IQN, so standalone backstores ARE covered.
  implication: Sweep already handles orphan backstores, but they need disable step too.

## Resolution

root_cause: Setup() uses t.Fatalf for errors after partially creating configfs resources. Since t.Fatalf calls runtime.Goexit(), the cleanup function (returned by Setup) is never registered by the caller's defer. Partial resources (backstores, IQNs, TPGs, portals, LUNs, ACLs) are permanently orphaned. Secondary: backstores not disabled before removal.
fix: (1) Added setupFatalf closure in Setup() that calls teardownState(state) before t.Fatalf, ensuring partial resources are cleaned up on any setup failure. (2) Added backstore disable (write "0" to enable file) before os.Remove in teardownState() and cleanOrphanBackstores(). (3) SweepOrphans now calls cleanOrphanBackstores() independently after IQN sweep to catch backstores orphaned without an IQN. (4) Removed unused iqn parameter from cleanOrphanBackstores.
verification: go vet passes for both test/lio and test/e2e packages. Requires real kernel test for full verification.
files_changed: [test/lio/lio.go, test/lio/sweep.go]
