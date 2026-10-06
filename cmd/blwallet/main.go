package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"

	"example.com/go-block-lattice/internal/wallet"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := wallet.RunCommand(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_ = json.NewEncoder(os.Stderr).Encode(map[string]any{"error": map[string]string{"code": "wallet_error", "message": err.Error()}})
		os.Exit(1)
	}
}
