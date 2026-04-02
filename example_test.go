package uiscsi_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/rkujawa/uiscsi"
)

func ExampleDial() {
	ctx := context.Background()
	sess, err := uiscsi.Dial(ctx, "192.168.1.100:3260",
		uiscsi.WithTarget("iqn.2026-03.com.example:storage"),
	)
	if err != nil {
		fmt.Println("dial:", err)
		return
	}
	defer sess.Close()
	fmt.Println("connected")
}

func ExampleDiscover() {
	ctx := context.Background()
	targets, err := uiscsi.Discover(ctx, "192.168.1.100:3260")
	if err != nil {
		fmt.Println("discover:", err)
		return
	}
	for _, t := range targets {
		fmt.Printf("target: %s (%d portals)\n", t.Name, len(t.Portals))
	}
}

func ExampleSession_ReadBlocks() {
	ctx := context.Background()
	sess, err := uiscsi.Dial(ctx, "192.168.1.100:3260",
		uiscsi.WithTarget("iqn.2026-03.com.example:storage"),
	)
	if err != nil {
		return
	}
	defer sess.Close()

	data, err := sess.ReadBlocks(ctx, 0, 0, 1, 512)
	if err != nil {
		fmt.Println("read:", err)
		return
	}
	fmt.Printf("read %d bytes\n", len(data))
}

func ExampleSession_WriteBlocks() {
	// Show write + readback verification pattern.
	ctx := context.Background()
	sess, err := uiscsi.Dial(ctx, "192.168.1.100:3260",
		uiscsi.WithTarget("iqn.2026-03.com.example:storage"),
	)
	if err != nil {
		return
	}
	defer sess.Close()

	data := make([]byte, 512)
	copy(data, []byte("hello iSCSI"))
	if err := sess.WriteBlocks(ctx, 0, 0, 1, 512, data); err != nil {
		fmt.Println("write:", err)
	}
}

func ExampleSession_Execute() {
	// Raw CDB pass-through: TEST UNIT READY.
	ctx := context.Background()
	sess, err := uiscsi.Dial(ctx, "192.168.1.100:3260",
		uiscsi.WithTarget("iqn.2026-03.com.example:storage"),
	)
	if err != nil {
		return
	}
	defer sess.Close()

	turCDB := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00} // TEST UNIT READY
	result, err := sess.Execute(ctx, 0, turCDB)
	if err != nil {
		fmt.Println("execute:", err)
		return
	}
	fmt.Printf("status: 0x%02X\n", result.Status)
}

func ExampleWithLogger() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx := context.Background()
	_, _ = uiscsi.Dial(ctx, "192.168.1.100:3260",
		uiscsi.WithTarget("iqn.2026-03.com.example:storage"),
		uiscsi.WithLogger(logger),
	)
}

func ExampleWithCHAP() {
	ctx := context.Background()
	_, _ = uiscsi.Dial(ctx, "192.168.1.100:3260",
		uiscsi.WithTarget("iqn.2026-03.com.example:storage"),
		uiscsi.WithCHAP("initiator-user", "s3cret"),
	)
}
