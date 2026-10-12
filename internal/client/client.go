// Package client implements the bounded, versioned node HTTP contract.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"example.com/go-block-lattice/internal/protocol"
)

// MaxResponseBytes accommodates a full certificate-bearing history page while bounding memory.
const MaxResponseBytes = 32 << 20

type Client struct {
	base  string
	token string
	http  *http.Client
}
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Status  int    `json:"-"`
}

func (e *Error) Error() string  { return fmt.Sprintf("RPC %s: %s", e.Code, e.Message) }
func IsNotFound(err error) bool { var e *Error; return errors.As(err, &e) && e.Code == "not_found" }

// SubmissionError means a request failed without a definitive application response.
// Query Hash before retrying; never sign a competing replacement automatically.
type SubmissionError struct {
	Hash  protocol.Hash
	Cause error
}

func (e *SubmissionError) Error() string {
	return fmt.Sprintf("submission outcome unknown for %s: %v; query this hash before retrying the same signed block", e.Hash, e.Cause)
}
func (e *SubmissionError) Unwrap() error { return e.Cause }

type Status struct {
	Network           protocol.Hash   `json:"network"`
	Height            uint64          `json:"height"`
	Tip               protocol.Hash   `json:"tip"`
	Supply            protocol.Amount `json:"supply"`
	Accounts          int             `json:"accounts"`
	Receivables       int             `json:"receivables"`
	HistoryBytes      uint64          `json:"history_bytes,omitempty"`
	HistoryLimitBytes uint64          `json:"history_limit_bytes,omitempty"`
	CapacityReached   bool            `json:"capacity_reached,omitempty"`
}
type AccountState struct {
	Account        protocol.Account `json:"account"`
	Frontier       protocol.Hash    `json:"frontier"`
	Balance        protocol.Amount  `json:"balance"`
	Representative protocol.Account `json:"representative"`
	Height         uint64           `json:"height"`
}
type Receivable struct {
	Source protocol.Hash    `json:"source"`
	From   protocol.Account `json:"from"`
	To     protocol.Account `json:"to"`
	Amount protocol.Amount  `json:"amount"`
}
type Submission struct {
	Hash   protocol.Hash `json:"hash"`
	Status string        `json:"status"`
}

func New(endpoint, token string) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Trim(u.Path, "/") != "" {
		return nil, errors.New("RPC endpoint must be an http(s) origin without credentials, path, query or fragment")
	}
	return &Client{base: strings.TrimRight(endpoint, "/"), token: token, http: &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("RPC redirects are disabled") }}}, nil
}
func (c *Client) request(ctx context.Context, method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(data) > MaxResponseBytes {
		return fmt.Errorf("RPC response exceeds %d MiB limit", MaxResponseBytes>>20)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var v struct {
			Error Error `json:"error"`
		}
		if json.Unmarshal(data, &v) != nil || v.Error.Code == "" {
			return &Error{Code: "http_error", Message: http.StatusText(resp.StatusCode), Status: resp.StatusCode}
		}
		v.Error.Status = resp.StatusCode
		return &v.Error
	}
	if out == nil {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	if err = dec.Decode(out); err != nil {
		return fmt.Errorf("decode RPC response: %w", err)
	}
	if err = dec.Decode(new(any)); err != io.EOF {
		return errors.New("trailing RPC JSON")
	}
	return nil
}
func (c *Client) Status(ctx context.Context) (Status, error) {
	var out Status
	err := c.request(ctx, "GET", "/v1/status", nil, &out)
	return out, err
}
func (c *Client) Genesis(ctx context.Context) (protocol.Genesis, error) {
	var out protocol.Genesis
	err := c.request(ctx, "GET", "/v1/genesis", nil, &out)
	if err == nil {
		err = out.Validate()
	}
	return out, err
}
func (c *Client) Account(ctx context.Context, a protocol.Account) (AccountState, error) {
	var out AccountState
	err := c.request(ctx, "GET", "/v1/account?address="+url.QueryEscape(string(a)), nil, &out)
	if err == nil && out.Account != a {
		return out, errors.New("RPC returned another account")
	}
	return out, err
}
func (c *Client) Receivables(ctx context.Context, a protocol.Account, offset, limit int) ([]Receivable, error) {
	var out []Receivable
	err := c.request(ctx, "GET", fmt.Sprintf("/v1/receivables?address=%s&offset=%d&limit=%d", url.QueryEscape(string(a)), offset, limit), nil, &out)
	return out, err
}
func (c *Client) History(ctx context.Context, a protocol.Account, offset, limit int) ([]json.RawMessage, error) {
	var out []json.RawMessage
	err := c.request(ctx, "GET", fmt.Sprintf("/v1/history?address=%s&offset=%d&limit=%d", url.QueryEscape(string(a)), offset, limit), nil, &out)
	return out, err
}
func (c *Client) Block(ctx context.Context, id protocol.Hash) (protocol.Block, error) {
	var out protocol.Block
	err := c.request(ctx, "GET", "/v1/block?id="+url.QueryEscape(string(id)), nil, &out)
	if err == nil {
		if out.ID() != id {
			return out, errors.New("RPC returned a different block")
		}
		err = out.Verify()
	}
	return out, err
}
func (c *Client) Submit(ctx context.Context, b protocol.Block) (Submission, error) {
	var out Submission
	if err := b.Verify(); err != nil {
		return out, err
	}
	err := c.request(ctx, "POST", "/v1/blocks", b, &out)
	if err != nil {
		var rpcErr *Error
		if !errors.As(err, &rpcErr) || rpcErr.Status >= 500 {
			err = &SubmissionError{Hash: b.ID(), Cause: err}
		}
		return out, err
	}
	if out.Hash != b.ID() || (out.Status != "admitted" && out.Status != "confirmed") {
		return out, &SubmissionError{Hash: b.ID(), Cause: errors.New("RPC returned an invalid submission acknowledgement")}
	}
	return out, nil
}
func (c *Client) Wait(ctx context.Context, id protocol.Hash) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := c.Block(ctx, id); err == nil {
			return nil
		} else if !IsNotFound(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Backup streams an authenticated consistent backup, refusing oversized downloads.
func (c *Client) Backup(ctx context.Context, dst io.Writer, maxBytes int64) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+"/v1/admin/backup", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("backup HTTP status %d", resp.StatusCode)
	}
	if maxBytes <= 0 || maxBytes == int64(^uint64(0)>>1) {
		return errors.New("backup byte limit must be positive and below int64 maximum")
	}
	n, err := io.Copy(dst, io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return err
	}
	if n > maxBytes {
		return errors.New("backup exceeds configured byte limit")
	}
	return nil
}
