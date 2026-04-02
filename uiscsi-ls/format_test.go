package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestFormatCapacity(t *testing.T) {
	tests := []struct {
		blocks    uint64
		blockSize uint32
		want      string
		contains  string
	}{
		{0, 512, "0B", ""},
		{1000, 512, "512000B", ""},
		{2048000, 512, "", "GB"},
		{2000000000, 512, "", "TB"},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("%d_%d", tt.blocks, tt.blockSize)
		t.Run(name, func(t *testing.T) {
			got := formatCapacity(tt.blocks, tt.blockSize)
			if tt.want != "" && got != tt.want {
				t.Errorf("formatCapacity(%d, %d) = %q, want %q", tt.blocks, tt.blockSize, got, tt.want)
			}
			if tt.contains != "" && !strings.Contains(got, tt.contains) {
				t.Errorf("formatCapacity(%d, %d) = %q, want containing %q", tt.blocks, tt.blockSize, got, tt.contains)
			}
		})
	}
}

func TestOutputColumnar(t *testing.T) {
	results := []PortalResult{
		{
			Portal: "10.0.0.1:3260",
			Targets: []TargetResult{
				{
					IQN: "iqn.2026.test:tgt1",
					LUNs: []LUNResult{
						{LUN: 0, DeviceType: 0x00, DeviceTypeS: "disk", Vendor: "VENDOR1", Product: "PRODUCT1", Revision: "1.0", CapacityStr: "100.00GB"},
						{LUN: 1, DeviceType: 0x05, DeviceTypeS: "cd/dvd", Vendor: "VENDOR2", Product: "CDROM", Revision: "2.0", CapacityStr: "-"},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	outputColumnar(&buf, results)
	out := buf.String()

	for _, want := range []string{
		"iqn.2026.test:tgt1",
		"10.0.0.1:3260",
		"LUN 0",
		"LUN 1",
		"disk",
		"cd/dvd",
		"VENDOR1",
		"PRODUCT1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("columnar output missing %q\nGot:\n%s", want, out)
		}
	}

	// Count non-empty lines.
	lines := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) != "" {
			lines++
		}
	}
	if lines != 2 {
		t.Errorf("expected 2 non-empty lines, got %d\nOutput:\n%s", lines, out)
	}
}

func TestOutputJSON(t *testing.T) {
	results := []PortalResult{
		{
			Portal: "10.0.0.1:3260",
			Targets: []TargetResult{
				{
					IQN: "iqn.2026.test:tgt1",
					LUNs: []LUNResult{
						{LUN: 0, DeviceType: 0x00, DeviceTypeS: "disk", Vendor: "VENDOR1", Product: "PRODUCT1", Revision: "1.0", CapacityStr: "100.00GB"},
						{LUN: 1, DeviceType: 0x05, DeviceTypeS: "cd/dvd", Vendor: "VENDOR2", Product: "CDROM", Revision: "2.0", CapacityStr: "-"},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	outputJSON(&buf, results)

	var parsed struct {
		Portals []PortalResult `json:"portals"`
	}
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON unmarshal failed: %v\nOutput:\n%s", err, buf.String())
	}
	if len(parsed.Portals) != 1 {
		t.Fatalf("expected 1 portal, got %d", len(parsed.Portals))
	}
	if parsed.Portals[0].Targets[0].IQN != "iqn.2026.test:tgt1" {
		t.Errorf("IQN = %q, want %q", parsed.Portals[0].Targets[0].IQN, "iqn.2026.test:tgt1")
	}
	if len(parsed.Portals[0].Targets[0].LUNs) != 2 {
		t.Fatalf("expected 2 LUNs, got %d", len(parsed.Portals[0].Targets[0].LUNs))
	}
	if parsed.Portals[0].Targets[0].LUNs[0].LUN != 0 {
		t.Errorf("LUN[0].LUN = %d, want 0", parsed.Portals[0].Targets[0].LUNs[0].LUN)
	}
	if parsed.Portals[0].Targets[0].LUNs[0].DeviceTypeS != "disk" {
		t.Errorf("LUN[0].DeviceTypeS = %q, want %q", parsed.Portals[0].Targets[0].LUNs[0].DeviceTypeS, "disk")
	}
}

func TestOutputColumnarErrorPortal(t *testing.T) {
	results := []PortalResult{
		{Portal: "bad:3260", Err: fmt.Errorf("connection refused")},
	}

	var buf bytes.Buffer
	outputColumnar(&buf, results)

	if buf.Len() != 0 {
		t.Errorf("expected empty stdout for error portal, got: %q", buf.String())
	}
}

