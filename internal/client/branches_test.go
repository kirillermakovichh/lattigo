package client

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
)

func branchClient(t *testing.T, status int, body string) *Client {
	t.Helper()
	c, err := New("http://local.invalid", "private-test-token")
	if err != nil {
		t.Fatal(err)
	}
	c.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	return c
}
func clientBlock(t *testing.T) protocol.Block {
	t.Helper()
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	b := protocol.Block{Version: 1, Network: protocol.Digest([]byte("client")), Kind: "send", Account: a, Representative: a, Balance: 99, Link: string(a)}
	if err := b.Sign(key); err != nil {
		t.Fatal(err)
	}
	return b
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestClientResponseAndSubmissionBranches(t *testing.T) {
	ctx := context.Background()
	block := clientBlock(t)
	for _, url := range []string{"%", "ftp://node.invalid", "http://u:p@node.invalid", "http://node.invalid/path", "http://node.invalid?q=x", "http://node.invalid#x", "http:///"} {
		if _, err := New(url, ""); err == nil {
			t.Errorf("invalid origin accepted:%s", url)
		}
	}
	for name, body := range map[string]string{"malformed": "{", "trailing": "{}{}", "fallback": "not-json", "empty error": `{"error":{}}`} {
		t.Run(name, func(t *testing.T) {
			status := 200
			if name == "fallback" || name == "empty error" {
				status = 400
			}
			c := branchClient(t, status, body)
			_, err := c.Status(ctx)
			if err == nil {
				t.Fatal("invalid response accepted")
			}
			if status == 400 {
				var rpcErr *Error
				if !errors.As(err, &rpcErr) || rpcErr.Code != "http_error" || rpcErr.Error() == "" {
					t.Fatal("fallback error lost")
				}
			}
		})
	}
	c := branchClient(t, 200, "")
	c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) { return nil, io.ErrClosedPipe })
	if _, err := c.Status(ctx); err == nil {
		t.Fatal("transport error ignored")
	}
	c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(brokenReader{})}, nil
	})
	if _, err := c.Status(ctx); err == nil {
		t.Fatal("body failure ignored")
	}
	for name, response := range map[string]string{"rejection": `{"error":{"code":"invalid","message":"rejected"}}`, "uncertain": `{}`, "wrong status": `{"hash":"` + string(block.ID()) + `","status":"pending"}`, "confirmed": `{"hash":"` + string(block.ID()) + `","status":"confirmed"}`} {
		t.Run(name, func(t *testing.T) {
			status := 200
			if name == "rejection" {
				status = 400
			}
			c := branchClient(t, status, response)
			_, err := c.Submit(ctx, block)
			if name == "confirmed" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil {
				t.Fatal("invalid submission acknowledged")
			}
			if name != "rejection" {
				var unknown *SubmissionError
				if !errors.As(err, &unknown) || unknown.Unwrap() == nil || unknown.Error() == "" {
					t.Fatal("uncertain outcome not preserved")
				}
			}
		})
	}
	for _, status := range []int{500, 503} {
		c := branchClient(t, status, `{"error":{"code":"unavailable"}}`)
		_, err := c.Submit(ctx, block)
		var unknown *SubmissionError
		if !errors.As(err, &unknown) {
			t.Fatal("server failure represented as definitive rejection")
		}
	}
	invalid := block
	invalid.Signature = "bad"
	if _, err := c.Submit(ctx, invalid); err == nil {
		t.Fatal("invalid signature submitted")
	}
	c = branchClient(t, 200, `{}`)
	if _, err := c.Genesis(ctx); err == nil {
		t.Fatal("invalid genesis accepted")
	}
	if _, err := c.Account(ctx, block.Account); err == nil {
		t.Fatal("wrong account accepted")
	}
	if _, err := c.Block(ctx, block.ID()); err == nil {
		t.Fatal("wrong block accepted")
	}
	raw, err := json.Marshal(block)
	if err != nil {
		t.Fatal(err)
	}
	c = branchClient(t, 200, string(raw))
	if _, err = c.Block(ctx, block.ID()); err != nil {
		t.Fatal(err)
	}
	c = branchClient(t, 200, `[]`)
	if _, err = c.History(ctx, block.Account, 0, 1); err != nil {
		t.Fatal(err)
	}
	if _, err = c.Receivables(ctx, block.Account, 0, 1); err != nil {
		t.Fatal(err)
	}
}
func TestBackupAndConfirmationFailureBranches(t *testing.T) {
	ctx := context.Background()
	b := clientBlock(t)
	c := branchClient(t, 400, `{"error":{"code":"invalid"}}`)
	if c.Wait(ctx, b.ID()) == nil {
		t.Fatal("wait ignored error")
	}
	if c.Backup(ctx, io.Discard, 10) == nil {
		t.Fatal("backup ignored status")
	}
	c = branchClient(t, 200, "archive")
	if c.Backup(ctx, io.Discard, 3) == nil {
		t.Fatal("oversized backup accepted")
	}
	if c.Backup(ctx, brokenWriter{}, 10) == nil {
		t.Fatal("write failure ignored")
	}
	if err := c.Backup(ctx, io.Discard, 10); err != nil {
		t.Fatal(err)
	}
	c = branchClient(t, 404, `{"error":{"code":"not_found"}}`)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if !errors.Is(c.Wait(canceled, b.ID()), context.Canceled) {
		t.Fatal("confirmation wait ignored cancellation")
	}
}
