//go:build e2e

package lio

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// SweepOrphans removes any LIO targets with the E2E IQN prefix.
// Call from TestMain before running tests to clean up orphans from
// crashed previous runs. Errors are logged but not returned as fatal --
// orphan cleanup is best-effort.
func SweepOrphans() error {
	entries, err := os.ReadDir(iscsiBase)
	if err != nil {
		// configfs not available or not mounted -- nothing to sweep.
		return nil
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), IQNPrefix) {
			log.Printf("lio sweep: removing orphaned target %s", e.Name())
			if err := teardownTarget(e.Name()); err != nil {
				log.Printf("lio sweep: failed to remove %s: %v", e.Name(), err)
			}
		}
	}

	// Sweep orphaned backstores independently. A backstore may be orphaned
	// if the IQN was already removed (or never created) but the backstore
	// was left behind -- e.g., Setup failed between backstore creation and
	// IQN creation on a previous run.
	cleanOrphanBackstores()

	return nil
}

// teardownTarget performs best-effort teardown of a single iSCSI target
// IQN in configfs. It handles partial teardown where some components may
// not exist (e.g., crashed mid-setup).
func teardownTarget(iqn string) error {
	iqnDir := filepath.Join(iscsiBase, iqn)
	tpgDir := filepath.Join(iqnDir, "tpgt_1")

	// Disable TPG (ignore errors -- may not exist or already disabled).
	_ = writeConfigfs(filepath.Join(tpgDir, "enable"), "0")

	// Remove ACLs and their mapped LUNs.
	aclsDir := filepath.Join(tpgDir, "acls")
	if acls, err := os.ReadDir(aclsDir); err == nil {
		for _, acl := range acls {
			aclDir := filepath.Join(aclsDir, acl.Name())
			// Remove mapped LUNs inside ACL.
			removeLUNDirs(aclDir)
			_ = os.Remove(aclDir)
		}
	}

	// Remove TPG LUNs.
	lunBaseDir := filepath.Join(tpgDir, "lun")
	removeLUNDirs(lunBaseDir)

	// Remove network portals.
	npDir := filepath.Join(tpgDir, "np")
	if nps, err := os.ReadDir(npDir); err == nil {
		for _, np := range nps {
			_ = os.Remove(filepath.Join(npDir, np.Name()))
		}
	}

	// Remove TPG.
	_ = os.Remove(tpgDir)

	// Remove IQN.
	if err := os.Remove(iqnDir); err != nil {
		return fmt.Errorf("remove IQN %s: %w", iqn, err)
	}

	// Remove associated backstores. Scans rd_mcp_0 for all e2e- prefixed
	// entries -- intentionally broad to catch any orphans.
	cleanOrphanBackstores()

	return nil
}

// removeLUNDirs removes all lun_N directories inside a parent directory,
// first removing any symlinks within each.
func removeLUNDirs(parent string) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "lun_") {
			continue
		}
		lunDir := filepath.Join(parent, e.Name())
		removeSymlinksIn(lunDir)
		_ = os.Remove(lunDir)
	}
}

// cleanOrphanBackstores removes any backstores under iblock_0 that have
// the e2e- prefix. Also detaches orphaned loop devices and removes
// backing files.
func cleanOrphanBackstores() {
	hbaDir := filepath.Join(coreBase, backstoreHBA)
	entries, err := os.ReadDir(hbaDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "e2e-") {
			continue
		}
		bsDir := filepath.Join(hbaDir, e.Name())
		// Disable before removal -- some kernels resist removing an
		// enabled backstore that had references.
		_ = writeConfigfs(filepath.Join(bsDir, "enable"), "0")
		_ = os.Remove(bsDir)
	}

	// Clean orphaned backing files and loop devices.
	shmEntries, err := os.ReadDir(shmDir)
	if err != nil {
		return
	}
	for _, e := range shmEntries {
		if !strings.HasPrefix(e.Name(), "e2e-") || !strings.HasSuffix(e.Name(), ".img") {
			continue
		}
		shmPath := filepath.Join(shmDir, e.Name())
		// Find and detach any loop device using this file.
		out, _ := execCommand("losetup", "-j", shmPath)
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line == "" {
				continue
			}
			loopDev := strings.SplitN(line, ":", 2)[0]
			_, _ = execCommand("losetup", "-d", loopDev)
		}
		_ = os.Remove(shmPath)
	}
}
