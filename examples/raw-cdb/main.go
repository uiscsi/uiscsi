// Command raw-cdb demonstrates sending raw SCSI CDBs via the Execute
// pass-through API for custom or vendor-specific SCSI commands.
//
// Usage:
//
//	raw-cdb <target-address> <target-iqn>
package main

import (
	"context"
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

	// Connect to target.
	sess, err := uiscsi.Dial(ctx, addr,
		uiscsi.WithTarget(iqn),
	)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer sess.Close()

	// Example 1: TEST UNIT READY (opcode 0x00, 6-byte CDB).
	// No data transfer -- just checks if the LUN is ready.
	turCDB := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	result, err := sess.Execute(ctx, 0, turCDB)
	if err != nil {
		log.Fatalf("TEST UNIT READY: %v", err)
	}
	fmt.Printf("TEST UNIT READY: status=0x%02X\n", result.Status)

	// Example 2: INQUIRY (opcode 0x12, 6-byte CDB).
	// Requests up to 255 bytes of inquiry data.
	inquiryCDB := []byte{0x12, 0x00, 0x00, 0x00, 0xFF, 0x00}
	result, err = sess.Execute(ctx, 0, inquiryCDB,
		uiscsi.WithDataIn(255),
	)
	if err != nil {
		log.Fatalf("INQUIRY: %v", err)
	}
	fmt.Printf("INQUIRY: status=0x%02X, %d bytes returned\n", result.Status, len(result.Data))
	if len(result.Data) >= 36 {
		vendor := string(result.Data[8:16])
		product := string(result.Data[16:32])
		revision := string(result.Data[32:36])
		fmt.Printf("  Vendor:   %q\n", vendor)
		fmt.Printf("  Product:  %q\n", product)
		fmt.Printf("  Revision: %q\n", revision)
	}

	// Example 3: Building a custom/vendor-specific CDB.
	// This shows how you would construct any arbitrary CDB.
	// Here we build a READ CAPACITY(10) manually (opcode 0x25).
	readCapCDB := []byte{0x25, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	result, err = sess.Execute(ctx, 0, readCapCDB,
		uiscsi.WithDataIn(8), // READ CAPACITY(10) returns 8 bytes
	)
	if err != nil {
		log.Fatalf("READ CAPACITY(10): %v", err)
	}
	fmt.Printf("READ CAPACITY(10): status=0x%02X, data=%x\n", result.Status, result.Data)
}
