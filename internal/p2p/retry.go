package p2p

import (
	"crypto/sha256"
	"encoding/binary"
	"time"
)

// retryDelay is deterministic for a peer/attempt and bounded to avoid retry storms.
// Attempts saturate before shifting, including after arbitrarily long outages.
func retryDelay(peerURL string, attempt uint64) time.Duration {
	if attempt > 6 {
		attempt = 6
	}
	base := 250 * time.Millisecond * time.Duration(uint64(1)<<attempt)
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], attempt)
	h := sha256.New()
	h.Write([]byte(peerURL))
	h.Write(counter[:])
	sum := h.Sum(nil)
	jitter := time.Duration(binary.BigEndian.Uint64(sum[:8]) % uint64(base/2))
	return base + jitter
}
