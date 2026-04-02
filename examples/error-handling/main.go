// Command error-handling demonstrates typed error handling and recovery
// patterns with the uiscsi library.
//
// Usage:
//
//	error-handling <target-address> <target-iqn>
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/rkujawa/uiscsi"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: %s <target-address> <target-iqn>\n", os.Args[0])
		os.Exit(1)
	}
	addr := os.Args[1]
	iqn := os.Args[2]
	ctx := context.Background()

	// --- Example 1: TransportError handling ---
	// Dial to a bad address to trigger a transport error.
	fmt.Println("=== Transport Error ===")
	_, err := uiscsi.Dial(ctx, "192.0.2.1:3260", // RFC 5737 TEST-NET, guaranteed unreachable
		uiscsi.WithTarget(iqn),
	)
	if err != nil {
		var transportErr *uiscsi.TransportError
		if errors.As(err, &transportErr) {
			fmt.Printf("TransportError: op=%q err=%v\n", transportErr.Op, transportErr.Err)
		} else {
			fmt.Printf("unexpected error type: %T: %v\n", err, err)
		}
	}

	// --- Example 2: AuthError handling ---
	// Attempt CHAP with bad credentials to trigger an auth error.
	fmt.Println("\n=== Auth Error ===")
	_, err = uiscsi.Dial(ctx, addr,
		uiscsi.WithTarget(iqn),
		uiscsi.WithCHAP("baduser", "badsecret"),
	)
	if err != nil {
		var authErr *uiscsi.AuthError
		if errors.As(err, &authErr) {
			fmt.Printf("AuthError: class=%d detail=%d msg=%q\n",
				authErr.StatusClass, authErr.StatusDetail, authErr.Message)
		} else {
			// Target may not require CHAP, or connection may fail first.
			fmt.Printf("error (not AuthError): %T: %v\n", err, err)
		}
	}

	// --- Example 3: SCSIError handling ---
	// Connect successfully, then trigger a SCSI error.
	fmt.Println("\n=== SCSI Error ===")
	sess, err := uiscsi.Dial(ctx, addr,
		uiscsi.WithTarget(iqn),
	)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer sess.Close()

	// Read from a high LUN number that likely does not exist.
	_, err = sess.ReadBlocks(ctx, 255, 0, 1, 512)
	if err != nil {
		var scsiErr *uiscsi.SCSIError
		if errors.As(err, &scsiErr) {
			fmt.Printf("SCSIError: status=0x%02X sense_key=0x%02X ASC=0x%02X ASCQ=0x%02X\n",
				scsiErr.Status, scsiErr.SenseKey, scsiErr.ASC, scsiErr.ASCQ)
			fmt.Printf("  Message: %s\n", scsiErr.Message)

			// Interpret common sense keys.
			switch scsiErr.SenseKey {
			case 0x02:
				fmt.Println("  Interpretation: NOT READY -- LUN not available")
			case 0x05:
				fmt.Println("  Interpretation: ILLEGAL REQUEST -- invalid LUN or parameter")
			case 0x06:
				fmt.Println("  Interpretation: UNIT ATTENTION -- LUN state changed, retry")
			default:
				fmt.Printf("  Interpretation: sense key 0x%02X\n", scsiErr.SenseKey)
			}
		} else {
			fmt.Printf("error (not SCSIError): %T: %v\n", err, err)
		}
	}

	// --- Example 4: Recovery pattern ---
	// On UNIT ATTENTION (sense key 0x06), retry the command.
	fmt.Println("\n=== Recovery Pattern ===")
	fmt.Println("Retry logic for CHECK CONDITION / UNIT ATTENTION:")
	const maxRetries = 3
	for attempt := range maxRetries {
		err = sess.TestUnitReady(ctx, 0)
		if err == nil {
			fmt.Printf("  Attempt %d: success\n", attempt+1)
			break
		}
		var scsiErr *uiscsi.SCSIError
		if errors.As(err, &scsiErr) && scsiErr.SenseKey == 0x06 {
			fmt.Printf("  Attempt %d: UNIT ATTENTION, retrying...\n", attempt+1)
			continue
		}
		// Non-retryable error.
		fmt.Printf("  Attempt %d: non-retryable error: %v\n", attempt+1, err)
		break
	}
}
