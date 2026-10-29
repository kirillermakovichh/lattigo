package wallet

import (
	"context"
	"errors"
	"io"

	"example.com/go-block-lattice/internal/client"
)

// usePending holds account exclusion through the entire remote operation and
// validates ownership and network before the caller may submit or remove intent.
func usePending(ctx context.Context, f commandFlags, operation func(*client.Client, OutboxEntry, string) error) error {
	w, err := f.open()
	if err != nil {
		return err
	}
	account, err := keyAccount(w, f.index)
	w.Close()
	if err != nil {
		return err
	}
	base, err := canonicalWallet(f.path)
	if err != nil {
		return err
	}
	lock, err := lockAccount(base, f.index)
	if err != nil {
		return err
	}
	defer lock.Close()
	entry, err := InspectOutbox(base, f.index)
	if err != nil {
		return err
	}
	if entry.Block.Account != account {
		return errors.New("outbox block does not belong to the selected wallet account")
	}
	if f.expectedNetwork != "" && string(entry.Block.Network) != f.expectedNetwork {
		return errors.New("outbox network differs from --expected-network")
	}
	c, err := f.client()
	if err != nil {
		return err
	}
	genesis, err := c.Genesis(ctx)
	if err != nil {
		return err
	}
	status, err := c.Status(ctx)
	if err != nil {
		return err
	}
	if entry.Block.Network != genesis.ID() || status.Network != genesis.ID() {
		return errors.New("outbox and RPC network do not match")
	}
	file, err := outboxFile(base, f.index)
	if err != nil {
		return err
	}
	return operation(c, entry, file)
}
func retryOutbox(ctx context.Context, f commandFlags, wait bool, out io.Writer) error {
	return usePending(ctx, f, func(c *client.Client, entry OutboxEntry, _ string) error {
		return submit(ctx, c, entry.Block, wait, out)
	})
}
