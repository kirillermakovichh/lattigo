// Package p2p exchanges signed consensus messages and verified ledger records.
package p2p

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"example.com/go-block-lattice/internal/consensus"
	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

const (
	maxMessage      = 20 << 20
	maxResponse     = 32 << 20
	protocolVersion = "1"
)

type Config struct {
	Store    *ledger.Store
	Engine   *consensus.Engine
	Peers    []string
	MaxPeers int
}
type peer struct {
	url           string
	queue         chan []byte
	connected     atomic.Bool
	height        atomic.Uint64
	queued        atomic.Int64
	pendingCursor int
	failures      atomic.Uint64
	retryAt       atomic.Int64
}
type rate struct {
	tokens  float64
	updated time.Time
}
type Network struct {
	cfg      Config
	peers    []*peer
	client   *http.Client
	slots    chan struct{}
	out      chan []byte
	stopped  atomic.Bool
	dropped  atomic.Uint64
	received atomic.Uint64
	queued   atomic.Int64
	mu       sync.Mutex
	rates    map[string]rate
}

type peerStatus struct {
	Version uint8         `json:"version"`
	Network protocol.Hash `json:"network"`
	Height  uint64        `json:"height"`
	Tip     protocol.Hash `json:"tip"`
	Peers   []string      `json:"peers"`
}

func ValidatePeer(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("peer URL requires http or https")
	}
	if u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("peer URL must contain only scheme and host")
	}
	if strings.ContainsAny(u.Host, "\r\n") {
		return "", errors.New("invalid peer host")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", errors.New("invalid peer port")
		}
	}
	return strings.TrimSuffix(u.String(), "/"), nil
}

func New(cfg Config) (*Network, error) {
	if cfg.Store == nil || cfg.Engine == nil {
		return nil, errors.New("P2P requires ledger and consensus engine")
	}
	if cfg.MaxPeers == 0 {
		cfg.MaxPeers = 32
	}
	if cfg.MaxPeers < 1 || cfg.MaxPeers > 128 {
		return nil, errors.New("max_peers must be 1..128")
	}
	if len(cfg.Peers) > cfg.MaxPeers {
		return nil, errors.New("configured peers exceed max_peers")
	}
	transport := &http.Transport{Proxy: nil, MaxIdleConns: cfg.MaxPeers * 2, MaxIdleConnsPerHost: 2, MaxConnsPerHost: 2, IdleConnTimeout: 30 * time.Second, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 3 * time.Second, DialContext: (&net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}).DialContext}
	n := &Network{cfg: cfg, client: &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error { return errors.New("peer redirects are disabled") }}, slots: make(chan struct{}, 16), out: make(chan []byte, 256), rates: make(map[string]rate)}
	seen := make(map[string]bool)
	for _, raw := range cfg.Peers {
		normalized, err := ValidatePeer(raw)
		if err != nil {
			return nil, fmt.Errorf("peer %q: %w", raw, err)
		}
		if seen[normalized] {
			continue
		}
		seen[normalized] = true
		n.peers = append(n.peers, &peer{url: normalized, queue: make(chan []byte, 32)})
	}
	return n, nil
}

// Publish is bounded and nonblocking. A saturated gossip queue sheds messages; periodic
// record synchronization and pending-block retransmission repair lost gossip.
func (n *Network) Publish(message consensus.Message) {
	if n.stopped.Load() {
		return
	}
	raw, err := json.Marshal(message)
	if err != nil || len(raw) > maxMessage {
		n.dropped.Add(1)
		return
	}
	if !reserve(&n.queued, int64(len(raw)), 32<<20) {
		n.dropped.Add(1)
		return
	}
	select {
	case n.out <- raw:
	default:
		n.queued.Add(-int64(len(raw)))
		n.dropped.Add(1)
	}
}

func (n *Network) Run(ctx context.Context) error {
	var wg sync.WaitGroup
	for _, p := range n.peers {
		wg.Add(1)
		go func(p *peer) { defer wg.Done(); n.runPeer(ctx, p) }(p)
	}
	defer func() { n.stopped.Store(true); wg.Wait(); n.client.CloseIdleConnections() }()
	for {
		select {
		case <-ctx.Done():
			return nil
		case raw := <-n.out:
			n.queued.Add(-int64(len(raw)))
			for _, p := range n.peers {
				if !reserve(&p.queued, int64(len(raw)), 24<<20) {
					n.dropped.Add(1)
					continue
				}
				select {
				case p.queue <- raw:
				default:
					p.queued.Add(-int64(len(raw)))
					n.dropped.Add(1)
				}
			}
		}
	}
}

func (n *Network) runPeer(ctx context.Context, p *peer) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	n.syncPeer(ctx, p)
	for {
		select {
		case <-ctx.Done():
			return
		case raw := <-p.queue:
			p.queued.Add(-int64(len(raw)))
			if time.Now().UnixNano() < p.retryAt.Load() {
				continue
			}
			if err := n.request(ctx, p, "POST", "/p2p/message", raw, nil); err != nil {
				p.connected.Store(false)
			}
		case <-ticker.C:
			if time.Now().UnixNano() >= p.retryAt.Load() {
				n.syncPeer(ctx, p)
			}
		}
	}
}

func (n *Network) request(ctx context.Context, p *peer, method, path string, body []byte, result any) (requestErr error) {
	defer func() {
		if requestErr != nil {
			attempt := p.failures.Add(1) - 1
			p.retryAt.Store(time.Now().Add(retryDelay(p.url, attempt)).UnixNano())
			p.connected.Store(false)
		} else {
			p.failures.Store(0)
			p.retryAt.Store(0)
		}
	}()
	req, err := http.NewRequestWithContext(ctx, method, p.url+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("X-BL-Network", string(n.cfg.Store.Status().Network))
	req.Header.Set("X-BL-Version", protocolVersion)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := n.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.Header.Get("X-BL-Network") != string(n.cfg.Store.Status().Network) || response.Header.Get("X-BL-Version") != protocolVersion {
		return errors.New("peer network or protocol mismatch")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("peer returned HTTP %d", response.StatusCode)
	}
	if result == nil {
		_, err = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return err
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil {
		return err
	}
	if len(raw) > maxResponse {
		return errors.New("peer response exceeds limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(result); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("peer response has trailing data")
	}
	return nil
}

func (n *Network) syncPeer(ctx context.Context, p *peer) {
	var status peerStatus
	if err := n.request(ctx, p, "GET", "/p2p/status", nil, &status); err != nil {
		p.connected.Store(false)
		return
	}
	local := n.cfg.Store.Status()
	if status.Version != 1 || status.Network != local.Network {
		p.connected.Store(false)
		return
	}
	p.connected.Store(true)
	p.height.Store(status.Height)
	// Only explicitly configured peer URLs are contacted. Advertised peers are diagnostic.
	// At most 16 certified records are processed per tick, bounding catch-up work.
	if status.Height > local.Height {
		var records []ledger.Record
		if err := n.request(ctx, p, "GET", "/p2p/records?after="+strconv.FormatUint(local.Height, 10)+"&limit=16", nil, &records); err != nil {
			// A maximum-size certificate fits individually even when a page does not.
			records = nil
			if err := n.request(ctx, p, "GET", "/p2p/records?after="+strconv.FormatUint(local.Height, 10)+"&limit=1", nil, &records); err != nil {
				p.connected.Store(false)
				return
			}
		}
		if len(records) == 0 || len(records) > 16 {
			p.connected.Store(false)
			return
		}
		for i := range records {
			current := n.cfg.Store.Status()
			if records[i].Height <= current.Height {
				continue
			}
			if records[i].Height != current.Height+1 {
				p.connected.Store(false)
				return
			}
			record := records[i]
			if err := n.cfg.Engine.Handle(consensus.Message{Type: "commit", Record: &record}); err != nil {
				p.connected.Store(false)
				return
			}
		}
	}
	// Repair lost block gossip. Votes and proposals are retransmitted by consensus rounds.
	pending := n.cfg.Engine.Pending()
	for sent := 0; sent < len(pending) && sent < 32; sent++ {
		if p.pendingCursor >= len(pending) {
			p.pendingCursor = 0
		}
		b := pending[p.pendingCursor]
		p.pendingCursor++
		raw, err := json.Marshal(consensus.Message{Type: "block", Block: &b})
		if err != nil {
			continue
		}
		if err := n.request(ctx, p, "POST", "/p2p/message", raw, nil); err != nil {
			p.connected.Store(false)
			return
		}
	}
}

func (n *Network) Syncing() bool {
	height := n.cfg.Store.Status().Height
	for _, p := range n.peers {
		if p.connected.Load() && p.height.Load() > height {
			return true
		}
	}
	return false
}
func (n *Network) Stats() any {
	connected := 0
	for _, p := range n.peers {
		if p.connected.Load() {
			connected++
		}
	}
	queued := n.queued.Load()
	for _, p := range n.peers {
		queued += p.queued.Load()
	}
	return map[string]any{"peers": len(n.peers), "connected": connected, "dropped": int(n.dropped.Load()), "received": int(n.received.Load()), "queued_bytes": int(queued)}
}

func (n *Network) allow(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	now := time.Now()
	entry, ok := n.rates[host]
	if !ok {
		if len(n.rates) >= 4096 {
			for key, value := range n.rates {
				if now.Sub(value.updated) > time.Minute {
					delete(n.rates, key)
				}
			}
			if len(n.rates) >= 4096 {
				return false
			}
		}
		entry = rate{tokens: 100, updated: now}
	}
	entry.tokens += now.Sub(entry.updated).Seconds() * 50
	if entry.tokens > 100 {
		entry.tokens = 100
	}
	entry.updated = now
	allowed := entry.tokens >= 1
	if allowed {
		entry.tokens--
	}
	n.rates[host] = entry
	return allowed
}

func (n *Network) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /p2p/status", func(w http.ResponseWriter, r *http.Request) {
		state := n.cfg.Store.Status()
		peers := make([]string, 0, len(n.peers))
		for _, p := range n.peers {
			peers = append(peers, p.url)
		}
		respond(w, 200, peerStatus{Version: 1, Network: state.Network, Height: state.Height, Tip: state.Tip, Peers: peers})
	})
	mux.HandleFunc("GET /p2p/records", n.records)
	mux.HandleFunc("POST /p2p/message", n.message)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-BL-Network", string(n.cfg.Store.Status().Network))
		w.Header().Set("X-BL-Version", protocolVersion)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Header.Get("X-BL-Network") != string(n.cfg.Store.Status().Network) || r.Header.Get("X-BL-Version") != protocolVersion {
			respond(w, 409, map[string]string{"error": "network or protocol mismatch"})
			return
		}
		if !n.allow(r.RemoteAddr) {
			respond(w, 429, map[string]string{"error": "peer rate limit"})
			return
		}
		select {
		case n.slots <- struct{}{}:
			defer func() { <-n.slots }()
		default:
			respond(w, 503, map[string]string{"error": "peer capacity reached"})
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func (n *Network) records(w http.ResponseWriter, r *http.Request) {
	after, err := strconv.ParseUint(r.URL.Query().Get("after"), 10, 64)
	if err != nil {
		respond(w, 400, map[string]string{"error": "invalid after height"})
		return
	}
	limit := 16
	if value := r.URL.Query().Get("limit"); value != "" {
		limit, err = strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 16 {
			respond(w, 400, map[string]string{"error": "limit must be 1..16"})
			return
		}
	}
	records := n.cfg.Store.Records(after, limit)
	if records == nil {
		records = []ledger.Record{}
	}
	// Encode one record at a time so a high-count request cannot allocate an
	// entire history-sized JSON page before enforcing the transport byte limit.
	raw := make([]byte, 0, 64<<10)
	raw = append(raw, '[')
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			respond(w, 500, map[string]string{"error": "cannot encode certified record"})
			return
		}
		overhead := 1
		if len(raw) > 1 {
			overhead++
		}
		if len(encoded) > maxResponse-len(raw)-overhead {
			if len(raw) == 1 {
				respond(w, 413, map[string]string{"error": "certified record exceeds transport limit"})
				return
			}
			break
		}
		if len(raw) > 1 {
			raw = append(raw, ',')
		}
		raw = append(raw, encoded...)
	}
	raw = append(raw, ']')
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}
func (n *Network) message(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxMessage)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var message consensus.Message
	if err := decoder.Decode(&message); err != nil {
		respond(w, 400, map[string]string{"error": "invalid bounded message"})
		return
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		respond(w, 400, map[string]string{"error": "trailing JSON value"})
		return
	}
	if err := n.cfg.Engine.Handle(message); err != nil {
		respond(w, 400, map[string]string{"error": err.Error()})
		return
	}
	n.received.Add(1)
	respond(w, 202, map[string]bool{"accepted": true})
}

// reserve bounds retained bytes in addition to queue item counts.
func reserve(counter *atomic.Int64, size, limit int64) bool {
	for {
		value := counter.Load()
		if size > limit-value {
			return false
		}
		if counter.CompareAndSwap(value, value+size) {
			return true
		}
	}
}
