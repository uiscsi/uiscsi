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

	// Step 1: Connect to target.
	sess, err := uiscsi.Dial(ctx, addr,
		uiscsi.WithTarget(iqn),
	)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer sess.Close()

	// Step 2: Read capacity to get block size.
	cap, err := sess.ReadCapacity(ctx, 0)
	if err != nil {
		log.Fatalf("read capacity: %v", err)
	}
	fmt.Printf("Block size: %d bytes\n", cap.BlockSize)

	// Step 3: Write test pattern to LBA 0.
	pattern := make([]byte, cap.BlockSize)
	for i := range pattern {
		pattern[i] = 0xAA
	}
	if err := sess.WriteBlocks(ctx, 0, 0, 1, cap.BlockSize, pattern); err != nil {
		log.Fatalf("write: %v", err)
	}
	fmt.Printf("Wrote %d bytes of 0xAA to LBA 0\n", cap.BlockSize)

	// Step 4: Read back LBA 0.
	readback, err := sess.ReadBlocks(ctx, 0, 0, 1, cap.BlockSize)
	if err != nil {
		log.Fatalf("read: %v", err)
	}

	// Step 5: Compare written vs read data.
	if bytes.Equal(pattern, readback) {
		fmt.Println("Verification: PASSED -- written data matches readback")
	} else {
		fmt.Println("Verification: FAILED -- data mismatch")
		os.Exit(1)
	}
}
