package wallet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"

	"example.com/go-block-lattice/internal/client"
	"example.com/go-block-lattice/internal/protocol"
)

type transaction struct {
	Command, To, Amount, Source, Representative string
	Wait                                        bool
}

func createPayment(ctx context.Context, f commandFlags, c *client.Client, request transaction) (client.Submission, error) {
	command, to, amount, source, representative, wait := request.Command, request.To, request.Amount, request.Source, request.Representative, request.Wait
	w, e := f.open()
	if e != nil {
		return client.Submission{}, e
	}
	defer w.Close()
	a, e := keyAccount(w, f.index)
	if e != nil {
		return client.Submission{}, e
	}
	// Resolve aliases so commands for the same wallet share one outbox and lock.
	f.path, e = filepath.EvalSymlinks(f.path)
	if e != nil {
		return client.Submission{}, e
	}
	accountLock, e := lockAccount(f.path, f.index)
	if e != nil {
		return client.Submission{}, e
	}
	defer accountLock.Close() // Closing the descriptor releases Flock, including on process exit.
	// A durable outbox prevents an uncertain prior submission from being silently replaced.
	pending := fmt.Sprintf("%s.pending-%d.json", f.path, f.index)
	if existing, e := loadBlock(pending); e == nil {
		if _, e = c.Block(ctx, existing.ID()); e != nil {
			return client.Submission{}, fmt.Errorf("unresolved signed block %s in %s: inspect or resubmit that file before creating another block", existing.ID(), pending)
		}
		if e = os.Remove(pending); e != nil {
			return client.Submission{}, e
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return client.Submission{}, e
	}
	g, e := c.Genesis(ctx)
	if e != nil {
		return client.Submission{}, e
	}
	status, e := c.Status(ctx)
	if e != nil {
		return client.Submission{}, e
	}
	if f.expectedNetwork != "" && g.ID() != protocol.Hash(f.expectedNetwork) {
		return client.Submission{}, errors.New("RPC network differs from pinned --expected-network")
	}
	if status.Network != g.ID() {
		return client.Submission{}, errors.New("RPC genesis does not match status network")
	}
	state, e := c.Account(ctx, a)
	if e != nil && !(command == "receive" && client.IsNotFound(e)) {
		return client.Submission{}, e
	}
	if e == nil {
		if e = verifyAccountState(ctx, c, g, state); e != nil {
			return client.Submission{}, e
		}
	}
	b := protocol.Block{Version: 1, Network: g.ID(), Kind: command, Account: a, Previous: state.Frontier, Representative: state.Representative, Balance: state.Balance}
	switch command {
	case "send":
		value, e := protocol.ParseAmount(amount)
		if e != nil {
			return client.Submission{}, e
		}
		if value == 0 || value > state.Balance {
			return client.Submission{}, errors.New("send amount must be positive and no greater than confirmed balance")
		}
		if _, e = protocol.Account(to).PublicKey(); e != nil {
			return client.Submission{}, e
		}
		b.Balance -= value
		b.Link = to
	case "delegate":
		b.Kind = "change"
		b.Representative = protocol.Account(representative)
	case "receive":
		if source == "" {
			return client.Submission{}, errors.New("--source is required")
		}
		// Validate the actual source block; this also supports sources beyond one RPC page.
		sending, e := c.Block(ctx, protocol.Hash(source))
		if e != nil {
			return client.Submission{}, e
		}
		if sending.Kind != "send" || sending.Link != string(a) || sending.Network != g.ID() {
			return client.Submission{}, errors.New("source is not a confirmed send to this account on this network")
		}
		var amount protocol.Amount
		for offset := 0; ; offset += 1000 {
			rows, e := c.Receivables(ctx, a, offset, 1000)
			if e != nil {
				return client.Submission{}, e
			}
			for _, r := range rows {
				if r.Source == protocol.Hash(source) {
					amount = r.Amount
				}
			}
			if amount != 0 || len(rows) < 1000 {
				break
			}
			if offset >= 1_000_000 {
				return client.Submission{}, errors.New("receivable search limit exceeded; use offline signing")
			}
		}
		if amount == 0 {
			return client.Submission{}, errors.New("source is not receivable; it may already be consumed")
		}
		if uint64(amount) > math.MaxUint64-uint64(state.Balance) {
			return client.Submission{}, errors.New("receive amount overflows balance")
		}
		b.Balance += amount
		b.Link = source
		if b.Representative == "" {
			if representative == "" {
				b.Representative = a
			} else {
				b.Representative = protocol.Account(representative)
			}
		} else if representative != "" {
			return client.Submission{}, errors.New("--representative is only accepted when opening an account; use delegate otherwise")
		}
	default:
		return client.Submission{}, errors.New("unsupported transaction command")
	}
	if e = signWork(ctx, w, f.index, &b, g.WorkBits); e != nil {
		return client.Submission{}, e
	}
	encoded, e := json.MarshalIndent(b, "", "  ")
	if e != nil {
		return client.Submission{}, e
	}
	if e = WritePrivate(pending, append(encoded, '\n')); e != nil {
		return client.Submission{}, fmt.Errorf("reserve durable signed block: %w", e)
	}
	if f.out != "" {
		if e = WritePrivate(f.out, append(encoded, '\n')); e != nil {
			return client.Submission{}, fmt.Errorf("block remains in %s: %w", pending, e)
		}
	}
	return submitResult(ctx, c, b, wait)
}
