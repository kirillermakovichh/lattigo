package ledger

import (
	"crypto/ed25519"
	"example.com/go-block-lattice/internal/protocol"
	"testing"
)

func BenchmarkCertificateVerification(b *testing.B) {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	network := protocol.Digest([]byte("benchmark"))
	block := protocol.Block{Version: 1, Network: network, Kind: "send", Account: a, Representative: a, Balance: 99, Link: string(a)}
	if err := block.Sign(key); err != nil {
		b.Fatal(err)
	}
	r := Record{Height: 1, Previous: network, Block: block}
	v := Vote{Network: network, Height: 1, Phase: "precommit", Parent: network, Block: block.ID(), Representative: a}
	if err := v.Sign(key); err != nil {
		b.Fatal(err)
	}
	r.Votes = []Vote{v}
	weights := map[protocol.Account]protocol.Amount{a: 100}
	b.ReportAllocs()
	for b.Loop() {
		if err := certificate(r, network, weights, 100); err != nil {
			b.Fatal(err)
		}
	}
}
