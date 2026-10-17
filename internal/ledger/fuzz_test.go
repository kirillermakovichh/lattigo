package ledger

import (
	"crypto/ed25519"
	"encoding/json"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
)

func FuzzVoteJSON(f *testing.F) {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	vote := Vote{Network: protocol.Digest([]byte("fuzz-network")), Height: 1, Phase: "prevote", Parent: protocol.Digest([]byte("parent")), Representative: protocol.Address(key.Public().(ed25519.PublicKey))}
	if err := vote.Sign(key); err != nil {
		f.Fatal(err)
	}
	seed, err := json.Marshal(vote)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 8192 {
			t.Skip()
		}
		var v Vote
		if json.Unmarshal(data, &v) != nil {
			return
		}
		if v.Verify() != nil {
			return
		}
		v.Height++
		if v.Verify() == nil {
			t.Fatal("vote signature did not bind height")
		}
	})
}
