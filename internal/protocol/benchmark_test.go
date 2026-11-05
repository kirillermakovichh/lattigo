package protocol

import (
	"context"
	"crypto/ed25519"
	"testing"
)

func benchmarkBlock(b *testing.B) Block {
	b.Helper()
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := Address(key.Public().(ed25519.PublicKey))
	block := Block{Version: 1, Network: Digest([]byte("benchmark")), Kind: "send", Account: a, Representative: a, Balance: 99, Link: string(a)}
	if err := block.Sign(key); err != nil {
		b.Fatal(err)
	}
	return block
}
func BenchmarkBlockIdentity(b *testing.B) {
	block := benchmarkBlock(b)
	b.ReportAllocs()
	for b.Loop() {
		_ = block.ID()
	}
}
func BenchmarkBlockVerification(b *testing.B) {
	block := benchmarkBlock(b)
	b.ReportAllocs()
	for b.Loop() {
		if err := block.Verify(); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkWorkSearch(b *testing.B) {
	block := benchmarkBlock(b)
	b.ReportAllocs()
	for b.Loop() {
		if err := block.GenerateWork(context.Background(), 8); err != nil {
			b.Fatal(err)
		}
	}
}
