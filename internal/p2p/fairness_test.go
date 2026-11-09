package p2p

import (
	"context"
	"encoding/json"
	"example.com/go-block-lattice/internal/consensus"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSlowPeerDoesNotBlockHealthyDelivery(t *testing.T) {
	n := branchNetwork(t)
	for _, url := range []string{"http://slow", "http://healthy"} {
		n.peers = append(n.peers, &peer{url: url, queue: make(chan []byte, 32), priority: make(chan []byte, 16)})
	}
	healthy := make(chan struct{}, 10)
	n.client.Transport = peerTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "slow" {
			<-r.Context().Done()
			return nil, r.Context().Err()
		}
		body := ""
		if r.URL.Path == "/p2p/status" {
			raw, _ := json.Marshal(peerStatus{Version: 1, Network: n.cfg.Store.Status().Network})
			body = string(raw)
		} else {
			healthy <- struct{}{}
		}
		h := make(http.Header)
		h.Set("X-BL-Network", string(n.cfg.Store.Status().Network))
		h.Set("X-BL-Version", "1")
		return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- n.Run(ctx) }()
	n.Publish(consensus.Message{Type: "vote"})
	select {
	case <-healthy:
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("slow peer blocked healthy delivery")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("peer workers did not join")
	}
}

func TestPeerPriorityBurstYieldsToOrdinaryQueue(t *testing.T) {
	n := branchNetwork(t)
	p := &peer{url: "http://peer", queue: make(chan []byte, 32), priority: make(chan []byte, 16)}
	for i := 0; i < 5; i++ {
		p.priority <- []byte("urgent")
		p.priorityBytes.Add(6)
	}
	p.queue <- []byte("ordinary")
	p.queued.Add(8)
	delivered := make(chan string, 6)
	n.client.Transport = peerTransport(func(r *http.Request) (*http.Response, error) {
		body := ""
		if r.Method == "POST" {
			raw, _ := io.ReadAll(r.Body)
			delivered <- string(raw)
		} else {
			raw, _ := json.Marshal(peerStatus{Version: 1, Network: n.cfg.Store.Status().Network})
			body = string(raw)
		}
		h := make(http.Header)
		h.Set("X-BL-Network", string(n.cfg.Store.Status().Network))
		h.Set("X-BL-Version", "1")
		return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); n.runPeer(ctx, p) }()
	defer func() { cancel(); <-done }()
	for i := 0; i < 6; i++ {
		select {
		case value := <-delivered:
			if (i == 4) != (value == "ordinary") {
				t.Fatal("priority fairness", i, value)
			}
		case <-time.After(time.Second):
			t.Fatal("queue stalled")
		}
	}
	if p.queued.Load() != 0 || p.priorityBytes.Load() != 0 {
		t.Fatal("delivered frames retained bytes")
	}
}
