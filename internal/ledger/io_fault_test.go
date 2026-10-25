//go:build faultaudit

package ledger

import (
	"bytes"
	"crypto/ed25519"
	"errors"
	"example.com/go-block-lattice/internal/protocol"
	"os"
	"path/filepath"
	"testing"
)

func TestLedgerOpenAndRestoreIOFailures(t *testing.T) {
	state, _, _ := fixture(t)
	original, err := Open(t.TempDir(), state.genesis)
	if err != nil {
		t.Fatal(err)
	}
	var archive bytes.Buffer
	if err = original.Backup(&archive); err != nil {
		t.Fatal(err)
	}
	if err = original.Close(); err != nil {
		t.Fatal(err)
	}
	for _, point := range _auditPoints {
		if point.Function != "open" && point.Function != "OpenExisting" && point.Function != "RestoreWithOptions" {
			continue
		}
		t.Run(point.ID, func(t *testing.T) {
			parent := t.TempDir()
			dir := filepath.Join(parent, "data")
			if point.Function == "OpenExisting" {
				s, err := Open(dir, state.genesis)
				if err != nil {
					t.Fatal(err)
				}
				if err = s.Close(); err != nil {
					t.Fatal(err)
				}
			}
			_auditArm(point.ID, 1)
			defer _auditArm("", 0)
			var err error
			switch point.Function {
			case "open":
				var s *Store
				s, err = Open(dir, state.genesis)
				if s != nil {
					s.Close()
				}
			case "OpenExisting":
				var s *Store
				s, err = OpenExisting(dir, state.genesis, Options{})
				if s != nil {
					s.Close()
				}
			case "RestoreWithOptions":
				err = Restore(dir, bytes.NewReader(archive.Bytes()), state.genesis)
			}
			if !_auditReached() {
				t.Fatal("fault site not reached")
			}
			if !errors.Is(err, _auditInjected) {
				t.Fatalf("ledger I/O error lost: %v", err)
			}
			stages, _ := filepath.Glob(filepath.Join(parent, ".restore-*"))
			if len(stages) != 0 {
				t.Fatal("restore staging directory leaked")
			}
			_auditArm("", 0)
			// A failed restore must never grant historical signing authority.
			if s, err := OpenExisting(dir, state.genesis, Options{}); err == nil {
				defer s.Close()
				if _, err = os.Stat(filepath.Join(dir, "RESTORED")); err == nil && !errors.Is(s.SaveSigningState([]byte("intent")), ErrObserverOnly) {
					t.Fatal("failed restore granted signer authority")
				}
			}
		})
	}
}

func TestLedgerPropagatesJournalAndRestoreDependencyFailures(t *testing.T) {
	state, key, other := fixture(t)
	a := state.genesis.Allocations[0].Account
	b := protocol.Address(other.Public().(ed25519.PublicKey))
	r := nextRecord(t, state, protocol.Block{Version: 1, Network: state.genesis.ID(), Kind: "send", Account: a, Representative: a, Balance: 90, Link: string(b)}, key)
	for _, operation := range []string{"commit", "restore-marker", "restore-close"} {
		t.Run(operation, func(t *testing.T) {
			store, err := Open(t.TempDir(), state.genesis)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			var archive bytes.Buffer
			if err = store.Backup(&archive); err != nil {
				t.Fatal(err)
			}
			site := "journal.go/Append/j.file.Write/1"
			if operation == "restore-marker" {
				site = "journal.go/AtomicWrite/os.Rename/1"
			}
			if operation == "restore-close" {
				site = "journal.go/Close/j.file.Close/1"
			}
			_auditArm(site, 1)
			defer _auditArm("", 0)
			if operation == "commit" {
				err = store.Commit(r)
			} else {
				err = Restore(filepath.Join(t.TempDir(), "restored"), bytes.NewReader(archive.Bytes()), state.genesis)
			}
			if !_auditReached() || !errors.Is(err, _auditInjected) {
				t.Fatalf("dependency error lost: %v", err)
			}
			if operation == "commit" && store.Status().Height != 0 {
				t.Fatal("failed append changed confirmed state")
			}
		})
	}
}

func TestLedgerEncodingFailuresLeaveStateUnchanged(t *testing.T) {
	for _, scenario := range []string{"genesis", "record"} {
		t.Run(scenario, func(t *testing.T) {
			state, key, other := fixture(t)
			s, err := Open(t.TempDir(), state.genesis)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			a := protocol.Address(key.Public().(ed25519.PublicKey))
			b := protocol.Address(other.Public().(ed25519.PublicKey))
			r := nextRecord(t, state, protocol.Block{Version: 1, Network: state.genesis.ID(), Kind: "send", Account: a, Representative: a, Balance: 99, Link: string(b)}, key)
			if scenario == "genesis" {
				_auditArm("store.go/open/json.Marshal/1", 1)
				_, err = Open(t.TempDir(), state.genesis)
			} else {
				_auditArm("store.go/Commit/json.Marshal/1", 1)
				err = s.Commit(r)
			}
			defer _auditArm("", 0)
			if !_auditReached() || !errors.Is(err, _auditInjected) {
				t.Fatalf("ledger encoding error: %v", err)
			}
			if s.Status().Height != 0 {
				t.Fatal("failed encoding advanced ledger")
			}
		})
	}
}
