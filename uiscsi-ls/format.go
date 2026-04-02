package main

import (
	"io"
)

// PortalResult holds discovery results for a single iSCSI portal.
type PortalResult struct {
	Portal  string         `json:"address"`
	Targets []TargetResult `json:"targets"`
	Err     error          `json:"-"`
}

// TargetResult holds discovery results for a single iSCSI target.
type TargetResult struct {
	IQN  string      `json:"iqn"`
	LUNs []LUNResult `json:"luns"`
	Err  error       `json:"-"`
}

// LUNResult holds SCSI inquiry and capacity data for a single LUN.
type LUNResult struct {
	LUN           uint64 `json:"lun"`
	DeviceType    uint8  `json:"device_type_code"`
	DeviceTypeS   string `json:"device_type"`
	Vendor        string `json:"vendor"`
	Product       string `json:"product"`
	Revision      string `json:"revision"`
	CapacityBytes uint64 `json:"capacity_bytes,omitempty"`
	BlockSize     uint32 `json:"block_size,omitempty"`
	LogicalBlocks uint64 `json:"capacity_blocks,omitempty"`
	CapacityStr   string `json:"-"`
}

// formatCapacity returns a human-readable SI capacity string.
func formatCapacity(blocks uint64, blockSize uint32) string {
	return "TODO"
}

// outputColumnar writes lsscsi-style tab-aligned output.
func outputColumnar(w io.Writer, results []PortalResult) {
}

// outputJSON writes machine-parseable JSON output.
func outputJSON(w io.Writer, results []PortalResult) {
}
