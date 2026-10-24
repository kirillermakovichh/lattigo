package p2p

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
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

func TestPeerCatchupRejectionAndFallbackOutcomes(t *testing.T) {
	for _, scenario := range []string{"version", "network", "fallback-fails", "fallback-empty", "fallback-stale", "gap", "certificate"} {
		t.Run(scenario, func(t *testing.T) {
			n := branchNetwork(t)
			p := &peer{url: "http://local.invalid"}
			n.peers = []*peer{p}
			state := n.cfg.Store.Status()
			status := peerStatus{Version: 1, Network: state.Network, Height: state.Height + 1}
			if scenario == "version" {
				status.Version = 2
			}
			if scenario == "network" {
				status.Network = "wrong"
			}
			pages := 0
			n.client.Transport = peerTransport(func(r *http.Request) (*http.Response, error) {
				header := make(http.Header)
				header.Set("X-BL-Network", string(state.Network))
				header.Set("X-BL-Version", "1")
				var value any = status
				if r.URL.Path == "/p2p/records" {
					pages++
					if strings.HasPrefix(scenario, "fallback-") && (pages == 1 || scenario == "fallback-fails") {
						return nil, io.ErrClosedPipe
					}
					switch scenario {
					case "fallback-empty":
						value = []ledger.Record{}
					case "fallback-stale":
						value = []ledger.Record{{Height: state.Height}}
					case "gap":
						value = []ledger.Record{{Height: state.Height + 2}}
					default:
						value = []ledger.Record{{Height: state.Height + 1}}
					}
				}
				raw, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
			})
			n.syncPeer(context.Background(), p)
			wantConnected := scenario == "fallback-stale"
			if p.connected.Load() != wantConnected {
				t.Fatalf("connected=%v", p.connected.Load())
			}
			if strings.HasPrefix(scenario, "fallback-") && pages != 2 {
				t.Fatalf("fallback pages=%d", pages)
			}
			if n.cfg.Store.Status().Height != state.Height {
				t.Fatal("unverified history applied")
			}
			if n.Syncing() != wantConnected {
				t.Fatal("sync observation disagrees with peer state")
			}
			stats := n.Stats().(map[string]any)
			wantCount := 0
			if wantConnected {
				wantCount = 1
			}
			if stats["connected"] != wantCount || stats["peers"] != 1 {
				t.Fatal(stats)
			}
		})
	}
}

func TestPeerRequestSizeAndConstructionOutcomes(t *testing.T) {
	n := branchNetwork(t)
	if err := n.request(context.Background(), &peer{url: "%"}, "GET", "", nil, nil); err == nil {
		t.Fatal("invalid request accepted")
	}
	network := string(n.cfg.Store.Status().Network)
	n.client.Transport = peerTransport(func(*http.Request) (*http.Response, error) {
		header := make(http.Header)
		header.Set("X-BL-Network", network)
		header.Set("X-BL-Version", "1")
		return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(io.LimitReader(repeatedSpace{}, maxResponse+1))}, nil
	})
	var result peerStatus
	if err := n.request(context.Background(), &peer{url: "http://local.invalid"}, "GET", "", nil, &result); err == nil || !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("size rejection: %v", err)
	}
	if !n.allow("host-without-port") {
		t.Fatal("host-only rate key rejected")
	}
}

type repeatedSpace struct{}

func (repeatedSpace) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = ' '
	}
	return len(p), nil
}

func TestPeerFanoutShedsFullItemAndByteQueues(t *testing.T) {
	n := branchNetwork(t)
	bytesFull := &peer{url: "http://bytes.invalid", queue: make(chan []byte, 1)}
	bytesFull.queued.Store(24 << 20)
	itemsFull := &peer{url: "http://items.invalid", queue: make(chan []byte, 1)}
	n.peers = []*peer{bytesFull, itemsFull}
	started := make(chan struct{}, 2)
	n.client.Transport = peerTransport(func(r *http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- n.Run(ctx) }()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("peer worker did not start")
		}
	}
	for i := 0; i < 3; i++ {
		n.Publish(consensus.Message{Type: "unknown"})
	}
	deadline := time.After(time.Second)
	for n.dropped.Load() < 5 {
		select {
		case <-deadline:
			t.Fatal("fanout capacity did not shed excess work")
		case <-time.After(time.Millisecond):
		}
	}
	if len(itemsFull.queue) != 1 || itemsFull.queued.Load() == 0 {
		t.Fatal("accepted queued bytes were not retained")
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestPendingRepairCursorAndSubmissionFailure(t *testing.T) {
	n := branchNetwork(t)
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	block := protocol.Block{Version: 1, Network: n.cfg.Store.Genesis().ID(), Kind: "send", Account: a, Representative: a, Balance: 99, Link: string(a)}
	if err := block.Sign(key); err != nil {
		t.Fatal(err)
	}
	if _, err := n.cfg.Engine.Submit(block); err != nil {
		t.Fatal(err)
	}
	p := &peer{url: "http://local.invalid", pendingCursor: 99}
	posts := 0
	n.client.Transport = peerTransport(func(r *http.Request) (*http.Response, error) {
		header := make(http.Header)
		header.Set("X-BL-Network", string(block.Network))
		header.Set("X-BL-Version", "1")
		body := ""
		if r.Method == "POST" {
			posts++
			return nil, io.ErrClosedPipe
		}
		raw, err := json.Marshal(peerStatus{Version: 1, Network: block.Network})
		if err != nil {
			t.Fatal(err)
		}
		body = string(raw)
		return &http.Response{StatusCode: 200, Header: header, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	n.syncPeer(context.Background(), p)
	if posts != 1 || p.pendingCursor != 1 || p.connected.Load() {
		t.Fatal("pending repair failed to wrap cursor and disconnect rejected peer")
	}
	before := n.dropped.Load()
	n.Publish(consensus.Message{Type: strings.Repeat("x", maxMessage+1)})
	if n.dropped.Load() != before+1 {
		t.Fatal("oversized publication retained")
	}
}

func TestCertifiedPeerPagesEncodeMultipleRecords(t *testing.T) {
	n := branchNetwork(t)
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	for i := 0; i < 2; i++ {
		state, err := n.cfg.Store.Account(a)
		if err != nil {
			t.Fatal(err)
		}
		block := protocol.Block{Version: 1, Network: n.cfg.Store.Genesis().ID(), Kind: "send", Account: a, Previous: state.Frontier, Representative: a, Balance: state.Balance - 1, Link: string(a)}
		if err := block.Sign(key); err != nil {
			t.Fatal(err)
		}
		status := n.cfg.Store.Status()
		vote := ledger.Vote{Network: block.Network, Height: status.Height + 1, Parent: status.Tip, Block: block.ID(), Phase: "precommit", Representative: a}
		if err := vote.Sign(key); err != nil {
			t.Fatal(err)
		}
		if err := n.cfg.Store.Commit(ledger.Record{Height: status.Height + 1, Previous: status.Tip, Block: block, Votes: []ledger.Vote{vote}}); err != nil {
			t.Fatal(err)
		}
	}
	out := httptest.NewRecorder()
	n.records(out, httptest.NewRequest("GET", "/p2p/records?after=0&limit=16", nil))
	var records []ledger.Record
	if err := json.Unmarshal(out.Body.Bytes(), &records); err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0].Height != 1 || records[1].Height != 2 || out.Body.Len() > maxResponse {
		t.Fatal("certified page ordering or size invalid")
	}
}
