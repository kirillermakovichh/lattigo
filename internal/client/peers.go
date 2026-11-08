package client

import (
	"context"
	"errors"
)

type PeerDiagnostic struct {
	LatencyNanos        int64  `json:"latency_nanos"`
	LastSuccessUnixNano int64  `json:"last_success_unix_nano"`
	URL                 string `json:"url"`
	Connected           bool   `json:"connected"`
	Height              uint64 `json:"height"`
	QueuedBytes         int64  `json:"queued_bytes"`
	QueuedMessages      int    `json:"queued_messages"`
	ConsecutiveFailures uint64 `json:"consecutive_failures"`
	RetryAtUnixNano     int64  `json:"retry_at_unix_nano"`
}

func (c *Client) Peers(ctx context.Context) ([]PeerDiagnostic, error) {
	var peers []PeerDiagnostic
	if err := c.request(ctx, "GET", "/v1/admin/peers", nil, &peers); err != nil {
		return nil, err
	}
	if len(peers) > 128 {
		return nil, errors.New("peer diagnostics exceed configured peer budget")
	}
	return peers, nil
}
