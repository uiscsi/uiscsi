//go:build e2e

// Package lio provides helpers for setting up and tearing down Linux kernel
// LIO iSCSI targets via direct configfs manipulation. It is used exclusively
// by E2E tests that exercise the uiscsi public API against a real kernel
// iSCSI target. All operations require root privileges and loaded kernel
// modules (target_core_mod, iscsi_target_mod, target_core_file).
package lio

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	iscsiBase      = "/sys/kernel/config/target/iscsi"
	coreBase       = "/sys/kernel/config/target/core"
	IQNPrefix      = "iqn.2026-04.com.uiscsi.e2e:"
	shmDir         = "/dev/shm"
	backstoreHBA   = "iblock_0"
	defaultLUNSize = 64 * 1024 * 1024 // 64MB
)

// Config describes the LIO iSCSI target to create.
type Config struct {
	// TargetSuffix is appended to IQNPrefix for a unique target IQN.
	TargetSuffix string

	// InitiatorIQN is the initiator IQN for ACL creation.
	InitiatorIQN string

	// LUNs defines the LUNs to create. Each entry is a size in bytes.
	// If empty, one 64MB LUN is created.
	LUNs []int64

	// CHAPUser and CHAPPassword enable one-way CHAP when non-empty.
	CHAPUser     string
	CHAPPassword string

	// MutualUser and MutualPassword enable mutual CHAP when non-empty.
	MutualUser     string
	MutualPassword string
}

// Target describes a running LIO iSCSI target.
type Target struct {
	IQN  string // full target IQN
	Addr string // "127.0.0.1:<port>"
	Port int    // ephemeral port number
}

// RequireRoot skips the test with a clear message if not running as root.
func RequireRoot(t *testing.T) {
	t.Helper()
	if os.Getuid() != 0 {
		t.Skip("e2e tests require root (configfs writes need CAP_SYS_ADMIN)")
	}
}

// RequireModules skips the test if required kernel modules are not loaded.
func RequireModules(t *testing.T) {
	t.Helper()
	modules := []string{"target_core_mod", "iscsi_target_mod", "target_core_iblock"}
	data, err := os.ReadFile("/proc/modules")
	if err != nil {
		t.Skipf("cannot read /proc/modules: %v", err)
	}
	content := string(data)
	for _, mod := range modules {
		if !strings.Contains(content, mod) {
			t.Skipf("kernel module %s not loaded", mod)
		}
	}
}

// RequireConfigfs skips the test if the iSCSI configfs directory is not available.
func RequireConfigfs(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(iscsiBase); err != nil {
		t.Skip("configfs target/iscsi not available")
	}
}

// randomHex returns n random bytes encoded as hex.
func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// allocatePort finds an ephemeral port on 127.0.0.1 by opening and
// immediately closing a TCP listener.
func allocatePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate ephemeral port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

// writeConfigfs writes value to a configfs file. No trailing newline is added.
func writeConfigfs(path, value string) error {
	return os.WriteFile(path, []byte(value), 0o644)
}

// execCommand runs a command and returns its combined stdout+stderr output.
func execCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// setupState tracks all resources created during Setup for reverse-order teardown.
type setupState struct {
	// Resources in creation order (teardown reverses this).
	iqn            string
	port           int
	backstoreNames []string // one per LUN
	loopDevices    []string // loop device paths (e.g., /dev/loop0)
	shmPaths       []string // backing file paths
	initiatorIQN   string
	hasExplicitACL bool // true if explicit ACL was created (CHAP targets)
	lunCount       int
}

// Setup creates a real LIO iSCSI target via configfs. It returns the target
// information and a cleanup function that tears down all resources in strict
// reverse order. The cleanup function is NOT registered with t.Cleanup --
// callers control teardown timing.
//
// If Setup fails partway through resource creation, it tears down all
// partially-created resources before calling t.Fatalf. This prevents
// orphaned configfs entries since the caller's defer cleanup() would never
// run (t.Fatalf calls runtime.Goexit before Setup returns).
//
// Setup calls RequireRoot, RequireModules, and RequireConfigfs internally,
// skipping the test if prerequisites are not met.
func Setup(t *testing.T, cfg Config) (*Target, func()) {
	t.Helper()

	RequireRoot(t)
	RequireModules(t)
	RequireConfigfs(t)

	// Resolve LUN sizes.
	lunSizes := cfg.LUNs
	if len(lunSizes) == 0 {
		lunSizes = []int64{defaultLUNSize}
	}

	// Generate unique names.
	suffix := randomHex(4)
	iqn := IQNPrefix + cfg.TargetSuffix + "-" + suffix

	// Allocate ephemeral port.
	port := allocatePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	state := &setupState{
		iqn:          iqn,
		port:         port,
		initiatorIQN: cfg.InitiatorIQN,
		lunCount:     len(lunSizes),
	}

	// setupFatalf tears down partially-created resources before aborting.
	// This is necessary because the caller's defer cleanup() has not been
	// registered yet (Setup hasn't returned), so t.Fatalf alone would
	// orphan any configfs resources created so far.
	setupFatalf := func(format string, args ...any) {
		t.Helper()
		teardownState(state)
		t.Fatalf(format, args...)
	}

	// Step 1: Create iblock backstores via loop devices. The fileio backend
	// has a kernel bug on some versions (e.g., 6.19) where ReadCapacity
	// returns wrong capacity regardless of fd_dev_size. Using iblock +
	// loop devices avoids this — blockdev capacity is always correct.
	for i, size := range lunSizes {
		bsName := fmt.Sprintf("e2e-%s-lun%d", suffix, i)
		shmPath := filepath.Join(shmDir, bsName+".img")

		// Create backing file (must be non-sparse for correct block count).
		f, err := os.Create(shmPath)
		if err != nil {
			setupFatalf("create backing file %s: %v", shmPath, err)
		}
		if err := f.Truncate(size); err != nil {
			f.Close()
			setupFatalf("truncate backing file %s: %v", shmPath, err)
		}
		f.Close()

		// Create loop device.
		out, err := execCommand("losetup", "-f", "--show", shmPath)
		if err != nil {
			setupFatalf("losetup for %s: %v (output: %s)", shmPath, err, out)
		}
		loopDev := strings.TrimSpace(out)

		state.backstoreNames = append(state.backstoreNames, bsName)
		state.loopDevices = append(state.loopDevices, loopDev)
		state.shmPaths = append(state.shmPaths, shmPath)

		// Create iblock backstore in configfs.
		bsDir := filepath.Join(coreBase, backstoreHBA, bsName)
		if err := os.MkdirAll(bsDir, 0o755); err != nil {
			setupFatalf("mkdir backstore %s: %v", bsDir, err)
		}

		ctrl := fmt.Sprintf("udev_path=%s", loopDev)
		if err := writeConfigfs(filepath.Join(bsDir, "control"), ctrl); err != nil {
			setupFatalf("write backstore control: %v", err)
		}
		if err := writeConfigfs(filepath.Join(bsDir, "enable"), "1"); err != nil {
			setupFatalf("enable backstore: %v", err)
		}
	}

	// Step 2: Create iSCSI target IQN.
	iqnDir := filepath.Join(iscsiBase, iqn)
	if err := os.MkdirAll(iqnDir, 0o755); err != nil {
		setupFatalf("mkdir IQN %s: %v", iqnDir, err)
	}

	// Step 3: Create TPG.
	tpgDir := filepath.Join(iqnDir, "tpgt_1")
	if err := os.MkdirAll(tpgDir, 0o755); err != nil {
		setupFatalf("mkdir TPG: %v", err)
	}

	// Step 4: Create network portal.
	npDir := filepath.Join(tpgDir, "np", addr)
	if err := os.MkdirAll(npDir, 0o755); err != nil {
		setupFatalf("mkdir network portal %s: %v", npDir, err)
	}

	// Step 5: Create LUNs and link to backstores.
	for i, bsName := range state.backstoreNames {
		lunDir := filepath.Join(tpgDir, "lun", fmt.Sprintf("lun_%d", i))
		if err := os.MkdirAll(lunDir, 0o755); err != nil {
			setupFatalf("mkdir LUN %d: %v", i, err)
		}

		bsTarget := filepath.Join(coreBase, backstoreHBA, bsName)
		linkPath := filepath.Join(lunDir, "backstore")
		if err := os.Symlink(bsTarget, linkPath); err != nil {
			setupFatalf("symlink LUN %d to backstore: %v", i, err)
		}
	}

	// Step 6–8: ACL and authentication setup.
	//
	// For CHAP targets: create explicit ACL with CHAP credentials and
	// mapped LUNs (kernel auto-links by LUN number on mkdir — no manual
	// symlink needed on modern kernels).
	//
	// For non-CHAP targets: skip explicit ACLs entirely. Set
	// generate_node_acls=1 so the kernel auto-creates ACLs and maps
	// all LUNs for any connecting initiator.
	if cfg.CHAPUser != "" {
		state.hasExplicitACL = true

		// Create ACL for the specific initiator.
		aclDir := filepath.Join(tpgDir, "acls", cfg.InitiatorIQN)
		if err := os.MkdirAll(aclDir, 0o755); err != nil {
			setupFatalf("mkdir ACL %s: %v", aclDir, err)
		}

		// Create mapped LUNs in ACL. The kernel auto-links to the
		// matching TPG LUN by number — no explicit symlink needed.
		for i := range state.backstoreNames {
			mappedDir := filepath.Join(aclDir, fmt.Sprintf("lun_%d", i))
			if err := os.MkdirAll(mappedDir, 0o755); err != nil {
				setupFatalf("mkdir mapped LUN %d: %v", i, err)
			}
		}

		// Set CHAP credentials on the ACL.
		authDir := filepath.Join(aclDir, "auth")
		if err := writeConfigfs(filepath.Join(authDir, "userid"), cfg.CHAPUser); err != nil {
			setupFatalf("set CHAP userid: %v", err)
		}
		if err := writeConfigfs(filepath.Join(authDir, "password"), cfg.CHAPPassword); err != nil {
			setupFatalf("set CHAP password: %v", err)
		}

		// Mutual CHAP credentials.
		if cfg.MutualUser != "" {
			if err := writeConfigfs(filepath.Join(authDir, "userid_mutual"), cfg.MutualUser); err != nil {
				setupFatalf("set mutual CHAP userid: %v", err)
			}
			if err := writeConfigfs(filepath.Join(authDir, "password_mutual"), cfg.MutualPassword); err != nil {
				setupFatalf("set mutual CHAP password: %v", err)
			}
			// authenticate_target is read-only on some kernels (auto-set
			// when userid_mutual/password_mutual are written). Best-effort.
			_ = writeConfigfs(filepath.Join(authDir, "authenticate_target"), "1")
		}

		// Enforce CHAP authentication on the TPG — set AFTER credentials
		// are in place so the kernel sees valid auth config.
		if err := writeConfigfs(filepath.Join(tpgDir, "attrib", "authentication"), "1"); err != nil {
			setupFatalf("set authentication=1: %v", err)
		}
	} else {
		// No CHAP — use demo mode with auto-generated ACLs.
		if err := writeConfigfs(filepath.Join(tpgDir, "attrib", "authentication"), "0"); err != nil {
			setupFatalf("set authentication=0: %v", err)
		}
		if err := writeConfigfs(filepath.Join(tpgDir, "param", "AuthMethod"), "CHAP,None"); err != nil {
			setupFatalf("set AuthMethod=CHAP,None: %v", err)
		}
		if err := writeConfigfs(filepath.Join(tpgDir, "attrib", "generate_node_acls"), "1"); err != nil {
			setupFatalf("set generate_node_acls=1: %v", err)
		}
		if err := writeConfigfs(filepath.Join(tpgDir, "attrib", "demo_mode_write_protect"), "0"); err != nil {
			setupFatalf("set demo_mode_write_protect=0: %v", err)
		}
	}

	// Step 9: Enable TPG.
	if err := writeConfigfs(filepath.Join(tpgDir, "enable"), "1"); err != nil {
		setupFatalf("enable TPG: %v", err)
	}

	tgt := &Target{
		IQN:  iqn,
		Addr: addr,
		Port: port,
	}

	cleanup := func() {
		teardownState(state)
	}

	return tgt, cleanup
}

// teardownState tears down all configfs resources in strict reverse order.
// Errors are logged but not fatal -- best-effort cleanup.
func teardownState(st *setupState) {
	iqnDir := filepath.Join(iscsiBase, st.iqn)
	tpgDir := filepath.Join(iqnDir, "tpgt_1")
	addr := fmt.Sprintf("127.0.0.1:%d", st.port)

	// 1. Disable TPG.
	if err := writeConfigfs(filepath.Join(tpgDir, "enable"), "0"); err != nil {
		log.Printf("lio cleanup: disable TPG: %v", err)
	}

	// 2-3. Remove all ACLs (both explicit and auto-generated).
	aclsDir := filepath.Join(tpgDir, "acls")
	if acls, err := os.ReadDir(aclsDir); err == nil {
		for _, acl := range acls {
			aclDir := filepath.Join(aclsDir, acl.Name())
			// Remove mapped LUNs inside ACL.
			if luns, err := os.ReadDir(aclDir); err == nil {
				for _, l := range luns {
					if !strings.HasPrefix(l.Name(), "lun_") {
						continue
					}
					mappedDir := filepath.Join(aclDir, l.Name())
					removeSymlinksIn(mappedDir)
					if err := os.Remove(mappedDir); err != nil {
						log.Printf("lio cleanup: remove mapped LUN %s: %v", l.Name(), err)
					}
				}
			}
			if err := os.Remove(aclDir); err != nil {
				log.Printf("lio cleanup: remove ACL %s: %v", acl.Name(), err)
			}
		}
	}

	// 4-5. Remove LUN backstore symlinks and dirs.
	for i := range st.lunCount {
		lunDir := filepath.Join(tpgDir, "lun", fmt.Sprintf("lun_%d", i))
		removeSymlinksIn(lunDir)
		if err := os.Remove(lunDir); err != nil {
			log.Printf("lio cleanup: remove LUN %d: %v", i, err)
		}
	}

	// 6. Remove network portal.
	npDir := filepath.Join(tpgDir, "np", addr)
	if err := os.Remove(npDir); err != nil {
		log.Printf("lio cleanup: remove NP: %v", err)
	}

	// 7. Remove TPG.
	if err := os.Remove(tpgDir); err != nil {
		log.Printf("lio cleanup: remove TPG: %v", err)
	}

	// 8. Remove IQN.
	if err := os.Remove(iqnDir); err != nil {
		log.Printf("lio cleanup: remove IQN: %v", err)
	}

	// 9. Disable and remove backstores.
	for _, bsName := range st.backstoreNames {
		bsDir := filepath.Join(coreBase, backstoreHBA, bsName)
		// Disable before removal -- some kernels resist removing an
		// enabled backstore that had references.
		if err := writeConfigfs(filepath.Join(bsDir, "enable"), "0"); err != nil {
			log.Printf("lio cleanup: disable backstore %s: %v", bsName, err)
		}
		if err := os.Remove(bsDir); err != nil {
			log.Printf("lio cleanup: remove backstore %s: %v", bsName, err)
		}
	}

	// 10. Detach loop devices.
	for _, loopDev := range st.loopDevices {
		if _, err := execCommand("losetup", "-d", loopDev); err != nil {
			log.Printf("lio cleanup: detach loop device %s: %v", loopDev, err)
		}
	}

	// 11. Remove /dev/shm backing files.
	for _, shmPath := range st.shmPaths {
		if err := os.Remove(shmPath); err != nil {
			log.Printf("lio cleanup: remove backing file %s: %v", shmPath, err)
		}
	}
}

// removeSymlinksIn reads a directory and removes any symlinks found in it.
func removeSymlinksIn(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		fi, err := os.Lstat(p)
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			if err := os.Remove(p); err != nil {
				log.Printf("lio cleanup: remove symlink %s: %v", p, err)
			}
		}
	}
}
