package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/rkujawa/uiscsi"
)

func TestResolveCHAP(t *testing.T) {
	tests := []struct {
		name       string
		flagUser   string
		flagSecret string
		envUser    string
		envSecret  string
		wantUser   string
		wantSecret string
	}{
		{
			name:       "flags set",
			flagUser:   "fuser",
			flagSecret: "fsecret",
			wantUser:   "fuser",
			wantSecret: "fsecret",
		},
		{
			name:       "env fallback",
			envUser:    "euser",
			envSecret:  "esecret",
			wantUser:   "euser",
			wantSecret: "esecret",
		},
		{
			name:       "flag precedence over env",
			flagUser:   "fuser",
			flagSecret: "fsecret",
			envUser:    "euser",
			envSecret:  "esecret",
			wantUser:   "fuser",
			wantSecret: "fsecret",
		},
		{
			name:       "both empty",
			wantUser:   "",
			wantSecret: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envUser != "" {
				t.Setenv("ISCSI_CHAP_USER", tc.envUser)
			} else {
				t.Setenv("ISCSI_CHAP_USER", "")
			}
			if tc.envSecret != "" {
				t.Setenv("ISCSI_CHAP_SECRET", tc.envSecret)
			} else {
				t.Setenv("ISCSI_CHAP_SECRET", "")
			}

			gotUser, gotSecret := resolveCHAP(tc.flagUser, tc.flagSecret)
			if gotUser != tc.wantUser {
				t.Errorf("user = %q, want %q", gotUser, tc.wantUser)
			}
			if gotSecret != tc.wantSecret {
				t.Errorf("secret = %q, want %q", gotSecret, tc.wantSecret)
			}
		})
	}
}

func TestProbePortalError(t *testing.T) {
	// Save original and restore after test.
	origDiscover := discoverFunc
	t.Cleanup(func() { discoverFunc = origDiscover })

	// Stub discoverFunc to always return an error.
	discoverFunc = func(ctx context.Context, addr string, opts ...uiscsi.Option) ([]uiscsi.Target, error) {
		return nil, fmt.Errorf("connection refused")
	}

	// Single portal error: PortalResult should carry the error.
	pr := probePortal(context.Background(), "unreachable:3260", nil)
	if pr.Portal != "unreachable:3260" {
		t.Errorf("Portal = %q, want %q", pr.Portal, "unreachable:3260")
	}
	if pr.Err == nil {
		t.Fatal("expected non-nil Err for unreachable portal")
	}
	if got := pr.Err.Error(); !contains(got, "connection refused") {
		t.Errorf("Err = %q, want it to contain %q", got, "connection refused")
	}

	// probeAll with two failing portals: both results must be returned,
	// proving the first error does not abort the second portal (CLI-06).
	results := probeAll(context.Background(), []string{"portal1:3260", "portal2:3260"}, nil)
	if len(results) != 2 {
		t.Fatalf("probeAll returned %d results, want 2", len(results))
	}
	for i, r := range results {
		if r.Err == nil {
			t.Errorf("results[%d].Err is nil, want non-nil", i)
		}
	}
}

func TestProbeAllWithInitiatorName(t *testing.T) {
	origDiscover := discoverFunc
	t.Cleanup(func() { discoverFunc = origDiscover })

	var receivedOpts []uiscsi.Option
	discoverFunc = func(ctx context.Context, addr string, opts ...uiscsi.Option) ([]uiscsi.Target, error) {
		receivedOpts = opts
		return nil, fmt.Errorf("expected failure")
	}

	opts := []uiscsi.Option{uiscsi.WithInitiatorName("iqn.2025-01.com.example:test")}
	results := probeAll(context.Background(), []string{"10.0.0.1:3260"}, opts)

	if len(results) != 1 {
		t.Fatalf("probeAll returned %d results, want 1", len(results))
	}
	if results[0].Err == nil {
		t.Fatal("expected non-nil Err")
	}
	// Verify the option was passed through to discoverFunc.
	if len(receivedOpts) != 1 {
		t.Fatalf("discoverFunc received %d opts, want 1", len(receivedOpts))
	}
}

// contains reports whether s contains substr (avoids strings import in test).
func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
