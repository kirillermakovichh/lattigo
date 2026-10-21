package p2p

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/go-block-lattice/internal/consensus"
	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

func branchNetwork(t *testing.T) *Network {
	t.Helper()
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	g := protocol.Genesis{Version: 1, Name: "peer-branches", Supply: 100, Allocations: []protocol.Allocation{{Account: a, Balance: 100, Representative: a}}}
	s, err := ledger.Open(t.TempDir(), g)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	e, err := consensus.New(consensus.Config{Store: s})
	if err != nil {
		t.Fatal(err)
	}
	n, err := New(Config{Store: s, Engine: e})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

type peerTransport func(*http.Request) (*http.Response, error)

func (f peerTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type readFailure struct{}

func (readFailure) Read([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestPeerConstructionAndHTTPAdmissionBranches(t *testing.T) {
	n := branchNetwork(t)
	for _, cfg := range []Config{{}, {Store: n.cfg.Store}, {Store: n.cfg.Store, Engine: n.cfg.Engine, MaxPeers: -1}, {Store: n.cfg.Store, Engine: n.cfg.Engine, MaxPeers: 129}, {Store: n.cfg.Store, Engine: n.cfg.Engine, MaxPeers: 1, Peers: []string{"http://a.invalid", "http://b.invalid"}}, {Store: n.cfg.Store, Engine: n.cfg.Engine, Peers: []string{"file:///bad"}}} {
		if _, err := New(cfg); err == nil {
			t.Fatal("invalid peer configuration accepted")
		}
	}
	normalized, err := New(Config{Store: n.cfg.Store, Engine: n.cfg.Engine, Peers: []string{"http://a.invalid/", "http://a.invalid"}})
	if err != nil || len(normalized.peers) != 1 {
		t.Fatal("peer normalization failed")
	}
	for _, url := range []string{"%", "http://node.invalid:0", "http://node.invalid:-1", "http://node.invalid:99999"} {
		if _, err := ValidatePeer(url); err == nil {
			t.Fatal("invalid port or URL accepted")
		}
	}
	call := func(path, body string, headers bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, strings.NewReader(body))
		if strings.HasPrefix(path, "/p2p/records") || path == "/p2p/status" {
			req.Method = "GET"
		}
		if headers {
			req.Header.Set("X-BL-Version", "1")
			req.Header.Set("X-BL-Network", string(n.cfg.Store.Status().Network))
		}
		out := httptest.NewRecorder()
		n.Handler().ServeHTTP(out, req)
		return out
	}
	if call("/p2p/status", "", false).Code != 409 {
		t.Fatal("mismatched network admitted")
	}
	if call("/p2p/status", "", true).Code != 200 {
		t.Fatal("status rejected")
	}
	for _, path := range []string{"/p2p/records?after=bad", "/p2p/records?after=0&limit=0", "/p2p/records?after=0&limit=17"} {
		if call(path, "", true).Code != 400 {
			t.Fatal("invalid page accepted")
		}
	}
	if call("/p2p/records?after=0", "", true).Body.String() != "[]" {
		t.Fatal("empty certified page changed")
	}
	for _, body := range []string{"{", "{}{}", `{"type":"unsupported"}`, strings.Repeat(" ", maxMessage+1)} {
		if call("/p2p/message", body, true).Code != 400 {
			t.Fatal("invalid message accepted")
		}
	}
	for i := 0; i < cap(n.slots); i++ {
		n.slots <- struct{}{}
	}
	if call("/p2p/status", "", true).Code != 503 {
		t.Fatal("concurrency capacity ignored")
	}
	for i := 0; i < cap(n.slots); i++ {
		<-n.slots
	}
	for i := 0; i < 110; i++ {
		call("/p2p/status", "", true)
	}
	if call("/p2p/status", "", true).Code != 429 {
		t.Fatal("source rate ignored")
	}
	n.mu.Lock()
	n.rates = map[string]rate{}
	for i := 0; i < 4096; i++ {
		n.rates[fmt.Sprintf("host-%d", i)] = rate{tokens: 100, updated: time.Now()}
	}
	n.mu.Unlock()
	if n.allow("new:1") {
		t.Fatal("source table exceeded capacity")
	}
	n.mu.Lock()
	n.rates["host-0"] = rate{updated: time.Now().Add(-2 * time.Minute)}
	n.mu.Unlock()
	if !n.allow("new:1") {
		t.Fatal("expired source did not release capacity")
	}
}
func TestPeerTransportAndQueueBranches(t *testing.T) {
	n := branchNetwork(t)
	p := &peer{url: "http://local.invalid"}
	network := string(n.cfg.Store.Status().Network)
	for name, response := range map[string]struct {
		status                 int
		network, version, body string
	}{"network": {200, "wrong", "1", "{}"}, "version": {200, network, "2", "{}"}, "http": {503, network, "1", "{}"}, "json": {200, network, "1", "{"}, "trailing": {200, network, "1", "{}{}"}, "unknown": {200, network, "1", `{"unknown":true}`}} {
		t.Run(name, func(t *testing.T) {
			n.client.Transport = peerTransport(func(*http.Request) (*http.Response, error) {
				header := make(http.Header)
				header.Set("X-BL-Network", response.network)
				header.Set("X-BL-Version", response.version)
				return &http.Response{StatusCode: response.status, Header: header, Body: io.NopCloser(strings.NewReader(response.body))}, nil
			})
			var status peerStatus
			if n.request(context.Background(), p, "GET", "/p2p/status", nil, &status) == nil {
				t.Fatal("invalid transport response accepted")
			}
		})
	}
	n.client.Transport = peerTransport(func(*http.Request) (*http.Response, error) { return nil, io.ErrClosedPipe })
	if n.request(context.Background(), p, "GET", "/p2p/status", nil, nil) == nil {
		t.Fatal("transport failure ignored")
	}
	n.client.Transport = peerTransport(func(*http.Request) (*http.Response, error) {
		h := make(http.Header)
		h.Set("X-BL-Network", network)
		h.Set("X-BL-Version", "1")
		return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(readFailure{})}, nil
	})
	var status peerStatus
	if n.request(context.Background(), p, "GET", "/p2p/status", nil, &status) == nil {
		t.Fatal("body failure ignored")
	}
	n.syncPeer(context.Background(), p)
	if p.connected.Load() {
		t.Fatal("failed peer reported connected")
	}
	for i := 0; i < 300; i++ {
		n.Publish(consensus.Message{Type: "unknown"})
	}
	if len(n.out) != 256 || n.dropped.Load() == 0 {
		t.Fatal("item queue budget ignored")
	}
	n.queued.Store(32 << 20)
	before := n.dropped.Load()
	n.Publish(consensus.Message{Type: "block"})
	if n.dropped.Load() != before+1 {
		t.Fatal("byte budget ignored")
	}
	n.stopped.Store(true)
	size := len(n.out)
	n.Publish(consensus.Message{})
	if len(n.out) != size {
		t.Fatal("stopped publisher retained work")
	}
	n.stopped.Store(false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if n.Run(ctx) != nil {
		t.Fatal("cancellation failed")
	}
}
