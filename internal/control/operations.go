package control

import (
	"context"
	"errors"
	"flag"
	"io"

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
