package session

import (
	"testing"
)

func TestParseSendTargetsResponse(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		want    []DiscoveryTarget
	}{
		{
			name: "single target single portal",
			data: []byte("TargetName=iqn.2001-04.com.example:storage1\x00TargetAddress=10.0.0.1:3260,1\x00"),
			want: []DiscoveryTarget{
				{
					Name: "iqn.2001-04.com.example:storage1",
					Portals: []Portal{
						{Address: "10.0.0.1", Port: 3260, GroupTag: 1},
					},
				},
			},
		},
		{
			name: "single target multiple portals",
			data: []byte("TargetName=iqn.2001-04.com.example:storage1\x00TargetAddress=10.0.0.1:3260,1\x00TargetAddress=10.0.0.2:3260,2\x00"),
			want: []DiscoveryTarget{
				{
					Name: "iqn.2001-04.com.example:storage1",
					Portals: []Portal{
						{Address: "10.0.0.1", Port: 3260, GroupTag: 1},
						{Address: "10.0.0.2", Port: 3260, GroupTag: 2},
					},
				},
			},
		},
		{
			name: "multiple targets",
			data: []byte("TargetName=iqn.2001-04.com.example:storage1\x00TargetAddress=10.0.0.1:3260,1\x00TargetName=iqn.2001-04.com.example:storage2\x00TargetAddress=10.0.0.2:3260,1\x00TargetAddress=10.0.0.3:3261,2\x00"),
			want: []DiscoveryTarget{
				{
					Name: "iqn.2001-04.com.example:storage1",
					Portals: []Portal{
						{Address: "10.0.0.1", Port: 3260, GroupTag: 1},
					},
				},
				{
					Name: "iqn.2001-04.com.example:storage2",
					Portals: []Portal{
						{Address: "10.0.0.2", Port: 3260, GroupTag: 1},
						{Address: "10.0.0.3", Port: 3261, GroupTag: 2},
					},
				},
			},
		},
		{
			name: "empty input",
			data: nil,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSendTargetsResponse(tt.data)
			if len(got) != len(tt.want) {
				t.Fatalf("parseSendTargetsResponse() returned %d targets, want %d", len(got), len(tt.want))
			}
			for i, target := range got {
				if target.Name != tt.want[i].Name {
					t.Errorf("target[%d].Name = %q, want %q", i, target.Name, tt.want[i].Name)
				}
				if len(target.Portals) != len(tt.want[i].Portals) {
					t.Fatalf("target[%d].Portals has %d entries, want %d", i, len(target.Portals), len(tt.want[i].Portals))
				}
				for j, portal := range target.Portals {
					wantP := tt.want[i].Portals[j]
					if portal.Address != wantP.Address {
						t.Errorf("target[%d].Portals[%d].Address = %q, want %q", i, j, portal.Address, wantP.Address)
					}
					if portal.Port != wantP.Port {
						t.Errorf("target[%d].Portals[%d].Port = %d, want %d", i, j, portal.Port, wantP.Port)
					}
					if portal.GroupTag != wantP.GroupTag {
						t.Errorf("target[%d].Portals[%d].GroupTag = %d, want %d", i, j, portal.GroupTag, wantP.GroupTag)
					}
				}
			}
		})
	}
}

func TestParsePortal(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want Portal
	}{
		{
			name: "IPv4 with port and group tag",
			raw:  "10.0.0.1:3260,1",
			want: Portal{Address: "10.0.0.1", Port: 3260, GroupTag: 1},
		},
		{
			name: "IPv6 with port and group tag",
			raw:  "[2001:db8::1]:3260,1",
			want: Portal{Address: "2001:db8::1", Port: 3260, GroupTag: 1},
		},
		{
			name: "no port defaults to 3260",
			raw:  "10.0.0.1,1",
			want: Portal{Address: "10.0.0.1", Port: 3260, GroupTag: 1},
		},
		{
			name: "no group tag defaults to 1",
			raw:  "10.0.0.1:3260",
			want: Portal{Address: "10.0.0.1", Port: 3260, GroupTag: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePortal(tt.raw)
			if got.Address != tt.want.Address {
				t.Errorf("parsePortal(%q).Address = %q, want %q", tt.raw, got.Address, tt.want.Address)
			}
			if got.Port != tt.want.Port {
				t.Errorf("parsePortal(%q).Port = %d, want %d", tt.raw, got.Port, tt.want.Port)
			}
			if got.GroupTag != tt.want.GroupTag {
				t.Errorf("parsePortal(%q).GroupTag = %d, want %d", tt.raw, got.GroupTag, tt.want.GroupTag)
			}
		})
	}
}
