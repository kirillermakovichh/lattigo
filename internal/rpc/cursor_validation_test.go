package rpc

import (
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
)

func TestCursorRejectsAuthenticatedMalformedPayloads(t *testing.T) {
	c := cursorCodec{}
	p := cursorFixture()
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{[]byte("{"), append(append([]byte{}, raw...), []byte("{}")...), []byte(`{"unknown":true}`), []byte(`{"version":2}`)} {
		token := base64.RawURLEncoding.EncodeToString(append(data, c.tag(data)...))
		if _, err := c.decode(token); err == nil {
			t.Fatal("authenticated malformed cursor accepted")
		}
	}
	for _, change := range []func(*pageCursor){func(p *pageCursor) { p.Version = 2 }, func(p *pageCursor) { p.Limit = 0 }, func(p *pageCursor) { p.Network = "bad" }, func(p *pageCursor) { p.Account = "bad" }, func(p *pageCursor) { p.Tip = "bad" }, func(p *pageCursor) { p.Kind = "other" }, func(p *pageCursor) { p.Height = 0; p.Before = 1 }, func(p *pageCursor) { p.After = protocol.Digest([]byte("source")) }, func(p *pageCursor) { p.Kind = "receivables" }, func(p *pageCursor) { p.Kind = "receivables"; p.Before = 0; p.After = "bad" }} {
		bad := p
		change(&bad)
		if _, err := c.encode(bad); err == nil {
			t.Fatal("invalid cursor scope encoded")
		}
	}
	p.Kind = "receivables"
	p.Before = 0
	p.After = protocol.Digest([]byte("source"))
	token, err := c.encode(p)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := c.decode(token); err != nil || got != p {
		t.Fatal("receivable cursor round trip")
	}
}
func TestPageEndpointsRejectScopeChangesAndMalformedQueries(t *testing.T) {
	s, a, _ := pageFixture(t)
	scope := pageCursor{Version: 1, Network: s.cfg.Store.Status().Network, Kind: "history", Account: a, Height: 3, Tip: s.cfg.Store.Status().Tip, Before: 2, Limit: 1}
	token, err := s.cursors.encode(scope)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/v1/history/page?address=bad", "/v1/history/page?address=" + string(a) + "&unknown=x", "/v1/history/page?address=" + string(a) + "&limit=0", "/v1/history/page?address=" + string(a) + "&limit=bad", "/v1/history/page?address=" + string(a) + "&limit=1&limit=1", "/v1/history/page?address=" + string(a) + "&cursor=" + token + "&limit=2", "/v1/receivables/page?address=" + string(a) + "&cursor=" + token, "/v1/history/page?address=" + string(a) + "&cursor=%", "/v1/history/page?address=" + string(a) + "&cursor=" + strings.Repeat("x", 4097)} {
		out := httptest.NewRecorder()
		s.Handler().ServeHTTP(out, httptest.NewRequest("GET", path, nil))
		if out.Code != 400 {
			t.Fatalf("invalid cursor query: %s => %d", path, out.Code)
		}
	}
	scope.Tip = protocol.Digest([]byte("wrong tip"))
	token, err = s.cursors.encode(scope)
	if err != nil {
		t.Fatal(err)
	}
	out := httptest.NewRecorder()
	s.Handler().ServeHTTP(out, httptest.NewRequest("GET", "/v1/history/page?address="+string(a)+"&cursor="+token, nil))
	if out.Code != 409 {
		t.Fatal("invalid historical anchor did not require restart")
	}
}
