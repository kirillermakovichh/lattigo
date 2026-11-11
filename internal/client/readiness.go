package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Readiness describes the node's current admission and quorum observation.
type Readiness struct {
	Ready          bool                       `json:"ready"`
	Syncing        bool                       `json:"syncing"`
	QuorumObserved bool                       `json:"quorum_observed"`
	Consensus      map[string]json.RawMessage `json:"consensus"`
}

// Readiness returns unavailable admission as Ready=false, not a transport error.
func (c *Client) Readiness(ctx context.Context) (Readiness, error) {
	var out Readiness
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+"/health/ready", nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("User-Agent", c.agent)
	resp, err := c.http.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 503 {
		return out, &Error{Code: "http_error", Message: http.StatusText(resp.StatusCode), Status: resp.StatusCode}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil {
		return out, err
	}
	if len(raw) > 64<<10 {
		return out, errors.New("readiness response exceeds 64 KiB")
	}
	var wire struct {
		Ready          *bool                      `json:"ready"`
		Syncing        *bool                      `json:"syncing"`
		QuorumObserved *bool                      `json:"quorum_observed"`
		Consensus      map[string]json.RawMessage `json:"consensus"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		return out, err
	}
	if wire.Ready == nil || wire.Syncing == nil || wire.QuorumObserved == nil || wire.Consensus == nil {
		return out, errors.New("incomplete readiness response")
	}
	if *wire.Ready != (resp.StatusCode == 200) {
		return out, errors.New("readiness status conflicts with body")
	}
	return Readiness{Ready: *wire.Ready, Syncing: *wire.Syncing, QuorumObserved: *wire.QuorumObserved, Consensus: wire.Consensus}, nil
}
