package wallet

import (
	"context"
	"errors"
	"fmt"

	"example.com/go-block-lattice/internal/client"
	"example.com/go-block-lattice/internal/protocol"
)

type ReceiveBatch struct {
	Completed    []client.Submission `json:"completed"`
	Pending      *client.Submission  `json:"pending,omitempty"`
	LimitReached bool                `json:"limit_reached"`
	Error        string              `json:"error,omitempty"`
}

func receiveBatch(ctx context.Context, f commandFlags, limit int, representative string) (ReceiveBatch, error) {
	result := ReceiveBatch{Completed: []client.Submission{}}
	if limit < 1 || limit > 100 {
		return result, errors.New("receive-all limit must be 1..100")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	w, err := f.open()
	if err != nil {
		return result, err
	}
	account, err := keyAccount(w, f.index)
	w.Close()
	if err != nil {
		return result, err
	}
	c, err := f.client()
	if err != nil {
		return result, err
	}
	if representative != "" {
		if _, err := c.Account(ctx, account); err == nil {
			return result, errors.New("--representative is only accepted when opening an account")
		} else if !client.IsNotFound(err) {
			return result, err
		}
	}
	rows, err := c.Receivables(ctx, account, 0, limit)
	if err != nil {
		return result, err
	}
	if len(rows) > limit {
		return result, errors.New("RPC returned more receivables than requested")
	}
	seen := map[protocol.Hash]bool{}
	for _, row := range rows {
		if _, err := row.Source.Bytes(); err != nil {
			return result, err
		}
		if row.To != account || row.Amount == 0 || seen[row.Source] {
			return result, errors.New("RPC returned invalid or repeated receivables")
		}
		seen[row.Source] = true
	}
	for i, row := range rows {
		rep := ""
		if i == 0 {
			rep = representative
		}
		submitted, err := createPayment(ctx, f, c, transaction{Command: "receive", Source: string(row.Source), Representative: rep, Wait: true})
		if err != nil {
			if submitted.Hash != "" {
				result.Pending = &submitted
			} else {
				var unknown *client.SubmissionError
				if errors.As(err, &unknown) {
					result.Pending = &client.Submission{Hash: unknown.Hash, Status: "unknown"}
				}
			}
			return result, fmt.Errorf("receive-all stopped after %d confirmations: %w", len(result.Completed), err)
		}
		result.Completed = append(result.Completed, submitted)
	}
	result.LimitReached = len(rows) == limit
	return result, nil
}
