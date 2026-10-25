//go:build faultaudit

package protocol

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenesisIOFailures(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := Address(key.Public().(ed25519.PublicKey))
	g := Genesis{Version: 1, Name: "fault-input", Supply: 100, Allocations: []Allocation{{Account: a, Balance: 100, Representative: a}}}
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range _auditPoints {
		if point.Function != "LoadGenesis" {
			continue
		}
		t.Run(point.ID, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "genesis.json")
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			_auditArm(point.ID, 1)
			defer _auditArm("", 0)
			_, err := LoadGenesis(path)
			if !_auditReached() {
				t.Fatal("fault site not reached")
			}
			if !errors.Is(err, _auditInjected) {
				t.Fatalf("genesis I/O error lost: %v", err)
			}
		})
	}
}

func TestWorkSearchRefusesToWrapFinalNonce(t *testing.T) {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := Address(key.Public().(ed25519.PublicKey))
	b := Block{Version: 1, Network: Digest([]byte("nonce-boundary")), Kind: "send", Account: a, Representative: a, Balance: 1, Link: string(a), Work: 123}
	if err := b.Sign(key); err != nil {
		t.Fatal(err)
	}
	last := b
	last.Work = math.MaxUint64
	if last.CheckWork(24) {
		t.Fatal("boundary fixture unexpectedly satisfies work")
	}
	// The isolated copy starts at the final nonce; the production search still
	// starts at zero. This checks exhaustion without enumerating 2^64 candidates.
	_auditArm("block.go/GenerateWork/nonce-start", 1)
	defer _auditArm("", 0)
	err := b.GenerateWork(context.Background(), 24)
	if !_auditReached() || err == nil || !strings.Contains(err.Error(), "exhausted") || b.Work != 123 {
		t.Fatalf("nonce exhaustion wrapped or modified work: %v", err)
	}
}
