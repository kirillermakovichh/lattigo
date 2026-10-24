//go:build faultaudit

package control

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"example.com/go-block-lattice/internal/protocol"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdministrationIOFailures(t *testing.T) {
	for _, point := range _auditPoints {
		if point.Function != "writeNew" && point.Function != "controlSecret" {
			continue
		}
		t.Run(point.ID, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "token")
			if err := os.WriteFile(path, []byte(strings.Repeat("a", 32)), 0600); err != nil {
				t.Fatal(err)
			}
			_auditArm(point.ID, 1)
			defer _auditArm("", 0)
			var err error
			if point.Function == "writeNew" {
				err = writeNew(filepath.Join(dir, "out"), func(w io.Writer) error { _, err := w.Write([]byte("output")); return err })
			} else {
				_, err = controlSecret(path)
			}
			if !_auditReached() {
				t.Fatal("fault site not reached")
			}
			if !errors.Is(err, _auditInjected) {
				t.Fatalf("I/O error lost: %v", err)
			}
			leftovers, _ := filepath.Glob(filepath.Join(dir, ".bl-output-*"))
			if len(leftovers) != 0 {
				t.Fatal("temporary output leaked")
			}
		})
	}
}
func TestAllocationDescriptorFailures(t *testing.T) {
	for _, point := range _auditPoints {
		if point.Function != "RunControl" || (!strings.Contains(point.ID, "f.Stat") && !strings.Contains(point.ID, "io.ReadAll") && !strings.Contains(point.ID, "os.OpenFile/")) {
			continue
		}
		t.Run(point.ID, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "allocations")
			if err := os.WriteFile(path, []byte("[]"), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"genesis", "--allocations", path}
			if strings.Contains(point.ID, "f.Stat/2") || strings.Contains(point.ID, "os.OpenFile/2") {
				key := ed25519.NewKeyFromSeed(make([]byte, 32))
				a := protocol.Address(key.Public().(ed25519.PublicKey))
				g := protocol.Genesis{Version: 1, Name: "restore-fault", Supply: 1, Allocations: []protocol.Allocation{{Account: a, Balance: 1, Representative: a}}}
				raw, err := json.Marshal(g)
				if err != nil {
					t.Fatal(err)
				}
				manifest := filepath.Join(t.TempDir(), "genesis.json")
				if err = os.WriteFile(manifest, raw, 0600); err != nil {
					t.Fatal(err)
				}
				args = []string{"restore", "--genesis", manifest, "--backup", path, "--data-dir", filepath.Join(t.TempDir(), "data")}
			}
			_auditArm(point.ID, 1)
			defer _auditArm("", 0)
			err := RunControl(context.Background(), args, io.Discard, io.Discard)
			if !_auditReached() {
				t.Fatal("fault site not reached")
			}
			if !errors.Is(err, _auditInjected) {
				t.Fatalf("allocation descriptor error: %v", err)
			}
		})
	}
}
