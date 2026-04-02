// Command discover-read demonstrates iSCSI target discovery followed by
// login, capacity query, and block read.
//
// Usage:
//
//	discover-read <target-address>
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/rkujawa/uiscsi"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <target-address>\n", os.Args[0])
		os.Exit(1)
	}
	addr := os.Args[1]
	ctx := context.Background()

	// Step 1: Discover available targets.
	targets, err := uiscsi.Discover(ctx, addr)
	if err != nil {
		log.Fatalf("discover: %v", err)
	}
	fmt.Printf("Found %d target(s):\n", len(targets))
	for _, t := range targets {
		fmt.Printf("  %s\n", t.Name)
		for _, p := range t.Portals {
			fmt.Printf("    portal: %s:%d (group %d)\n", p.Address, p.Port, p.GroupTag)
		}
	}
	if len(targets) == 0 {
		return
	}

	// Step 2: Connect to first target.
	sess, err := uiscsi.Dial(ctx, addr,
		uiscsi.WithTarget(targets[0].Name),
	)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer sess.Close()

	// Step 3: Query capacity.
	cap, err := sess.ReadCapacity(ctx, 0)
	if err != nil {
		log.Fatalf("read capacity: %v", err)
	}
	fmt.Printf("LUN 0: %d blocks of %d bytes (%.2f GB)\n",
		cap.LogicalBlocks, cap.BlockSize,
		float64(cap.LogicalBlocks)*float64(cap.BlockSize)/1e9)

	// Step 4: Read first block.
	data, err := sess.ReadBlocks(ctx, 0, 0, 1, cap.BlockSize)
	if err != nil {
		log.Fatalf("read: %v", err)
	}
	fmt.Printf("Read %d bytes from LBA 0\n", len(data))
	// Print first 32 bytes in hex.
	n := 32
	if len(data) < n {
		n = len(data)
	}
	fmt.Printf("Data: %x\n", data[:n])
}
