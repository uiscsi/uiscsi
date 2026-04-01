# E2E Testing Approaches for iSCSI Initiator Libraries

**Researched:** 2026-04-01
**Context:** uiscsi is a pure-userspace Go iSCSI initiator. Phase 6 used custom mock targets for error recovery testing. Phase 7 needs real target infrastructure for conformance/interoperability E2E tests.
**Overall confidence:** MEDIUM (mixed sources; some areas well-documented, others required inference)

---

## 1. How libiscsi Runs Conformance Tests (iscsi-test-cu)

**Confidence: HIGH** (official README, man page, SNIA presentation)

### Target Requirement

libiscsi's README explicitly states: **"These tests require that you have STGT version 1.0.58 or later installed to use as a target to test against."** STGT is the Linux SCSI Target Framework -- the same project as `tgt`/`tgtd`. The daemon `tgtd` is the runtime component of STGT.

### How It Works

`iscsi-test-cu` is a **target-agnostic initiator-side test tool**. It connects to any iSCSI target at a URL like `iscsi://127.0.0.1/iqn.example.test/1` and exercises the target with SCSI commands and iSCSI protocol operations. Despite the STGT recommendation in the README, the tool works against any compliant target -- tgtd is just the reference/recommended target.

### Test Structure

Tests are organized hierarchically as `Family.Suite.Test`:

- **SCSI family:** Tests SCSI commands (Read10, Read16, Write10, Write16, CompareAndWrite, TestUnitReady, Inquiry, etc.)
- **iSCSI family:** Tests iSCSI protocol layer (Residuals, DataDigest, HeaderDigest, etc.)
- **LINUX family:** Tests via SG_IO passthrough to local SCSI devices (requires root, real hardware)

### CI Integration

`iscsi-test-cu` produces CUnit XML output (`--xml` flag) for CI consumption. The typical CI flow is:

1. Start `tgtd` with a file-backed LUN
2. Run `iscsi-test-cu --test=ALL --xml iscsi://127.0.0.1/target/1`
3. Parse `CUnitAutomated-Results.xml`

### Key Insight for uiscsi

**iscsi-test-cu tests targets, not initiators.** It validates that a target correctly responds to protocol operations. For testing an initiator library like uiscsi, we need the reverse: a known-good target to test our initiator against. The value of iscsi-test-cu for us is:

1. **Study its test structure** to inform our own test design
2. **Use it to validate any target** we choose for E2E testing (verify the target is conformant before trusting it)
3. **Not directly runnable against uiscsi** -- it is an initiator testing targets, not a target testing initiators

### Sources

- [libiscsi README](https://github.com/sahlberg/libiscsi/blob/master/README.md) -- STGT requirement, test execution
- [iscsi-test-cu man page](https://github.com/sahlberg/libiscsi/blob/master/doc/iscsi-test-cu.1) -- test families, XML output
- [SNIA presentation on libiscsi testing](https://www.snia.org/educational-library/testing-iscsi-scsi-protocol-compliance-using-libiscsi-2013) -- methodology (PDF access blocked, referenced from SNIA catalog)

---

## 2. How open-iscsi Tests Its Initiator

**Confidence: LOW** (minimal public documentation on testing)

open-iscsi is a Linux kernel + userspace hybrid iSCSI initiator. Their testing approach is:

- **Travis CI** for build verification (`.travis.yml` in repo)
- A `/test` directory exists but is not well-documented
- **No visible automated E2E test infrastructure** in the public repository
- Testing appears to rely heavily on manual integration against real targets and on the UNH InterOperability Laboratory (UNH-IOL) for formal conformance testing

### UNH-IOL Testing

The [UNH-IOL iSCSI Testing Services](https://www.iol.unh.edu/testing/storage/iscsi) provides formal iSCSI conformance testing for both initiators and targets. This is the industry-standard certification path but is a lab service, not something you run in CI.

### Key Insight for uiscsi

open-iscsi's testing approach is not a useful model for us. They benefit from being the de facto Linux iSCSI stack (tested by every SAN vendor during qualification). Their testing is largely done by integration -- if it works with enterprise storage arrays, it works. We cannot rely on this; we need automated, reproducible E2E tests.

### Sources

- [open-iscsi GitHub](https://github.com/open-iscsi/open-iscsi) -- repository structure, Travis CI
- [UNH-IOL iSCSI Testing](https://www.iol.unh.edu/testing/storage/iscsi) -- formal conformance testing services

---

## 3. Containerized iSCSI Target Solutions

**Confidence: MEDIUM** (multiple Docker Hub images verified, but most are unmaintained)

### Available Docker Images

| Image | Target | Status | Notes |
|-------|--------|--------|-------|
| `fabiand/iscsi-target-tgtd` | tgtd | Old, unmaintained | Simple Alpine + tgtd |
| `kubevirt/iscsi-demo-target-tgtd` | tgtd | Used by KubeVirt | Includes demo images, CirrOS |
| `lionelnicolas/docker-targetd` | LIO (targetd) | Moderate activity | HTTP API for LVM + iSCSI |
| `sdsys/tgtd-container` | tgtd | Simple | Basic tgtd in container |
| `gostor/gotgt` | gotgt | Active | Available on Docker Hub |

### LIO vs tgtd in Containers

**LIO (kernel):** Requires `--privileged` or specific kernel module access from the container. Not truly portable. The container is just a management wrapper around the host kernel's LIO subsystem. Poor fit for CI environments without root/kernel access.

**tgtd (userspace):** Runs entirely in userspace. Can work in unprivileged containers (only needs TCP port binding). Better fit for CI, but still Linux-only (uses Linux-specific I/O paths internally).

### Key Insight for uiscsi

Containerized tgtd is the most practical option for CI-based E2E testing on Linux. However:
- None of these images are well-maintained
- Our development platform is NetBSD 10.1, where Docker is not available
- For local development, we need a non-container approach
- For CI (GitHub Actions on Linux), a tgtd container or direct tgtd install is viable

### Sources

- [Docker Hub: fabiand/iscsi-target-tgtd](https://hub.docker.com/r/fabiand/iscsi-target-tgtd)
- [kubevirt/iscsi-demo-target-tgtd](https://hub.docker.com/r/kubevirt/iscsi-demo-target-tgtd)
- [lionelnicolas/docker-targetd](https://github.com/lionelnicolas/docker-targetd)

---

## 4. tgtd (Linux SCSI Target Framework, Userspace)

**Confidence: HIGH** (well-documented, extensively used, libiscsi's reference target)

### Overview

tgtd is the daemon component of the Linux SCSI Target Framework (tgt/STGT). It is **entirely userspace** -- no kernel modules required. This is the same project libiscsi recommends for running iscsi-test-cu.

- **Repository:** [github.com/fujita/tgt](https://github.com/fujita/tgt)
- **License:** GPL-2.0
- **Language:** C
- **Commits:** 2,048+
- **Status:** Mature, low-activity maintenance mode (superseded by LIO for production use, but still functional and packaged in major distros)

### Setup Complexity via tgtadm CLI

Setup is straightforward and scriptable:

```bash
# Start daemon
tgtd -f &

# Create target
tgtadm --lld iscsi --op new --mode target --tid 1 \
  -T iqn.2026-04.com.uiscsi:test

# Create backing store (file-backed LUN)
dd if=/dev/zero of=/tmp/test-lun.img bs=1M count=100
tgtadm --lld iscsi --op new --mode logicalunit --tid 1 \
  --lun 1 -b /tmp/test-lun.img

# Allow all initiators
tgtadm --lld iscsi --op bind --mode target --tid 1 -I ALL
```

This is 4 commands. Easily wrappable in a Go `TestMain` or helper function using `os/exec`.

### Managing tgtd from Go

tgtadm is a CLI tool that communicates with tgtd over a Unix domain socket. Options for Go management:

1. **Shell out to tgtadm** (simplest): Use `os/exec` to run tgtadm commands. This is what `longhorn/go-iscsi-helper` does.
2. **Write a tgtadm protocol client**: tgtadm speaks a simple binary protocol over `/var/run/tgtd/tgtd.ipc`. Could be implemented in Go but probably not worth the effort for test infrastructure.
3. **Use static config file**: Write `/tmp/tgt-test.conf` and start tgtd with `--conf /tmp/tgt-test.conf`.

**Recommendation:** Shell out to tgtadm. The 4-command setup is trivial to manage via `os/exec`.

### Platform Limitations

**tgtd is Linux-only.** It uses Linux-specific I/O subsystems (epoll, libaio). It will not run on NetBSD. This means:
- Local development on NetBSD cannot use tgtd
- CI on Linux (GitHub Actions) can use tgtd
- This creates a split testing strategy: mock-based locally, tgtd-based in CI

### Sources

- [fujita/tgt GitHub](https://github.com/fujita/tgt)
- [tgtadm man page](https://linux.die.net/man/8/tgtadm)
- [Alpine Wiki: Linux iSCSI Target](https://wiki.alpinelinux.org/wiki/Linux_iSCSI_Target_(tgt))

---

## 5. gotgt with Instrumentation/Extensions

**Confidence: MEDIUM** (code structure known, but extensibility not tested firsthand)

### Current State of gotgt

- **Repository:** [github.com/gostor/gotgt](https://github.com/gostor/gotgt)
- **Stars:** 276, active (last update March 2026)
- **Latest release:** v0.2.2 (December 2022) -- no recent tagged releases
- **Language:** Go (same as uiscsi)

### Architecture (Relevant to Extension)

The `pkg/port/iscsit/` package contains 13 files with clear separation:
- `conn.go` -- connection handling
- `session.go` -- session management
- `login.go` / `logout.go` -- login/logout sequences
- `cmd.go` -- SCSI command processing
- `auth.go` -- CHAP authentication
- `iscsid.go` -- main daemon loop

The `pkg/scsi/` package handles SCSI command emulation (SPC, SBC).

### Extension Approach

Rather than replacing gotgt entirely, we could **fork and extend** it:

1. **Add TMF (Task Management Function) handling**: gotgt's TMF implementation is stubbed. We would need to implement ABORT TASK, ABORT TASK SET, LUN RESET, etc.
2. **Add error injection hooks**: Insert hooks in `conn.go` and `cmd.go` to simulate connection drops, delayed responses, CRC errors.
3. **Add SNACK support**: Not currently implemented. Would need additions to PDU parsing and response generation.
4. **Add ERL 1/2 support**: Current gotgt only partially supports ERL 0. Adding connection recovery (ERL 1) and session recovery (ERL 2) would be substantial work.

### Effort Estimate

| Feature | Effort | Value for Testing |
|---------|--------|-------------------|
| Error injection hooks | Low (1-2 days) | High -- simulates real-world failures |
| TMF stubs to working | Medium (1 week) | High -- needed for error recovery tests |
| SNACK support | High (2+ weeks) | Medium -- only needed for SNACK-specific tests |
| ERL 1/2 | Very High (weeks) | High -- but may be more effort than using tgtd |

### Pros and Cons

**Pros:**
- Same language (Go) -- can embed in-process, debug with Go tools
- No external process management needed
- Can add arbitrary instrumentation
- Already a test dependency in earlier phases

**Cons:**
- Significant effort to bring up to conformance level
- Maintaining a fork is ongoing work
- We would be testing against our own code, not a battle-tested target (circular validation risk)
- No guarantee our TMF/SNACK additions are correct -- we would need iscsi-test-cu to validate them

### Recommendation

**Use gotgt for basic functional tests only** (login, simple read/write, logout). Do NOT invest in making gotgt a conformance-grade target. For advanced protocol testing (TMF, error recovery, SNACK), use tgtd in CI.

### Sources

- [gostor/gotgt GitHub](https://github.com/gostor/gotgt)
- [gotgt Docker Hub](https://hub.docker.com/r/gostor/gotgt)

---

## 6. Other Pure-Userspace iSCSI Targets

**Confidence: MEDIUM**

### istgt (FreeBSD/NetBSD)

- **Origin:** FreeBSD iSCSI target, pre-dates FreeBSD's native kernel iSCSI (added in FreeBSD 10)
- **Language:** C
- **pkgsrc:** Available as `net/istgt` -- **works on NetBSD**
- **Version in pkgsrc:** 20150713nb2 (old, but functional)
- **Features:** RFC 3720 (predecessor to RFC 7143), SPC-3 LU emulation, CHAP, multi-path I/O, >2TB LBA
- **Platform:** FreeBSD, NetBSD, openSUSE, Debian
- **GitHub fork:** [elastocloud/istgt](https://github.com/elastocloud/istgt) (61 commits, 10 stars, unclear maintenance)

**This is significant for our NetBSD development environment.** istgt can run natively on NetBSD via pkgsrc, providing a local E2E test target without needing Linux or Docker.

### NetBSD Native iSCSI Target

- **pkgsrc:** `net/netbsd-iscsi-target`
- **Version:** 20111006 (very old)
- **Features:** Basic iSCSI target, mirroring, storage combining
- **Status:** Unmaintained, very old. Not recommended.

### Summary of Pure-Userspace Targets

| Target | Language | Platforms | Maintained | iSCSI Spec Level | Notes |
|--------|----------|-----------|------------|-------------------|-------|
| tgtd | C | Linux only | Low activity | Good (ERL 0) | libiscsi reference target |
| gotgt | Go | Cross-platform | Active | Partial ERL 0 | Embeddable, but limited conformance |
| istgt | C | BSD, Linux | Stale (2015) | RFC 3720 / SPC-3 | Works on NetBSD via pkgsrc |
| netbsd-iscsi-target | C | NetBSD | Dead (2011) | Basic | Not recommended |

### Sources

- [pkgsrc net/istgt](https://ftp.netbsd.org/pub/pkgsrc/current/pkgsrc/net/istgt/index.html)
- [pkgsrc net/netbsd-iscsi-target](https://ftp.netbsd.org/pub/pkgsrc/current/pkgsrc/net/netbsd-iscsi-target/index.html)
- [elastocloud/istgt GitHub](https://github.com/elastocloud/istgt)
- [FreshPorts istgt](https://www.freshports.org/net/istgt/)

---

## 7. SCST as a Target Option

**Confidence: HIGH** (well-documented project, clear architecture)

### Overview

SCST (Generic SCSI Target Subsystem for Linux) is a **kernel-based** SCSI target framework. Its iSCSI component (`iscsi-scst`) is a heavily reworked fork of IET (iSCSI Enterprise Target).

- **Repository:** [github.com/SCST-project/scst](https://github.com/SCST-project/scst)
- **Latest version:** 3.10.0-pre (December 2024)
- **License:** GPL-2.0

### Architecture

SCST is fundamentally **kernel-based**:
- Core SCST module runs in kernel space
- Target drivers (iscsi-scst, qla2x00t, srpt) are kernel modules
- Userspace components exist only for management (scstadmin) and the `scst_user` passthrough interface
- Requires kernel patching for optimal performance

### Why NOT SCST for uiscsi Testing

1. **Linux-only, kernel-dependent** -- requires building/loading kernel modules
2. **Complex setup** -- kernel module compilation, potential kernel patching
3. **Cannot run in standard CI containers** without privileged access
4. **Cannot run on NetBSD** at all
5. **Overkill** -- SCST is designed for production enterprise storage, not test targets
6. **Same category as LIO** -- kernel targets that need privileged environments

### When SCST Would Be Relevant

Only if testing against an enterprise-grade target is needed for performance benchmarking or advanced clustering features (persistent reservations, ALUA). This is not a Phase 7 concern.

### Sources

- [SCST project website](https://scst.sourceforge.net/)
- [SCST targets comparison](https://scst.sourceforge.net/comparison.html)
- [SCST GitHub](https://github.com/SCST-project/scst)

---

## Recommended E2E Testing Strategy

Based on the research above, here is the recommended multi-tier approach:

### Tier 1: In-Process gotgt (Local + CI)

**What:** Embed gotgt as an in-process iSCSI target in Go tests.
**Where:** Runs everywhere (NetBSD, Linux, CI).
**Tests:** Login/logout negotiation, basic read/write, CHAP authentication, simple error cases.
**Limitation:** Only tests what gotgt implements correctly (partial ERL 0, no TMF, no SNACK).

### Tier 2: tgtd in CI (Linux CI Only)

**What:** Start tgtd as a subprocess in GitHub Actions, run uiscsi against it.
**Where:** Linux CI only (GitHub Actions).
**Tests:** Full SCSI command coverage, TMF operations, digest negotiation, multi-connection sessions.
**Setup:** Install `tgt` package, create file-backed LUN via tgtadm, run tests.
**Validation:** Run `iscsi-test-cu` against the same tgtd instance to verify target conformance first.

### Tier 3: istgt for Local Development (NetBSD)

**What:** Install istgt from pkgsrc for local E2E testing on the development machine.
**Where:** NetBSD development environment.
**Tests:** Same as Tier 2 where istgt supports the operations.
**Caveat:** istgt is old (2015 vintage). Test what it supports; do not expect modern RFC 7143 features.

### Tier 4: Cross-Validation with iscsi-test-cu (CI)

**What:** Use iscsi-test-cu as a reference to validate our test expectations.
**Where:** Linux CI.
**How:** Run iscsi-test-cu against our Tier 2 target, compare results with what our initiator observes. If iscsi-test-cu says the target handles X correctly and our initiator fails on X, the bug is in our initiator.

### What NOT to Do

- **Do not invest in making gotgt conformance-grade.** The effort to add TMF/SNACK/ERL1+ to gotgt is comparable to writing a new target. Use tgtd instead.
- **Do not use SCST or LIO.** Kernel-based targets are wrong for CI and wrong for NetBSD.
- **Do not rely solely on gotgt.** It has known conformance gaps (103/209 libiscsi tests pass). Always cross-validate against tgtd.
- **Do not build a custom target from scratch.** Phase 6 already proved this is significant effort even for mocks. A real target is orders of magnitude more work.

### Implementation Sketch for CI

```yaml
# GitHub Actions workflow sketch
- name: Install tgtd
  run: sudo apt-get install -y tgt

- name: Create test LUN
  run: |
    sudo tgtd -f &
    sleep 1
    sudo tgtadm --lld iscsi --op new --mode target --tid 1 \
      -T iqn.2026-04.com.uiscsi:ci-test
    dd if=/dev/zero of=/tmp/test-lun.img bs=1M count=100
    sudo tgtadm --lld iscsi --op new --mode logicalunit --tid 1 \
      --lun 1 -b /tmp/test-lun.img
    sudo tgtadm --lld iscsi --op bind --mode target --tid 1 -I ALL

- name: Run E2E tests
  run: go test -v -tags=e2e ./test/e2e/...
```

### Go Test Helper Sketch

```go
// testutil/tgtd.go -- manages tgtd lifecycle for E2E tests
// Would use os/exec to start tgtd and configure via tgtadm
// Only runs on Linux (build tag: //go:build linux)
// Returns target URL for tests to connect to
```

---

## Open Questions

1. **istgt conformance level:** How many iscsi-test-cu tests pass against istgt? Need to verify before relying on it for NetBSD local testing.
2. **gotgt stability under load:** Phase 6 found gotgt insufficient for error recovery. Is it stable enough even for basic read/write E2E tests, or does it have reliability issues there too?
3. **tgtd in GitHub Actions:** Does the `tgt` package install cleanly on Ubuntu runners? Does tgtd require any special permissions beyond what Actions provides?
4. **Alternative: ganesanb4/gaern Go target:** Are there any other Go iSCSI targets besides gotgt that might be more complete? (Not found in research -- likely does not exist.)
5. **iscsi-test-cu as Go subprocess:** Could we build libiscsi/iscsi-test-cu in CI and run it against tgtd to generate a baseline, then run our tests against the same tgtd? This gives us a conformance reference.
