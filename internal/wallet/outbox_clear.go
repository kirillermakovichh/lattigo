package wallet

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"example.com/go-block-lattice/internal/client"
	"example.com/go-block-lattice/internal/storage"
)

func clearOutbox(ctx context.Context, f commandFlags, out io.Writer) error {
	return usePending(ctx, f, func(c *client.Client, entry OutboxEntry, file string) error {
		if _, err := c.Block(ctx, entry.Hash); err != nil {
			return err
		}
		if err := os.Remove(file); err != nil {
			return err
		}
		if err := storage.SyncDirectory(filepath.Dir(file)); err != nil {
			return err
		}
		return output(out, map[string]any{"hash": entry.Hash, "state": "cleared"})
	})
}
