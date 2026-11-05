package wallet

import (
	"context"
	"errors"
	"flag"
	"io"

	"example.com/go-block-lattice/internal/client"
	"example.com/go-block-lattice/internal/protocol"
)

var watchComplete = errors.New("requested events delivered")

func watchCommand(ctx context.Context, args []string, out, errout io.Writer) error {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	fs.SetOutput(errout)
	var f commandFlags
	f.add(fs)
	var address, after string
	var count int
	var incoming bool
	fs.StringVar(&address, "address", "", "optional account filter; explicit addresses need no wallet")
	fs.StringVar(&after, "after", "", "last successfully consumed event ID")
	fs.BoolVar(&incoming, "incoming", false, "only sends addressed to the account")
	fs.IntVar(&count, "count", 0, "stop after this many matching events; zero watches until timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || count < 0 || count > 1000000 || f.timeout <= 0 {
		return errors.New("watch requires no positional arguments, count0..1000000 and positive timeout")
	}
	if address != "" {
		if _, err := protocol.Account(address).PublicKey(); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	c, err := f.client()
	if err != nil {
		return err
	}
	defer c.CloseIdleConnections()
	status, err := c.Status(ctx)
	if err != nil {
		return err
	}
	if f.expectedNetwork != "" && protocol.Hash(f.expectedNetwork) != status.Network {
		return errors.New("RPC network differs from expected network")
	}
	delivered := 0
	_, err = c.Watch(ctx, client.EventOptions{Network: status.Network, After: after, Address: protocol.Account(address), Incoming: incoming}, func(e client.Event) error {
		if err := output(out, e); err != nil {
			return err
		}
		delivered++
		if count > 0 && delivered >= count {
			return watchComplete
		}
		return nil
	})
	if errors.Is(err, watchComplete) {
		return nil
	}
	return err
}
