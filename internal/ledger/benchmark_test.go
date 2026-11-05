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

// Query workloads isolate reads from signing, fsync and fixture construction.
func benchmarkQueryStore(b *testing.B) (*Store, protocol.Account) {
	b.Helper()
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	g := protocol.Genesis{Version: 1, Name: "benchmark", Supply: 10000, Allocations: []protocol.Allocation{{Account: a, Balance: 10000, Representative: a}}}
	s, err := Open(b.TempDir(), g)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { s.Close() })
	for i := 0; i < 1000; i++ {
		account := s.state.accounts[a]
		block := protocol.Block{Version: 1, Network: g.ID(), Kind: "send", Account: a, Previous: account.Frontier, Representative: a, Balance: account.Balance - 1, Link: string(a)}
		if err := block.Sign(key); err != nil {
			b.Fatal(err)
		}
		r := Record{Height: uint64(i + 1), Previous: s.state.tip, Block: block}
		s.state.apply(r)
	}
	return s, a
}
func BenchmarkHistoryPage(b *testing.B) {
	s, a := benchmarkQueryStore(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = s.History(a, 500, 20)
	}
}
func BenchmarkReceivablePage(b *testing.B) {
	s, a := benchmarkQueryStore(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = s.Receivables(a, 500, 20)
	}
}
func BenchmarkLedgerStatus(b *testing.B) {
	s, _ := benchmarkQueryStore(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = s.Status()
	}
}
