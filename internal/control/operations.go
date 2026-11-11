package control

import (
	"context"
	"errors"
	"example.com/go-block-lattice/internal/client"
	"flag"
	"io"
	"os"
	"time"

	"example.com/go-block-lattice/internal/node"
)

func runDoctor(ctx context.Context, args []string, out, errout io.Writer) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(errout)
	path := fs.String("config", "", "node configuration to inspect without opening runtime state")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 || *path == "" {
		return errors.New("doctor requires --config and no positional arguments")
	}
	cfg, err := node.LoadConfig(*path)
	if err != nil {
		return errors.New("configuration invalid or unreadable")
	}
	if err := cfg.ValidateFiles(); err != nil {
		return errors.New("startup input validation failed")
	}
	return controlOutput(out, map[string]any{"configuration_valid": true, "rpc_listener": cfg.RPCListen, "p2p_listener": cfg.P2PListen, "configured_peers": len(cfg.Peers), "max_peers": cfg.MaxPeers, "max_pending": cfg.MaxPending, "history_limit_bytes": cfg.HistoryLimitBytes, "rpc_authentication": cfg.RPCToken != "", "representative": cfg.RepresentativeKey != ""})
}

func runProfile(ctx context.Context, args []string, out, errout io.Writer) error {
	fs := flag.NewFlagSet("profile", flag.ContinueOnError)
	fs.SetOutput(errout)
	rpc := fs.String("rpc", "http://127.0.0.1:7070", "RPC origin")
	tokenFile := fs.String("token-file", "", "private token file; otherwise BL_RPC_TOKEN")
	output := fs.String("out", "", "exclusive private profile output")
	kind := fs.String("kind", "heap", "heap, goroutine or cpu")
	seconds := fs.Int("seconds", 1, "CPU collection seconds (1..5)")
	timeout := fs.Duration("timeout", 15*time.Second, "download deadline")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 || *output == "" || *timeout <= 0 {
		return errors.New("profile requires --out, positive timeout and no positional arguments")
	}
	token := os.Getenv("BL_RPC_TOKEN")
	if *tokenFile != "" {
		b, err := controlSecret(*tokenFile)
		if err != nil {
			return err
		}
		token = string(b)
		clear(b)
	}
	c, err := client.New(*rpc, token)
	if err != nil {
		return err
	}
	defer c.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	if err := writeNew(*output, func(w io.Writer) error { return c.Profile(ctx, *kind, *seconds, w) }); err != nil {
		return err
	}
	return controlOutput(out, map[string]any{"path": *output, "kind": *kind, "status": "complete"})
}

func runReady(ctx context.Context, args []string, out, errout io.Writer) error {
	fs := flag.NewFlagSet("ready", flag.ContinueOnError)
	fs.SetOutput(errout)
	rpc := fs.String("rpc", "http://127.0.0.1:7070", "RPC origin")
	timeout := fs.Duration("timeout", 5*time.Second, "probe deadline")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 || *timeout <= 0 {
		return errors.New("ready requires positive timeout and no positional arguments")
	}
	c, err := client.New(*rpc, "")
	if err != nil {
		return err
	}
	defer c.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	ready, err := c.Readiness(ctx)
	if err != nil {
		return err
	}
	if err := controlOutput(out, ready); err != nil {
		return err
	}
	if !ready.Ready {
		return errors.New("node is not ready")
	}
	return nil
}
