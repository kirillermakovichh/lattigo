package p2p

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"example.com/go-block-lattice/internal/consensus"
	"example.com/go-block-lattice/internal/protocol"
	"net/http/httptest"
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

func TestInboundDeduplicatesOnlyAcceptedMessages(t *testing.T) {
	n := branchNetwork(t)
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	block := protocol.Block{Version: 1, Network: n.cfg.Store.Genesis().ID(), Kind: "send", Account: a, Representative: a, Balance: 99, Link: string(a)}
	if err := block.Sign(key); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(consensus.Message{Type: "block", Block: &block})
	call := func(body []byte) int {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/p2p/message", bytes.NewReader(body))
		n.message(w, r)
		return w.Code
	}
	if call(raw) != 202 || call(raw) != 202 || n.received.Load() != 1 || n.deduplicated.Load() != 1 {
		t.Fatal("accepted duplicate reprocessed")
	}
	invalid := []byte(`{"type":"unsupported"}`)
	if call(invalid) != 400 || call(invalid) != 400 || n.deduplicated.Load() != 1 {
		t.Fatal("rejected message cached")
	}
}
