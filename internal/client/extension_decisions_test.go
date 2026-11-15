package client

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"example.com/go-block-lattice/internal/protocol"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExtensionBatchDecisions(t *testing.T) {
	var rows readResults
	for _, raw := range []string{"", `{}`, `[1]`, `[`, `[] {}`, `[{"data":"` + strings.Repeat("a", 1<<20) + `"}]`, `[` + strings.Repeat(`{},`, 32) + `{}]`} {
		if err := rows.UnmarshalJSON([]byte(raw)); err == nil {
			t.Fatalf("accepted malformed batch %q", raw[:min(len(raw), 20)])
		}
	}
	for _, id := range []string{"", strings.Repeat("a", 65), "\xff", "a\nb"} {
		if validReadID(id) {
			t.Fatal("invalid correlation ID")
		}
	}
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	network := protocol.Digest([]byte("n"))
	b := protocol.Block{Version: 1, Network: network, Kind: "send", Account: a, Representative: a, Balance: 1, Link: string(a)}
	b.Sign(key)
	block, _ := json.Marshal(b)
	cases := []struct {
		q     ReadQuery
		raw   string
		valid bool
	}{
		{ReadQuery{Operation: ReadAccount, Address: a}, `{"account":"` + string(a) + `"}`, true},
		{ReadQuery{Operation: ReadAccount, Address: a}, `{}`, false}, {ReadQuery{Operation: ReadAccount}, `1`, false},
		{ReadQuery{Operation: ReadBlock, Hash: b.ID()}, string(block), true}, {ReadQuery{Operation: ReadBlock, Hash: network}, string(block), false}, {ReadQuery{Operation: ReadBlock}, `1`, false},
		{ReadQuery{Operation: ReadStatus}, `1`, false}, {ReadQuery{Operation: ReadStatus}, `{}`, false},
		{ReadQuery{Operation: ReadStatus}, `{"network":"` + string(network) + `"}`, false},
		{ReadQuery{Operation: ReadStatus}, `{"network":"` + string(network) + `","tip":"` + string(network) + `"}`, true},
		{ReadQuery{Operation: "unknown"}, `{}`, false},
	}
	for _, c := range cases {
		if err := validateReadData(c.q, []byte(c.raw)); (err == nil) != c.valid {
			t.Fatal(c, err)
		}
	}
	response := `[]`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, response) }))
	defer server.Close()
	c, _ := New(server.URL, "")
	queries := []ReadQuery{{ID: "one", Operation: ReadStatus}}
	for _, raw := range []string{`[]`, `[{"id":"one"}]`, `[{"id":"one","data":{},"error":{"code":"x"}}]`, `[{"id":"one","error":{}}]`, `[{"id":"one","data":1}]`, `not-json`} {
		response = raw
		if _, err := c.Batch(context.Background(), queries); err == nil {
			t.Fatal("invalid batch accepted", raw)
		}
	}
}

func TestExtensionConfirmationDecisions(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	network := protocol.Digest([]byte("n"))
	b := protocol.Block{Version: 1, Network: network, Kind: "send", Account: a, Representative: a, Balance: 1, Link: string(a)}
	b.Sign(key)
	response := Confirmation{Hash: b.ID()}
	code := 200
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(code); json.NewEncoder(w).Encode(response) }))
	defer server.Close()
	c, _ := New(server.URL, "")
	if _, err := c.Confirmation(context.Background(), "", 0); err == nil {
		t.Fatal("invalid hash")
	}
	for _, wait := range []time.Duration{-1, 16 * time.Second} {
		if _, err := c.Confirmation(context.Background(), b.ID(), wait); err == nil {
			t.Fatal("invalid wait")
		}
	}
	if _, err := c.Confirmation(context.Background(), b.ID(), 0); err != nil {
		t.Fatal(err)
	}
	response.Confirmed = true
	response.Block = &b
	other := protocol.Digest([]byte("other"))
	response.Hash = other
	if _, err := c.Confirmation(context.Background(), other, 0); err == nil {
		t.Fatal("wrong block")
	}
	response.Hash = b.ID()
	b.Signature = "invalid"
	if _, err := c.Confirmation(context.Background(), b.ID(), 0); err == nil {
		t.Fatal("invalid signature")
	}
	code = 500
	if err := c.WaitConfirmed(context.Background(), b.ID()); err == nil {
		t.Fatal("server failure")
	}
	code = 200
	response = Confirmation{Hash: b.ID()}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := c.WaitConfirmed(ctx, b.ID()); err == nil {
		t.Fatal("unconfirmed deadline")
	}
}

func TestExtensionOperatorResponseBudgets(t *testing.T) {
	ctx := context.Background()
	for _, body := range []string{"", strings.Repeat("x", (16<<20)+1)} {
		c := branchClient(t, 200, body)
		if err := c.Profile(ctx, "cpu", 1, io.Discard); err == nil {
			t.Fatal("profile budget", len(body))
		}
	}
	c := branchClient(t, 500, "")
	if err := c.Profile(ctx, "heap", 1, io.Discard); err == nil {
		t.Fatal("profile HTTP error")
	}
	c = branchClient(t, 200, "profile")
	if err := c.Profile(ctx, "heap", 1, brokenWriter{}); err == nil {
		t.Fatal("profile output error")
	}
	if err := c.Profile(nil, "heap", 1, io.Discard); err == nil {
		t.Fatal("nil request context")
	}
	for _, body := range []string{"invalid", strings.Repeat("x", (64<<10)+1)} {
		c = branchClient(t, 200, body)
		if _, err := c.Readiness(ctx); err == nil {
			t.Fatal("readiness invalid response")
		}
	}
	c = branchClient(t, 400, "")
	if _, err := c.Readiness(ctx); err == nil {
		t.Fatal("readiness HTTP error")
	}
	if _, err := c.Readiness(nil); err == nil {
		t.Fatal("readiness nil context")
	}
	c = branchClient(t, 200, "[]")
	if _, err := c.Peers(nil); err == nil {
		t.Fatal("peer request error")
	}
	c = branchClient(t, 200, "["+strings.Repeat("{},", 128)+"{}]")
	if _, err := c.Peers(ctx); err == nil {
		t.Fatal("peer budget")
	}
	for _, method := range []string{"profile", "readiness"} {
		c = branchClient(t, 200, "")
		c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) { return nil, io.ErrClosedPipe })
		if method == "profile" {
			if err := c.Profile(ctx, "heap", 1, io.Discard); err == nil {
				t.Fatal("transport error")
			}
		} else {
			if _, err := c.Readiness(ctx); err == nil {
				t.Fatal("transport error")
			}
		}
		c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(brokenReader{})}, nil
		})
		if method == "readiness" {
			if _, err := c.Readiness(ctx); err == nil {
				t.Fatal("body read error")
			}
		}
	}
}

func TestExtensionReceivableResponseValidation(t *testing.T) {
	b := clientBlock(t)
	a := b.Account
	hash := b.ID()
	for _, row := range []Receivable{{To: a, From: a, Source: hash, Amount: 1}, {To: a, From: a, Source: "bad", Amount: 1}, {To: a, From: "bad", Source: hash, Amount: 1}, {To: a, From: a, Source: hash, Amount: 0}} {
		raw, _ := json.Marshal(Page[Receivable]{Items: []Receivable{row}, Anchor: Anchor{Tip: hash}})
		c := branchClient(t, 200, string(raw))
		_, err := c.ReceivablePage(context.Background(), a, "", 0)
		valid := row.Amount > 0 && row.Source == hash && row.From == a
		if (err == nil) != valid {
			t.Fatal(row, err)
		}
	}
	raw, _ := json.Marshal(Page[Receivable]{Items: []Receivable{}, Anchor: Anchor{Tip: hash}})
	c := branchClient(t, 200, string(raw))
	if _, err := c.ReceivablePage(context.Background(), a, "", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ReceivablePage(nil, a, "", 0); err == nil {
		t.Fatal("nil context")
	}
}

func TestExtensionEventRequestAndFrameRejections(t *testing.T) {
	options, frame := eventFixture(t, 1)
	consume := func(Event) error { return nil }
	c := branchClient(t, 200, "")
	if _, err := c.Events(context.Background(), options, nil); err == nil {
		t.Fatal("nil consumer")
	}
	if _, err := c.Watch(context.Background(), options, nil); err == nil {
		t.Fatal("nil watcher")
	}
	for _, o := range []EventOptions{{}, {Network: options.Network, After: "bad"}, {Network: options.Network, Address: "bad"}, {Network: options.Network, Incoming: true}} {
		if _, err := c.Events(context.Background(), o, consume); err == nil {
			t.Fatal("invalid options")
		}
		if _, err := c.Watch(context.Background(), o, consume); err == nil {
			t.Fatal("invalid watch options")
		}
	}
	if _, err := c.Events(nil, options, consume); err == nil {
		t.Fatal("nil event context")
	}
	for _, body := range []string{strings.Repeat("x", 8193), `{}`, `{"error":{"code":"overloaded","message":"retry"}}`} {
		c = branchClient(t, 503, body)
		if _, err := c.Events(context.Background(), options, consume); err == nil {
			t.Fatal("event status")
		}
	}
	c = branchClient(t, 200, "")
	if _, err := c.Events(context.Background(), options, consume); err == nil {
		t.Fatal("event content type")
	}
	c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) { return nil, io.ErrClosedPipe })
	if _, err := c.Events(context.Background(), options, consume); err == nil {
		t.Fatal("event transport")
	}
	c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 503, Body: io.NopCloser(brokenReader{}), Header: make(http.Header)}, nil
	})
	if _, err := c.Events(context.Background(), options, consume); err == nil {
		t.Fatal("event body read")
	}
	b := clientBlock(t)
	for _, input := range []string{strings.Replace(frame, "event: confirmed", "event: confirmed\nevent: confirmed", 1), strings.Replace(frame, "data: ", "data: {\ndata: ", 1), strings.Replace(frame, `"height":1`, `"height":2`, 1), strings.Replace(frame, `"signature":"`, `"signature":"x`, 1)} {
		if _, err := parseEvents(context.Background(), strings.NewReader(input), options, consume); err == nil {
			t.Fatal("invalid frame")
		}
	}
	filtered := options
	filtered.Address = b.Account
	filtered.Incoming = true
	// A verified record with a different filter is rejected after signature validation.
	key := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, 32))
	filtered.Address = protocol.Address(key.Public().(ed25519.PublicKey))
	if _, err := parseEvents(context.Background(), strings.NewReader(frame), filtered, consume); err == nil {
		t.Fatal("filter mismatch")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := parseEvents(ctx, strings.NewReader(frame), options, consume); err == nil {
		t.Fatal("parser cancellation")
	}
}

func TestExtensionWaitAndWatchCancellation(t *testing.T) {
	b := clientBlock(t)
	ctx := context.Background()
	c := branchClient(t, 404, `{"error":{"code":"not_found","message":"pending"}}`)
	waitCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	if err := c.Wait(waitCtx, b.ID()); err == nil {
		t.Fatal("lookup wait ignored deadline")
	}
	cancel()
	canceled, stop := context.WithCancel(ctx)
	stop()
	if err := c.WaitConfirmed(canceled, b.ID()); err == nil {
		t.Fatal("canceled wait")
	}
	options, _ := eventFixture(t, 1)
	options.Address = b.Account
	options.Incoming = true
	options.After = string(options.Network) + ":0"
	c = branchClient(t, 503, `{"error":{"code":"overloaded","message":"retry"}}`)
	if _, err := c.Events(ctx, options, func(Event) error { return nil }); err == nil {
		t.Fatal("expected remote overload")
	}
	watchCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	if _, err := c.Watch(watchCtx, options, func(Event) error { return nil }); err == nil {
		t.Fatal("watch ignored deadline")
	}
	cancel()
}

func TestExtensionWatchRetrySaturation(t *testing.T) {
	if testing.Short() {
		t.Skip("bounded reconnect saturation schedule")
	}
	options, _ := eventFixture(t, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	calls := 0
	c := branchClient(t, 503, "")
	c.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		calls++
		if calls >= 7 {
			cancel()
		}
		return nil, io.ErrUnexpectedEOF
	})
	if _, err := c.Watch(ctx, options, func(Event) error { return nil }); err == nil || calls < 7 {
		t.Fatal(calls, err)
	}
}
