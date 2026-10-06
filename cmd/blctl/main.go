package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"syscall"

	"example.com/go-block-lattice/internal/control"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := control.RunControl(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		_ = json.NewEncoder(os.Stderr).Encode(map[string]any{"error": map[string]string{"code": "control_error", "message": err.Error()}})
		os.Exit(1)
	}
}
