// Command write-verify demonstrates writing blocks to an iSCSI target
// and reading them back to verify correctness.
//
// Usage:
//
//	write-verify <target-address> <target-iqn>
package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"

	"github.com/uiscsi/uiscsi"
)

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: %s <target-address> <target-iqn>\n", os.Args[0])
		return 1
	}
	addr := os.Args[1]
	iqn := os.Args[2]
	ctx := context.Background()

	// Step 1: Connect to target.
	sess, err := uiscsi.Dial(ctx, addr,
		uiscsi.WithTarget(iqn),
	)
	if err != nil {
		log.Printf("dial: %v", err)
		return 1
	}
	defer func() { _ = sess.Close() }()

	// Step 2: Read capacity to get block size.
	cap, capErr := sess.SCSI().ReadCapacity(ctx, 0)
	if capErr != nil {
		log.Printf("read capacity: %v", capErr)
		return 1
	}
	fmt.Printf("Block size: %d bytes\n", cap.BlockSize)

	// Step 3: Write test pattern to LBA 0.
	pattern := make([]byte, cap.BlockSize)
	for i := range pattern {
		pattern[i] = 0xAA
	}
	if writeErr := sess.SCSI().WriteBlocks(ctx, 0, 0, 1, cap.BlockSize, pattern); writeErr != nil {
		log.Printf("write: %v", writeErr)
		return 1
	}
	fmt.Printf("Wrote %d bytes of 0xAA to LBA 0\n", cap.BlockSize)

	// Step 4: Read back LBA 0.
	readback, readErr := sess.SCSI().ReadBlocks(ctx, 0, 0, 1, cap.BlockSize)
	if readErr != nil {
		log.Printf("read: %v", readErr)
		return 1
	}

	// Step 5: Compare written vs read data.
	if bytes.Equal(pattern, readback) {
		fmt.Println("Verification: PASSED -- written data matches readback")
		return 0
	}
	fmt.Println("Verification: FAILED -- data mismatch")
	return 1
}
