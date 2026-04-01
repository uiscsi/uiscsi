# E2E Target Infrastructure Research

**Domain:** iSCSI target management for E2E testing of a Go userspace initiator library
**Researched:** 2026-04-01
**Overall confidence:** MEDIUM-HIGH

## Executive Summary

There is no single, clean Go library that does exactly what we need: programmatically spin up an RFC-compliant iSCSI target for testing. The ecosystem is fragmented across shell-out wrappers, Linux-kernel-coupled tools, and a single pure-Go target (gotgt). After surveying all options, the recommendation is a **two-tier approach**: use gotgt as an in-process Go target for fast integration tests, and use tgtd (managed via a thin Go wrapper around the tgtadm IPC protocol) for conformance-grade E2E tests that need a battle-tested, RFC-compliant target.

## 1. Go Libraries That Wrap Linux LIO Target (targetcli/rtslib/configfs)

**Finding: No Go library exists for LIO/configfs management.** Confidence: HIGH

LIO is the Linux kernel's in-kernel iSCSI target. It is managed through:

- **configfs** (`/sys/kernel/config/target/`) -- direct filesystem manipulation
- **rtslib-fb** -- Python library wrapping configfs ([GitHub](https://github.com/open-iscsi/rtslib-fb))
- **targetcli-fb** -- Python CLI built on rtslib ([GitHub](https://github.com/open-iscsi/targetcli-fb))
- **targetd** -- Python daemon exposing JSON-RPC 2.0 API over HTTP on port 18700 ([GitHub](https://github.com/open-iscsi/targetd), [API docs](https://github.com/open-iscsi/targetd/blob/main/API.md))

No Go library wraps any of these. The options for Go code to manage LIO are:

| Approach | Complexity | Notes |
|----------|-----------|-------|
| Shell out to `targetcli` | Low | Fragile, requires targetcli installed, parsing text output |
| Direct configfs manipulation via `os.MkdirAll`/`os.WriteFile` | Medium | Write to `/sys/kernel/config/target/iscsi/...`. No library needed, just filesystem ops. Requires root and Linux kernel. |
| Talk to `targetd` via JSON-RPC 2.0 | Low-Medium | Clean API. Requires targetd daemon running. Only ~12 methods needed. |

**Recommendation for LIO path:** If LIO is needed, use targetd's JSON-RPC API. It is well-documented, stable, and trivial to call from Go (`net/http` + `encoding/json`). The API supports: `vol_create`, `vol_destroy`, `export_create` (maps volume to initiator IQN + LUN), `export_destroy`, `pool_list`, `initiator_set_auth` (CHAP). This covers everything needed for test setup/teardown.

**However:** LIO requires Linux kernel modules. This makes it unsuitable as the primary test target for a library that values cross-platform portability and zero-infrastructure testing.

## 2. Go Libraries That Wrap tgtd/tgtadm

### longhorn/go-iscsi-helper (the only real option)

**Confidence: HIGH** -- examined source code directly.

[GitHub](https://github.com/longhorn/go-iscsi-helper) | [target.go](https://github.com/longhorn/go-iscsi-helper/blob/master/iscsi/target.go)

This is Longhorn's Go wrapper around the `tgtadm` CLI. It shells out to `tgtadm` binary for every operation:

**Supported operations:**
- `CreateTarget(tid, name)` -- creates iSCSI target
- `DeleteTarget(tid)` -- removes target
- `AddLunBackedByFile(tid, lun, path)` -- adds file-backed LUN
- `BindInitiator(tid, initiator)` -- ACL control
- `StartDaemon()` -- launches tgtd process
- `ShutdownTgtd()` -- graceful shutdown
- `GetTargetTid(name)` -- lookup by name
- `GetTargetConnections(tid)` -- list sessions
- `ExpandLun(tid, lun, path)` -- resize (Longhorn extension)

**Limitations:**
- Shells out to `tgtadm` binary (requires tgt package installed)
- Longhorn-specific assumptions (Unix domain socket backing stores)
- Tightly coupled to Longhorn's instance-manager architecture
- No standalone usage documentation
- Would need significant adaptation for test use

### ogre0403/iscsi-target-api

[GitHub](https://github.com/ogre0403/iscsi-target-api)

A Go REST API wrapping tgtadm. Provides HTTP endpoints for target/LUN management. Uses tgtd backend only. Limited: one LUN per target. Unmaintained (last commit years old). Not suitable as a library dependency.

### Direct tgtadm IPC protocol (write our own)

**Confidence: MEDIUM** -- reverse-engineered from tgt source code.

tgtadm communicates with tgtd over a Unix domain socket (`/var/run/tgtd/socket.0`) using a simple binary protocol:

**Request struct (C):**
```c
struct tgtadm_req {
    enum tgtadm_mode mode;    // MODE_TARGET, MODE_DEVICE, MODE_ACCOUNT, etc.
    enum tgtadm_op op;        // OP_NEW, OP_DELETE, OP_SHOW, OP_BIND, OP_UNBIND, OP_UPDATE
    char lld[64];             // "iscsi"
    uint32_t len;             // total message length
    int32_t tid;              // target ID
    uint64_t sid;             // session ID
    uint64_t lun;             // logical unit number
    uint32_t cid;             // connection ID
    uint32_t host_no;
    uint32_t device_type;
    uint32_t ac_dir;          // ACCOUNT_TYPE_INCOMING / ACCOUNT_TYPE_OUTGOING
    uint32_t pack;
    uint32_t force;
};
// Followed by variable-length key=value parameter buffer
```

**Response struct (C):**
```c
struct tgtadm_rsp {
    uint32_t err;    // error code
    uint32_t len;    // response data length
};
// Followed by variable-length text data
```

**Complexity assessment:** Writing a Go client for this protocol is straightforward. The struct is ~112 bytes, fixed layout, no alignment tricks. Parameters are comma-separated `key=value` pairs appended after the header. A Go implementation would be ~200-300 lines. This eliminates the `tgtadm` binary dependency (only need `tgtd` daemon).

**Advantage over shelling out:** No exec overhead, no output parsing, direct error codes, no PATH dependency on tgtadm binary.

## 3. Kubernetes CSI Drivers -- How They Manage Targets

### kubernetes-csi/csi-lib-iscsi

[GitHub](https://github.com/kubernetes-csi/csi-lib-iscsi) | [DeepWiki](https://deepwiki.com/kubernetes-csi/csi-lib-iscsi/2.2-iscsiadm-interface)

**Initiator-side only.** Wraps `iscsiadm` CLI for discovery, login, logout. Does NOT manage targets. Not useful for our case.

### dell/goiscsi

[GitHub](https://github.com/dell/goiscsi) | [pkg.go.dev](https://pkg.go.dev/github.com/dell/goiscsi)

**Initiator-side only.** Wraps `iscsiadm` for discovery and login. Does NOT manage targets.

### dell/gobrick

[GitHub](https://github.com/dell/gobrick)

CSI driver abstraction. Uses goiscsi internally. **Initiator-side only.**

### kubernetes-retired/external-storage (targetd provisioner)

[GitHub](https://github.com/kubernetes-retired/external-storage/blob/master/iscsi/targetd/README.md)

**This is the interesting one.** Uses targetd's JSON-RPC 2.0 API to create volumes and export them as iSCSI LUNs. Written in Go. Now retired but demonstrates the pattern of talking to targetd from Go.

### Kubernetes e2e test images

Kubernetes ships an `iscsi-server` e2e test image (version 2.0) that runs tgtd inside a container. This validates the approach of containerized tgtd for testing.

### Summary of K8s ecosystem

Every Kubernetes iSCSI integration either:
1. Shells out to `iscsiadm` (initiator side)
2. Shells out to `tgtadm` (target side, Longhorn)
3. Calls targetd JSON-RPC API (target side, external-storage)
4. Requires pre-provisioned targets (most CSI drivers)

No one has written a native Go library for target management. The ecosystem confirms that shelling out or calling targetd is the standard approach.

## 4. libstoragemgmt Go Binding

**Finding: No Go binding exists.** Confidence: HIGH

libstoragemgmt ([GitHub](https://github.com/libstorage/libstoragemgmt)) is a C library with Python bindings. It provides a vendor-neutral API for managing SAN arrays. Despite earlier search results hinting at a Go binding, no such binding exists in the repository or on pkg.go.dev. The library is also overkill -- it abstracts over enterprise SAN arrays, not local test targets.

**Verdict:** Not applicable. Do not pursue.

## 5. targetcli-fb / rtslib Python API

[rtslib-fb GitHub](https://github.com/open-iscsi/rtslib-fb) | [targetcli-fb GitHub](https://github.com/open-iscsi/targetcli-fb)

rtslib-fb is the Python library that directly manipulates LIO's configfs interface. It is mature and well-maintained (latest release: rtslib-fb 2.2.3).

**Options for Go integration:**

| Approach | Effort | Reliability |
|----------|--------|-------------|
| Shell out to `targetcli` CLI | Low | Medium -- text parsing is fragile |
| Shell out to Python script using rtslib | Low-Medium | High -- full API access |
| Use targetd daemon (built on rtslib) | Low | High -- clean JSON-RPC API |
| Direct configfs writes from Go | Medium | High -- but kernel-coupled |

**Recommendation:** If LIO is the target, use targetd's JSON-RPC API rather than shelling out to targetcli. targetd provides `export_create(pool, vol, initiator_wwn, lun)` and `export_destroy()` which is all we need for test setup/teardown. Authentication is HTTP Basic Auth. Port 18700. Format is JSON-RPC 2.0 -- trivial from Go.

## 6. tgt Project (fujita/tgt) Analysis

[GitHub](https://github.com/fujita/tgt)

tgt is a userspace iSCSI target daemon. Key properties:

- **Fully userspace** -- no kernel modules needed (unlike LIO)
- **Mature** -- widely deployed, used by OpenStack, Longhorn, Kubernetes e2e tests
- **Simple architecture** -- single `tgtd` daemon, managed via `tgtadm` CLI or IPC socket
- **File-backed LUNs** -- can use regular files as backing store (perfect for tests)
- **CHAP support** -- incoming and outgoing authentication
- **Multi-LUN, multi-target** -- up to 4095 targets

### Go wrapper complexity assessment

Writing a Go wrapper that talks directly to tgtd's IPC socket:

**Required operations for E2E testing:**
1. Create target (MODE_TARGET, OP_NEW) -- ~10 lines
2. Delete target (MODE_TARGET, OP_DELETE) -- ~10 lines
3. Add LUN backed by file (MODE_DEVICE, OP_NEW, params: path, bstype) -- ~15 lines
4. Bind initiator ACL (MODE_TARGET, OP_BIND, params: initiator-address) -- ~10 lines
5. Show target (MODE_TARGET, OP_SHOW) -- ~15 lines for parsing
6. Set CHAP credentials (MODE_ACCOUNT, OP_NEW + OP_BIND) -- ~20 lines

**Shared infrastructure:**
- Socket connection: ~20 lines
- Request serialization: ~30 lines
- Response deserialization: ~20 lines
- Error mapping: ~15 lines

**Total estimate: 200-350 lines of Go code.** No CGO needed. No external dependencies. Uses `encoding/binary`, `net` (Unix socket), `bytes`.

**This is very doable** and would give us a clean, dependency-free Go interface to a battle-tested iSCSI target.

## 7. gotgt (Pure Go iSCSI Target)

[GitHub](https://github.com/gostor/gotgt) | [pkg.go.dev](https://pkg.go.dev/github.com/gostor/gotgt/pkg/port/iscsit)

### Embedding API

gotgt can be embedded in-process. The key types:

```go
// ISCSITarget wraps api.SCSITarget with iSCSI protocol handling
type ISCSITarget struct {
    TPGTs      map[uint16]*iSCSITPGT
    Sessions   map[string]*ISCSISession
    MaxSessions int
    Alias      string
}

// Create target, add LUNs, start listening
target := newISCSITarget(scsiTarget)
target.CreateLu(lu)
```

### Strengths
- Pure Go, in-process -- no daemon, no socket, no exec
- Same language as our library -- easy debugging
- File-backed LUNs via flat-file plugin
- Used in production by OpenEBS Jiva
- Active development (last commit March 2026)

### Weaknesses
- No stable releases (always "under heavy development")
- RFC compliance is incomplete/untested -- unknown which edge cases are handled
- Limited SCSI command support compared to real targets
- No conformance test suite
- API is not well-documented for embedding

### Verdict for E2E testing

gotgt is good for **fast integration tests** (in-process, no setup overhead) but risky for **conformance testing** (we don't know if gotgt correctly implements the RFC edge cases we're trying to verify in our initiator).

## Recommended Approach

### Tier 1: gotgt for Integration Tests (already in STACK.md)

Use gotgt embedded in-process for:
- Basic login/logout cycle verification
- SCSI command round-trip tests
- Session management tests
- Fast CI feedback (no daemon startup)

### Tier 2: tgtd with Go IPC Wrapper for Conformance E2E Tests

Write a thin Go package (~300 lines) that speaks tgtd's IPC protocol directly:

```
package tgtctl

// Connect to tgtd daemon via Unix socket
func Dial(socketPath string) (*Client, error)

// Target management
func (c *Client) CreateTarget(tid int, name string) error
func (c *Client) DeleteTarget(tid int) error
func (c *Client) ShowTarget(tid int) (*TargetInfo, error)

// LUN management
func (c *Client) AddFileBackedLUN(tid int, lun int, path string) error
func (c *Client) RemoveLUN(tid int, lun int) error

// Access control
func (c *Client) BindInitiator(tid int, initiatorIQN string) error
func (c *Client) UnbindInitiator(tid int, initiatorIQN string) error

// Authentication
func (c *Client) SetCHAPCredentials(tid int, user, password string) error
```

**Why tgtd over LIO:**
- Fully userspace -- no kernel modules
- Runs on any Linux (including containers, CI runners)
- Battle-tested by Kubernetes, OpenStack, Longhorn
- Simple IPC protocol (binary struct + key=value params over Unix socket)
- File-backed LUNs from regular files (no LVM, no block devices)
- Can run multiple instances on different sockets

**Why native IPC over shelling out to tgtadm:**
- No exec overhead per operation
- No text output parsing
- Direct error codes
- No PATH dependency on tgtadm binary
- Cleaner for test setup/teardown (connect once, issue multiple commands)

### Tier 3: targetd for LIO-based Testing (optional, defer)

If we later need to test against the Linux kernel's LIO target (for maximum RFC compliance verification), use targetd's JSON-RPC API. This is a ~100-line Go client (`net/http` + `encoding/json`). Defer this until/unless gotgt and tgtd prove insufficient.

## Test Infrastructure Architecture

```
                    Tier 1 (Integration)          Tier 2 (Conformance)
                    =====================         =====================
    Target:         gotgt (in-process)            tgtd (daemon)
    Management:     Go API (direct)               tgtctl (IPC socket)
    Transport:      net.Pipe() / loopback         TCP loopback :3260
    Backing store:  In-memory                     Temp file (os.CreateTemp)
    Setup time:     ~0ms                          ~50ms (daemon + create)
    Teardown:       GC                            Delete target + rm file
    CI suitability: Excellent (any OS)            Good (Linux + tgt pkg)
    RFC fidelity:   Unknown (gotgt bugs?)         High (tgtd is mature)
```

## Platform Considerations

| Platform | gotgt (Tier 1) | tgtd (Tier 2) | LIO/targetd (Tier 3) |
|----------|----------------|----------------|-----------------------|
| Linux | Yes | Yes | Yes |
| NetBSD | Yes (pure Go) | No (Linux-only) | No (Linux-only) |
| macOS (dev) | Yes (pure Go) | No | No |
| CI (Linux container) | Yes | Yes | Maybe (needs modules) |

Since the project runs on NetBSD, gotgt (Tier 1) is the only option that works everywhere. tgtd (Tier 2) is Linux-only but covers CI and dedicated E2E test environments. Use build tags to skip tgtd-based tests on non-Linux platforms.

## Existing Libraries Summary

| Library | Type | Backend | Go? | Target Mgmt? | Usable? |
|---------|------|---------|-----|--------------|---------|
| longhorn/go-iscsi-helper | CLI wrapper | tgtadm | Yes | Yes | Partially -- Longhorn-coupled |
| kubernetes-csi/csi-lib-iscsi | CLI wrapper | iscsiadm | Yes | No (initiator only) | No |
| dell/goiscsi | CLI wrapper | iscsiadm | Yes | No (initiator only) | No |
| dell/gobrick | Abstraction | goiscsi | Yes | No (initiator only) | No |
| ogre0403/iscsi-target-api | REST API | tgtadm | Yes | Yes | Unmaintained, limited |
| open-iscsi/rtslib-fb | Library | configfs | Python | Yes | Not Go |
| open-iscsi/targetd | Daemon | LIO/configfs | Python | Yes (JSON-RPC) | Easy to call from Go |
| gostor/gotgt | Library | Pure Go | Yes | Yes (embedded) | Yes -- primary option |
| libstoragemgmt | Library | Various | C (no Go) | Sort of | Not applicable |

## Action Items

1. **Continue using gotgt** for integration tests (already in stack)
2. **Write `tgtctl` package** (~300 lines) speaking tgtd IPC for Linux E2E tests
3. **Add build-tagged test file** (`*_linux_test.go`) for tgtd-based conformance tests
4. **Create test helper** that starts tgtd, creates target + file-backed LUN, runs test, tears down
5. **Defer LIO/targetd** unless tgtd proves insufficient for conformance verification
6. **Do NOT adopt** longhorn/go-iscsi-helper (too coupled) or dell/goiscsi (wrong side)

## Sources

- [longhorn/go-iscsi-helper](https://github.com/longhorn/go-iscsi-helper) -- Go tgtadm CLI wrapper (HIGH confidence, examined source)
- [fujita/tgt](https://github.com/fujita/tgt) -- tgtd userspace iSCSI target daemon (HIGH confidence, examined IPC protocol)
- [tgtadm.h](https://github.com/fujita/tgt/blob/master/usr/tgtadm.h) -- IPC protocol struct definitions (HIGH confidence, examined source)
- [tgtadm IPC gist](https://gist.github.com/dankrause/4634345) -- Python reimplementation of tgtadm IPC protocol (MEDIUM confidence)
- [gostor/gotgt](https://github.com/gostor/gotgt) -- Pure Go iSCSI target framework (MEDIUM confidence -- no stable releases)
- [open-iscsi/targetd](https://github.com/open-iscsi/targetd) -- LIO management daemon with JSON-RPC API (HIGH confidence)
- [targetd API.md](https://github.com/open-iscsi/targetd/blob/main/API.md) -- JSON-RPC 2.0 API documentation (HIGH confidence)
- [open-iscsi/rtslib-fb](https://github.com/open-iscsi/rtslib-fb) -- Python configfs wrapper for LIO (HIGH confidence)
- [open-iscsi/targetcli-fb](https://github.com/open-iscsi/targetcli-fb) -- CLI for LIO management (HIGH confidence)
- [kubernetes-csi/csi-lib-iscsi](https://github.com/kubernetes-csi/csi-lib-iscsi) -- K8s iSCSI initiator library (HIGH confidence)
- [dell/goiscsi](https://github.com/dell/goiscsi) -- Go iscsiadm wrapper (HIGH confidence)
- [dell/gobrick](https://github.com/dell/gobrick) -- Go CSI driver library (HIGH confidence)
- [ogre0403/iscsi-target-api](https://github.com/ogre0403/iscsi-target-api) -- Go REST API for tgtd (LOW confidence -- unmaintained)
- [libstorage/libstoragemgmt](https://github.com/libstorage/libstoragemgmt) -- C storage management library, no Go binding (HIGH confidence)
- [kubernetes-retired/external-storage targetd provisioner](https://github.com/kubernetes-retired/external-storage/blob/master/iscsi/targetd/README.md) -- K8s iSCSI provisioner using targetd (MEDIUM confidence -- retired)
- [Kubernetes e2e test images](https://github.com/kubernetes/kubernetes/tree/master/test/images) -- includes iscsi-server image running tgtd (MEDIUM confidence)
