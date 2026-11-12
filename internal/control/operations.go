package control

import (
	"context"
	"errors"
	"flag"
	"io"
	"net"
	"os"
	"time"

	"example.com/go-block-lattice/internal/client"
	"example.com/go-block-lattice/internal/node"
)

func runDoctor(ctx context.Context, args []string, out, errout io.Writer) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(errout)
	path := fs.String("config", "", "node configuration to inspect without opening runtime state")
	live := fs.Bool("live", false, "explicitly probe configured listeners and runtime capacity")
	timeout := fs.Duration("timeout", 5*time.Second, "live probe deadline")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 0 || *path == "" || *timeout <= 0 {
		return errors.New("doctor requires --config and no positional arguments")
	}
	cfg, err := node.LoadConfig(*path)
	if err != nil {
		return errors.New("configuration invalid or unreadable")
	}
	if err := cfg.ValidateFiles(); err != nil {
		return errors.New("startup input validation failed")
	}
	report := map[string]any{"configuration_valid": true, "rpc_listener": cfg.RPCListen, "p2p_listener": cfg.P2PListen, "configured_peers": len(cfg.Peers), "max_peers": cfg.MaxPeers, "max_pending": cfg.MaxPending, "history_limit_bytes": cfg.HistoryLimitBytes, "rpc_authentication": cfg.RPCToken != "", "representative": cfg.RepresentativeKey != ""}
	if *live {
		ctx, cancel := context.WithTimeout(ctx, *timeout)
		defer cancel()
		probe, err := doctorLive(ctx, cfg)
		report["live"] = probe
		if outputErr := controlOutput(out, report); outputErr != nil {
			return outputErr
		}
		return err
	}
	return controlOutput(out, report)
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

func localProbeAddress(address string) string {
	host, port, _ := net.SplitHostPort(address)
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	if host == "::" {
		host = "::1"
	}
	return net.JoinHostPort(host, port)
}

func doctorLive(ctx context.Context, cfg node.Config) (map[string]any, error) {
	report := map[string]any{"rpc_reachable": false, "p2p_listener_reachable": false}
	c, err := client.New("http://"+localProbeAddress(cfg.RPCListen), "")
	if err != nil {
		return report, errors.New("live RPC configuration invalid")
	}
	defer c.CloseIdleConnections()
	status, err := c.Status(ctx)
	if err != nil {
		return report, errors.New("live RPC status probe failed")
	}
	report["rpc_reachable"] = true
	report["height"] = status.Height
	report["history_bytes"] = status.HistoryBytes
	report["history_limit_bytes"] = status.HistoryLimitBytes
	report["capacity_reached"] = status.CapacityReached
	ready, err := c.Readiness(ctx)
	if err != nil {
		return report, errors.New("live readiness probe failed")
	}
	report["ready"] = ready.Ready
	report["syncing"] = ready.Syncing
	report["quorum_observed"] = ready.QuorumObserved
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", localProbeAddress(cfg.P2PListen))
	if err != nil {
		return report, errors.New("live P2P listener probe failed")
	}
	conn.Close()
	report["p2p_listener_reachable"] = true
	if !ready.Ready {
		return report, errors.New("node is not ready")
	}
	return report, nil
}
