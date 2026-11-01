package rpc

import (
	"crypto/ed25519"
	"math"
	"strings"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
)

func cursorFixture() pageCursor {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	return pageCursor{Version: 1, Network: protocol.Digest([]byte("cursor-network")), Kind: "history", Account: protocol.Address(key.Public().(ed25519.PublicKey)), Height: math.MaxUint64, Tip: protocol.Digest([]byte("cursor-tip")), Before: math.MaxUint64, Limit: 1000}
}
func TestCursorRoundTripAuthenticatesScopeAndBounds(t *testing.T) {
	codec := cursorCodec{}
	original := cursorFixture()
	token, err := codec.encode(original)
	if err != nil || len(token) > maxCursorBytes {
		t.Fatal("cursor encoding exceeds parser budget")
	}
	got, err := codec.decode(token)
	if err != nil || got != original {
		t.Fatal("cursor round trip changed scope")
	}
	other := cursorCodec{1}
	if _, err := other.decode(token); err == nil {
		t.Fatal("another server accepted a cursor")
	}
	for _, raw := range []string{"", token + "=", strings.Repeat("x", maxCursorBytes+1), "%", token[:len(token)-2] + "AA"} {
		if _, err := codec.decode(raw); err == nil {
			t.Fatal("invalid cursor authenticated")
		}
	}
	original.Limit = 1001
	if _, err := codec.encode(original); err == nil {
		t.Fatal("oversized cursor page admitted")
	}
}
