package client

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"time"

	"example.com/go-block-lattice/internal/protocol"
)

type Confirmation struct {
	Hash      protocol.Hash   `json:"hash"`
	Confirmed bool            `json:"confirmed"`
	Block     *protocol.Block `json:"block,omitempty"`
}

// Confirmation observes finality through a trusted node; false does not imply admission.
func (c *Client) Confirmation(ctx context.Context, id protocol.Hash, wait time.Duration) (Confirmation, error) {
	var out Confirmation
	if _, err := id.Bytes(); err != nil {
		return out, err
	}
	if wait < 0 || wait > 15*time.Second {
		return out, errors.New("confirmation wait must be 0..15s")
	}
	milliseconds := (wait + time.Millisecond - 1) / time.Millisecond
	query := url.Values{"id": {string(id)}, "wait_ms": {strconv.FormatInt(int64(milliseconds), 10)}}
	if err := c.request(ctx, "GET", "/v1/confirmation?"+query.Encode(), nil, &out); err != nil {
		return out, err
	}
	if out.Hash != id || out.Confirmed != (out.Block != nil) {
		return out, errors.New("RPC returned an invalid confirmation result")
	}
	if out.Block != nil {
		if out.Block.ID() != id {
			return out, errors.New("RPC confirmed another block")
		}
		if err := out.Block.Verify(); err != nil {
			return out, err
		}
	}
	return out, nil
}

// WaitConfirmed uses long polling, falling back to the original lookup contract
// when an older node does not expose the additive confirmation route.
func (c *Client) WaitConfirmed(ctx context.Context, id protocol.Hash) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		result, err := c.Confirmation(ctx, id, 10*time.Second)
		if err != nil {
			var response *Error
			if errors.As(err, &response) && (response.Status == 404 || response.Status == 405) {
				return c.Wait(ctx, id)
			}
			return err
		}
		if result.Confirmed {
			return nil
		}
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
