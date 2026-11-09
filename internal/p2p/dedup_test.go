package p2p

import (
	"crypto/sha256"
	"example.com/go-block-lattice/internal/consensus"
	"testing"
	"time"
)

func TestBurstCacheExpiryAndCapacity(t *testing.T) {
	var cache burstCache
	now := time.Unix(100, 0)
	key := sha256.Sum256([]byte("message"))
	cache.remember(key, now)
	if !cache.contains(key, now) || cache.contains(key, now.Add(burstWindow)) {
		t.Fatal("expiry contract")
	}
	for i := 0; i < maxBurstEntries+10; i++ {
		cache.remember(sha256.Sum256([]byte{byte(i), byte(i >> 8)}), now)
	}
	if len(cache.entries) > maxBurstEntries {
		t.Fatal("cache exceeded budget")
	}
	cache.remember(key, now.Add(time.Second))
	if !cache.contains(key, now.Add(time.Second)) {
		t.Fatal("expired cache did not recycle")
	}
}
func TestOutboundBurstDeduplication(t *testing.T) {
	n := branchNetwork(t)
	n.Publish(consensus.Message{Type: "block"})
	n.Publish(consensus.Message{Type: "block"})
	if len(n.out) != 1 || n.deduplicated.Load() != 1 {
		t.Fatal("identical burst retained")
	}
}
