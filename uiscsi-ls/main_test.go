package main

import (
	"flag"
	"testing"
)

func TestStringSlice(t *testing.T) {
	var s stringSlice
	if err := s.Set("10.0.0.1:3260"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	if err := s.Set("10.0.0.2:3260"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	if len(s) != 2 {
		t.Fatalf("len = %d, want 2", len(s))
	}
	if s[0] != "10.0.0.1:3260" {
		t.Errorf("s[0] = %q, want %q", s[0], "10.0.0.1:3260")
	}
	if s[1] != "10.0.0.2:3260" {
		t.Errorf("s[1] = %q, want %q", s[1], "10.0.0.2:3260")
	}
	str := s.String()
	if str == "" {
		t.Error("String() returned empty string")
	}
}

func TestPortalFlagRepeated(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var portals stringSlice
	fs.Var(&portals, "portal", "iSCSI target portal address")

	err := fs.Parse([]string{"--portal", "10.0.0.1:3260", "--portal", "10.0.0.2:3260"})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(portals) != 2 {
		t.Fatalf("len(portals) = %d, want 2", len(portals))
	}
	if portals[0] != "10.0.0.1:3260" {
		t.Errorf("portals[0] = %q, want %q", portals[0], "10.0.0.1:3260")
	}
	if portals[1] != "10.0.0.2:3260" {
		t.Errorf("portals[1] = %q, want %q", portals[1], "10.0.0.2:3260")
	}
}

func TestPortalFlagMissing(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	var portals stringSlice
	fs.Var(&portals, "portal", "iSCSI target portal address")

	err := fs.Parse([]string{})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(portals) != 0 {
		t.Fatalf("len(portals) = %d, want 0", len(portals))
	}
}
