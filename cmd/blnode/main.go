// blnode runs a block-lattice node using an explicit network genesis and configuration.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"example.com/go-block-lattice/internal/node"
)

var version = "development"

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("node failed", "error", err)
		os.Exit(1)
	}
}
func run(args []string) error {
	flags := flag.NewFlagSet("blnode", flag.ContinueOnError)
	config := flags.String("config", "", "path to node JSON configuration")
	showVersion := flags.Bool("version", false, "print binary version")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *showVersion {
		fmt.Println(version)
		return nil
	}
	if *config == "" {
		return fmt.Errorf("--config is required")
	}
	cfg, err := node.LoadConfig(*config)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return node.Run(ctx, cfg)
}
