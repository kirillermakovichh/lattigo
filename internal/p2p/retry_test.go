package p2p

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRetryDelayBoundsAndReproducibility(t *testing.T) {
	for attempt := uint64(0); attempt < 100; attempt++ {
		delay := retryDelay("http://peer", attempt)
		if delay < 250*time.Millisecond || delay >= 24*time.Second || delay != retryDelay("http://peer", attempt) {
			t.Fatal(attempt, delay)
		}
	}
	if retryDelay("http://peer", 6) != retryDelay("http://peer", ^uint64(0)) {
		t.Fatal("retry saturation")
	}
	if retryDelay("http://one", 2) == retryDelay("http://two", 2) {
		t.Fatal("peer jitter collapsed")
	}
}

func TestPeerRequestRetryState(t *testing.T) {
	n := branchNetwork(t)
	p := &peer{url: "http://peer.invalid"}
	n.client.Transport = peerTransport(func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") })
	if err := n.request(context.Background(), p, "GET", "/p2p/status", nil, nil); err == nil || p.failures.Load() != 1 || p.retryAt.Load() <= time.Now().UnixNano() {
		t.Fatal("failure did not back off")
	}
	n.client.Transport = peerTransport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"X-Bl-Network": {string(n.cfg.Store.Status().Network)}, "X-Bl-Version": {"1"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	if err := n.request(context.Background(), p, "GET", "/p2p/status", nil, nil); err != nil || p.failures.Load() != 0 || p.retryAt.Load() != 0 {
		t.Fatal("success did not reset retry", err)
	}
}
