package protocol

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"testing"
)

func FuzzAmountJSON(f *testing.F) {
	for _, seed := range []string{`"0"`, `"18446744073709551615"`, `"01"`, `-1`, `null`, `{}`} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4096 {
			t.Skip()
		}
		var amount Amount
		if json.Unmarshal(data, &amount) != nil {
			return
		}
		encoded, err := json.Marshal(amount)
		if err != nil {
			t.Fatal(err)
		}
		var roundTrip Amount
		if err = json.Unmarshal(encoded, &roundTrip); err != nil || roundTrip != amount {
			t.Fatal("amount roundtrip changed value")
		}
		parsed, err := ParseAmount(amount.String())
		if err != nil || parsed != amount {
			t.Fatal("text amount changed value")
		}
	})
}
func FuzzBlockJSON(f *testing.F) {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	account := Address(key.Public().(ed25519.PublicKey))
	block := Block{Version: 1, Network: Digest([]byte("fuzz-block")), Kind: "send", Account: account, Representative: account, Balance: 99, Link: string(account)}
	if err := block.Sign(key); err != nil {
		f.Fatal(err)
	}
	seed, err := json.Marshal(block)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 8192 {
			t.Skip()
		}
		var b Block
		if json.Unmarshal(data, &b) != nil {
			return
		}
		canonical, err := b.Canonical()
		if err != nil {
			if b.ID() != "" {
				t.Fatal("invalid block has identifier")
			}
			return
		}
		encoded, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		var decoded Block
		if err = json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		after, err := decoded.Canonical()
		if err != nil || !bytes.Equal(canonical, after) || decoded.ID() != b.ID() {
			t.Fatal("block roundtrip changed canonical identity")
		}
		_ = b.Verify()
		_ = b.CheckWork(0)
	})
}
