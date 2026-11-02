package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"

	"example.com/go-block-lattice/internal/protocol"
)

type Anchor struct {
	Height uint64        `json:"height"`
	Tip    protocol.Hash `json:"tip"`
}
type Page[T any] struct {
	Items  []T    `json:"items"`
	Anchor Anchor `json:"anchor"`
	Next   string `json:"next,omitempty"`
}

func fetchPage[T any](ctx context.Context, c *Client, route string, account protocol.Account, cursor string, limit int) (Page[T], error) {
	var out Page[T]
	if _, err := account.PublicKey(); err != nil {
		return out, err
	}
	if limit < 0 || limit > 1000 || len(cursor) > 1024 {
		return out, errors.New("invalid page limit or cursor budget")
	}
	query := url.Values{"address": {string(account)}}
	bound := limit
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	} else if cursor == "" {
		bound = 100
	} else {
		bound = 1000
	}
	if err := c.request(ctx, "GET", route+"?"+query.Encode(), nil, &out); err != nil {
		return out, err
	}
	if _, err := out.Anchor.Tip.Bytes(); err != nil {
		return out, errors.New("RPC returned invalid page anchor")
	}
	if out.Items == nil || len(out.Items) > bound || len(out.Next) > 1024 || (out.Next != "" && len(out.Items) == 0) {
		return out, errors.New("RPC returned an invalid page envelope")
	}
	return out, nil
}
func (c *Client) HistoryPage(ctx context.Context, account protocol.Account, cursor string, limit int) (Page[json.RawMessage], error) {
	return fetchPage[json.RawMessage](ctx, c, "/v1/history/page", account, cursor, limit)
}
func (c *Client) ReceivablePage(ctx context.Context, account protocol.Account, cursor string, limit int) (Page[Receivable], error) {
	out, err := fetchPage[Receivable](ctx, c, "/v1/receivables/page", account, cursor, limit)
	if err != nil {
		return out, err
	}
	seen := map[protocol.Hash]bool{}
	for _, row := range out.Items {
		if row.To != account || row.Amount == 0 || seen[row.Source] {
			return out, errors.New("RPC returned invalid or repeated receivables")
		}
		if _, err := row.Source.Bytes(); err != nil {
			return out, err
		}
		if _, err := row.From.PublicKey(); err != nil {
			return out, err
		}
		seen[row.Source] = true
	}
	return out, nil
}
