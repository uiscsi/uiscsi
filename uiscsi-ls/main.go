// Command uiscsi-ls discovers iSCSI targets and lists their LUNs in an
// lsscsi-style columnar format (or JSON with --json).
//
// Usage:
//
//	uiscsi-ls --portal <addr> [--portal <addr2>] [--json] [--chap-user U] [--chap-secret S]
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/rkujawa/uiscsi"
)

// stringSlice implements flag.Value for a repeatable string flag.
type stringSlice []string

func (s *stringSlice) String() string { return fmt.Sprintf("%v", *s) }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func main() {
	var portals stringSlice
	flag.Var(&portals, "portal", "iSCSI target portal address (repeatable)")
	chapUser := flag.String("chap-user", "", "CHAP username (or ISCSI_CHAP_USER env)")
	chapSecret := flag.String("chap-secret", "", "CHAP secret (or ISCSI_CHAP_SECRET env)")
	jsonOutput := flag.Bool("json", false, "output as JSON")
	flag.Parse()

	if len(portals) == 0 {
		fmt.Fprintf(os.Stderr, "error: at least one --portal is required\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s --portal <addr> [--portal <addr2>] [--json] [--chap-user U] [--chap-secret S]\n", os.Args[0])
		os.Exit(1)
	}

	// Normalize portals (default port 3260).
	for i, p := range portals {
		portals[i] = normalizePortal(p)
	}

	// Resolve CHAP credentials: flags take precedence over env vars.
	user, secret := resolveCHAP(*chapUser, *chapSecret)
	var opts []uiscsi.Option
	if user != "" && secret != "" {
		opts = append(opts, uiscsi.WithCHAP(user, secret))
	}

	// Signal-based context cancellation (Ctrl+C / SIGTERM).
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	results := probeAll(ctx, portals, opts)

	// Output results.
	if *jsonOutput {
		outputJSON(os.Stdout, results)
	} else {
		outputColumnar(os.Stdout, results)
	}

	// Determine exit code.
	totalLUNs := 0
	for _, pr := range results {
		for _, tr := range pr.Targets {
			totalLUNs += len(tr.LUNs)
		}
	}
	if totalLUNs > 0 {
		os.Exit(0)
	}
	// All portals failed (or no LUNs found).
	os.Exit(2)
}
