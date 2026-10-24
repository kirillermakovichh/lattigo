//go:build faultaudit

package protocol

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
