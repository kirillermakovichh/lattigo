package p2p

import (
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
