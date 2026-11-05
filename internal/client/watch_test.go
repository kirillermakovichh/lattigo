package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"example.com/go-block-lattice/internal/event"
)

func TestWatchResumesDeliveredPosition(t *testing.T) {
	options, first := eventFixture(t, 1)
	_, second := eventFixture(t, 2)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if calls.Add(1) == 1 {
			_, _ = w.Write([]byte(first))
			return
		}
		if r.Header.Get("Last-Event-ID") != event.ID(options.Network, 1) {
			t.Error("resume position missing")
		}
		_, _ = w.Write([]byte(second))
	}))
	defer server.Close()
	c, _ := New(server.URL, "")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	stop := errors.New("delivered")
	count := 0
	last, err := c.Watch(ctx, options, func(e Event) error {
		count++
		if e.Height == 2 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || count != 2 || last != event.ID(options.Network, 1) || calls.Load() != 2 {
		t.Fatal(last, err, count, calls.Load())
	}
}
func TestWatchBackoffAndCancellation(t *testing.T) {
	for attempt := uint(0); attempt < 100; attempt++ {
		delay := eventRetryDelay(attempt, "position")
		if delay < 187*time.Millisecond || delay > 8*time.Second || delay != eventRetryDelay(attempt, "position") {
			t.Fatal(delay)
		}
	}
	options, _ := eventFixture(t, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer server.Close()
	c, _ := New(server.URL, "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Watch(ctx, options, func(Event) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	for _, err := range []error{ErrEventProtocol, &Error{Status: 400}, &Error{Status: 409, Code: "other"}, &eventConsumerError{errors.New("callback")}} {
		if retryEventError(err) {
			t.Fatal(err)
		}
	}
}
