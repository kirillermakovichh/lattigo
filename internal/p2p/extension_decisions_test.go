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

func TestExtensionPriorityQueueBudgets(t *testing.T) {
	n := branchNetwork(t)
	if len(n.Diagnostics().([]PeerDiagnostic)) != 0 {
		t.Fatal("unexpected peers")
	}
	n.priorityBytes.Store(32 << 20)
	n.Publish(consensus.Message{Type: "vote"})
	if n.dropped.Load() != 1 {
		t.Fatal("priority bytes unbounded")
	}
	n.priorityBytes.Store(0)
	for i := 0; i < 65; i++ {
		n.Publish(consensus.Message{Type: "vote", Proposal: &consensus.Proposal{Round: uint32(i)}})
	}
	if len(n.priority) != 64 || n.dropped.Load() != 2 {
		t.Fatal("priority frames unbounded")
	}
}

func TestExtensionPriorityFanoutBudgets(t *testing.T) {
	n := branchNetwork(t)
	bytesFull := &peer{url: "http://bytes", queue: make(chan []byte, 1), priority: make(chan []byte, 1)}
	bytesFull.priorityBytes.Store(24 << 20)
	itemsFull := &peer{url: "http://items", queue: make(chan []byte, 1), priority: make(chan []byte, 1)}
	n.peers = []*peer{bytesFull, itemsFull}
	started := make(chan struct{}, 2)
	n.client.Transport = peerTransport(func(r *http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- n.Run(ctx) }()
	defer func() { cancel(); <-done }()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("worker start")
		}
	}
	for i := 0; i < 3; i++ {
		n.Publish(consensus.Message{Type: "vote", Proposal: &consensus.Proposal{Round: uint32(i)}})
	}
	deadline := time.After(time.Second)
	for n.dropped.Load() < 5 {
		select {
		case <-deadline:
			t.Fatal("fanout overload not bounded")
		case <-time.After(time.Millisecond):
		}
	}
}

func TestExtensionPeerRetrySkipsUnavailableInterval(t *testing.T) {
	n := branchNetwork(t)
	p := &peer{url: "http://offline", queue: make(chan []byte, 1), priority: make(chan []byte, 1)}
	p.failures.Store(100)
	calls := make(chan struct{}, 4)
	n.client.Transport = peerTransport(func(*http.Request) (*http.Response, error) { calls <- struct{}{}; return nil, io.ErrUnexpectedEOF })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); n.runPeer(ctx, p) }()
	select {
	case <-calls:
	case <-time.After(time.Second):
		t.Fatal("initial request")
	}
	p.queue <- []byte("ordinary")
	p.queued.Add(8)
	timer := time.NewTimer(1200 * time.Millisecond)
	select {
	case <-calls:
		cancel()
		<-done
		t.Fatal("retry deadline ignored")
	case <-timer.C:
	}
	cancel()
	<-done
}

func TestExtensionDispatcherBlockingCases(t *testing.T) {
	for _, kind := range []string{"priority", "ordinary", "cancel"} {
		t.Run(kind, func(t *testing.T) {
			n := branchNetwork(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			timer := time.AfterFunc(30*time.Millisecond, func() {
				switch kind {
				case "priority":
					n.priorityBytes.Add(1)
					n.priority <- []byte("p")
				case "ordinary":
					n.queued.Add(1)
					n.out <- []byte("o")
				case "cancel":
					cancel()
				}
			})
			defer timer.Stop()
			_, ok := n.nextOutbound(ctx)
			if ok != (kind != "cancel") {
				t.Fatal("dispatch result")
			}
		})
	}
	n := branchNetwork(t)
	n.priorityBurst = 4
	n.priorityBytes.Store(1)
	n.priority <- []byte("p")
	if _, ok := n.nextOutbound(context.Background()); !ok {
		t.Fatal("priority failed without ordinary backlog")
	}
}

func TestExtensionPeerTickAfterDelivery(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "successful", true: "failed"}[failed], func(t *testing.T) {
			n := branchNetwork(t)
			p := &peer{url: "http://peer", queue: make(chan []byte, 1), priority: make(chan []byte, 1)}
			p.failures.Store(100)
			posted := make(chan struct{})
			n.client.Transport = peerTransport(func(r *http.Request) (*http.Response, error) {
				if r.Method == "POST" {
					time.Sleep(1100 * time.Millisecond)
					close(posted)
					if failed {
						return nil, io.ErrUnexpectedEOF
					}
				}
				body := ""
				if r.Method == "GET" {
					raw, _ := json.Marshal(peerStatus{Version: 1, Network: n.cfg.Store.Status().Network})
					body = string(raw)
				}
				h := make(http.Header)
				h.Set("X-BL-Network", string(n.cfg.Store.Status().Network))
				h.Set("X-BL-Version", "1")
				return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(body))}, nil
			})
			p.queue <- []byte("ordinary")
			p.queued.Add(8)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { defer close(done); n.runPeer(ctx, p) }()
			select {
			case <-posted:
			case <-time.After(3 * time.Second):
				cancel()
				<-done
				t.Fatal("delivery stalled")
			}
			// Let the already ready periodic tick run before cancellation.
			timer := time.NewTimer(30 * time.Millisecond)
			<-timer.C
			cancel()
			<-done
		})
	}
}

func TestExtensionPeerReceivesPriorityWhileIdle(t *testing.T) {
	n := branchNetwork(t)
	p := &peer{url: "http://peer", queue: make(chan []byte, 1), priority: make(chan []byte, 1)}
	ready := make(chan struct{})
	delivered := make(chan struct{})
	n.client.Transport = peerTransport(func(r *http.Request) (*http.Response, error) {
		body := ""
		if r.Method == "GET" {
			raw, _ := json.Marshal(peerStatus{Version: 1, Network: n.cfg.Store.Status().Network})
			body = string(raw)
			close(ready)
		} else {
			close(delivered)
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
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("peer initialization")
	}
	timer := time.NewTimer(30 * time.Millisecond)
	<-timer.C
	p.priorityBytes.Add(6)
	p.priority <- []byte("urgent")
	select {
	case <-delivered:
	case <-time.After(time.Second):
		t.Fatal("idle priority delivery")
	}
}

// Cancel precisely between the two nonblocking scheduling checks. A transport
// with no client timeout keeps the context owned solely by the peer worker.
type peerSchedulingContext struct {
	context.Context
	calls  int
	closed chan struct{}
}

func (c *peerSchedulingContext) Done() <-chan struct{} {
	c.calls++
	if c.calls == 2 {
		close(c.closed)
	}
	return c.closed
}
func (c *peerSchedulingContext) Err() error {
	if c.calls >= 2 {
		return context.Canceled
	}
	return nil
}
func TestPeerCancellationBetweenSchedulingChecks(t *testing.T) {
	n := branchNetwork(t)
	p := &peer{url: "http://peer", queue: make(chan []byte, 1), priority: make(chan []byte, 1)}
	n.client.Timeout = 0
	n.client.Transport = peerTransport(func(r *http.Request) (*http.Response, error) {
		raw, _ := json.Marshal(peerStatus{Version: 1, Network: n.cfg.Store.Status().Network})
		h := make(http.Header)
		h.Set("X-BL-Network", string(n.cfg.Store.Status().Network))
		h.Set("X-BL-Version", "1")
		return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})
	ctx := &peerSchedulingContext{Context: context.Background(), closed: make(chan struct{})}
	n.runPeer(ctx, p)
	if ctx.calls != 2 {
		t.Fatal("cancellation was not checked between scheduling phases", ctx.calls)
	}
}
