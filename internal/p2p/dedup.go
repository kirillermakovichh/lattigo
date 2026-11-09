package p2p

import (
	"bytes"
	"crypto/sha256"
	"sync"
	"time"
)

const burstWindow = 250 * time.Millisecond
const maxBurstEntries = 2048

type burstCache struct {
	mu      sync.Mutex
	entries map[[32]byte]time.Time
}

func (c *burstCache) contains(key [32]byte, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return now.Before(c.entries[key])
}
func (c *burstCache) remember(key [32]byte, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[[32]byte]time.Time)
	}
	if len(c.entries) >= maxBurstEntries {
		for k, expires := range c.entries {
			if !now.Before(expires) {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= maxBurstEntries {
			return
		}
	}
	c.entries[key] = now.Add(burstWindow)
}
func priorityMessage(kind string) bool {
	return kind == "vote" || kind == "proposal" || kind == "commit"
}
func priorityFrame(raw []byte) bool {
	return bytes.HasPrefix(raw, []byte(`{"type":"vote"`)) || bytes.HasPrefix(raw, []byte(`{"type":"proposal"`)) || bytes.HasPrefix(raw, []byte(`{"type":"commit"`))
}
func (n *Network) duplicateOutbound(raw []byte) bool {
	key := sha256.Sum256(raw)
	now := time.Now()
	if n.outbound.contains(key, now) {
		n.deduplicated.Add(1)
		return true
	}
	n.outbound.remember(key, now)
	return false
}
