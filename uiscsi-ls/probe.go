package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rkujawa/uiscsi"
)

// discoverFunc is a package-level variable wrapping uiscsi.Discover so tests
// can replace it with a stub. The signature matches uiscsi.Discover.
var discoverFunc = uiscsi.Discover

// dialFunc is a package-level variable wrapping uiscsi.Dial so tests can
// replace it with a stub.
var dialFunc = uiscsi.Dial

// resolveCHAP returns CHAP credentials resolved from explicit flag values
// first, falling back to ISCSI_CHAP_USER / ISCSI_CHAP_SECRET environment
// variables.
func resolveCHAP(flagUser, flagSecret string) (string, string) {
	user := flagUser
	if user == "" {
		user = os.Getenv("ISCSI_CHAP_USER")
	}
	secret := flagSecret
	if secret == "" {
		secret = os.Getenv("ISCSI_CHAP_SECRET")
	}
	return user, secret
}

// probeAll runs probePortal for each portal sequentially and returns all
// results. Errors on individual portals do not abort the remaining portals.
func probeAll(ctx context.Context, portals []string, opts []uiscsi.Option) []PortalResult {
	results := make([]PortalResult, 0, len(portals))
	for _, p := range portals {
		results = append(results, probePortal(ctx, p, opts))
	}
	return results
}

// probePortal runs the full discovery and LUN probe pipeline for a single
// portal: Discover -> Dial per target -> ReportLuns -> Inquiry + ReadCapacity
// per LUN.
func probePortal(ctx context.Context, portal string, opts []uiscsi.Option) PortalResult {
	pCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	targets, err := discoverFunc(pCtx, portal, opts...)
	if err != nil {
		return PortalResult{Portal: portal, Err: fmt.Errorf("discover %s: %w", portal, err)}
	}

	pr := PortalResult{Portal: portal}
	for _, t := range targets {
		tr := probeTarget(pCtx, portal, t, opts)
		pr.Targets = append(pr.Targets, tr)
	}
	return pr
}

// probeTarget dials a single target and probes all its LUNs.
func probeTarget(ctx context.Context, portal string, t uiscsi.Target, opts []uiscsi.Option) TargetResult {
	sessOpts := make([]uiscsi.Option, 0, len(opts)+1)
	sessOpts = append(sessOpts, opts...)
	sessOpts = append(sessOpts, uiscsi.WithTarget(t.Name))

	sess, err := dialFunc(ctx, portal, sessOpts...)
	if err != nil {
		return TargetResult{IQN: t.Name, Err: fmt.Errorf("dial %s: %w", t.Name, err)}
	}
	defer sess.Close()

	luns, err := sess.ReportLuns(ctx)
	if err != nil {
		return TargetResult{IQN: t.Name, Err: fmt.Errorf("report luns %s: %w", t.Name, err)}
	}

	tr := TargetResult{IQN: t.Name}
	for _, lun := range luns {
		lr := probeLUN(ctx, sess, lun)
		tr.LUNs = append(tr.LUNs, lr)
	}
	return tr
}

// probeLUN runs Inquiry and (conditionally) ReadCapacity for a single LUN.
// The lun parameter is a raw SAM-encoded 8-byte value from REPORT LUNS.
// We decode it for display but pass the raw value to SCSI commands.
func probeLUN(ctx context.Context, sess *uiscsi.Session, lun uint64) LUNResult {
	lr := LUNResult{LUN: uiscsi.DecodeLUN(lun)}

	inq, err := sess.Inquiry(ctx, lun)
	if err != nil {
		lr.CapacityStr = "-"
		return lr
	}

	lr.DeviceType = inq.DeviceType
	lr.DeviceTypeS = uiscsi.DeviceTypeName(inq.DeviceType)
	lr.Vendor = strings.TrimSpace(inq.VendorID)
	lr.Product = strings.TrimSpace(inq.ProductID)
	lr.Revision = strings.TrimSpace(inq.Revision)

	// Only call ReadCapacity for disk device types (0x00 direct-access,
	// 0x0E simplified direct-access).
	if inq.DeviceType != 0x00 && inq.DeviceType != 0x0E {
		lr.CapacityStr = "-"
		return lr
	}

	cap, err := sess.ReadCapacity(ctx, lun)
	if err != nil {
		lr.CapacityStr = "-"
		return lr
	}

	lr.CapacityBytes = cap.LogicalBlocks * uint64(cap.BlockSize)
	lr.BlockSize = cap.BlockSize
	lr.LogicalBlocks = cap.LogicalBlocks
	lr.CapacityStr = formatCapacity(cap.LogicalBlocks, cap.BlockSize)
	return lr
}
