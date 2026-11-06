package ledger

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
	"example.com/go-block-lattice/internal/storage"
)

func TestReplayRejectsMalformedUncertifiedAndOverBudgetRecords(t *testing.T) {
	for _, scenario := range []string{"json", "certificate", "capacity"} {
		t.Run(scenario, func(t *testing.T) {
			state, key, other := fixture(t)
			header, err := json.Marshal(state.genesis)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			journal, err := storage.Open(dir, header, func([]byte) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "json" {
				err = journal.Append([]byte("{"))
			} else if scenario == "certificate" {
				err = journal.Append([]byte(`{"height":1}`))
			} else {
				a := protocol.Address(key.Public().(ed25519.PublicKey))
				b := protocol.Address(other.Public().(ed25519.PublicKey))
				for i := 0; i < 3; i++ {
					account := state.accounts[a]
					block := protocol.Block{Version: 1, Network: state.genesis.ID(), Kind: "send", Account: a, Previous: account.Frontier, Representative: a, Balance: account.Balance - 1, Link: string(b)}
					record := nextRecord(t, state, block, key)
					raw, marshalErr := json.Marshal(record)
					if marshalErr != nil {
						t.Fatal(marshalErr)
					}
					raw = append(raw, bytes.Repeat([]byte(" "), (11<<20)-len(raw))...)
					if err = journal.Append(raw); err != nil {
						t.Fatal(err)
					}
					state.apply(record, len(raw))
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = journal.Close(); err != nil {
				t.Fatal(err)
			}
			s, err := OpenWithOptions(dir, state.genesis, Options{MaxHistoryBytes: 32 << 20})
			if s != nil {
				s.Close()
			}
			if err == nil {
				t.Fatal("invalid replay accepted")
			}
			if scenario == "capacity" && !errors.Is(err, ErrCapacity) {
				t.Fatalf("replay capacity: %v", err)
			}
		})
	}
}

func TestRecordValidationRejectsConfirmedBlockAtNewHeight(t *testing.T) {
	state, key, other := fixture(t)
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	b := protocol.Address(other.Public().(ed25519.PublicKey))
	record := nextRecord(t, state, protocol.Block{Version: 1, Network: state.genesis.ID(), Kind: "send", Account: a, Representative: a, Balance: 99, Link: string(b)}, key)
	state.apply(record)
	record.Height++
	record.Previous = state.tip
	if !errors.Is(state.validateRecord(record), ErrConflict) {
		t.Fatal("confirmed block reapplied at next height")
	}
	record.Block.Balance--
	record.Block.Signature = "bad"
	if !errors.Is(state.validateRecord(record), ErrInvalid) {
		t.Fatal("invalid transition entered record verification")
	}
}

func TestRepresentativeLimitPreservesExistingCommittee(t *testing.T) {
	state, key, _ := fixture(t)
	owner := protocol.Address(key.Public().(ed25519.PublicKey))
	state.genesis.Supply = protocol.Amount(protocol.MaxRepresentatives + 1)
	state.genesis.Allocations[0].Balance = state.genesis.Supply
	state = newState(state.genesis)
	state.accounts[owner] = AccountState{Account: owner, Representative: owner, Balance: 1}
	state.owned[owner] = 1
	state.delegated[owner] = 1
	// A conserved snapshot attainable by opening funded accounts. One account
	// retains delegation to the owner, so changing the owner splits its weight.
	for i := 0; i < protocol.MaxRepresentatives; i++ {
		var seed [32]byte
		binary.BigEndian.PutUint64(seed[:], uint64(i+1000))
		account := protocol.Address(ed25519.NewKeyFromSeed(seed[:]).Public().(ed25519.PublicKey))
		rep := account
		if i == 0 {
			rep = owner
		}
		state.accounts[account] = AccountState{Account: account, Representative: rep, Balance: 1}
		state.owned[account] = 1
		state.delegated[rep]++
	}
	var seed [32]byte
	seed[0] = 255
	newRep := protocol.Address(ed25519.NewKeyFromSeed(seed[:]).Public().(ed25519.PublicKey))
	block := protocol.Block{Version: 1, Network: state.genesis.ID(), Kind: "change", Account: owner, Representative: newRep, Balance: 1}
	if err := block.Sign(key); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(state.validate(block), ErrInvalid) {
		t.Fatal("representative committee exceeded capacity")
	}
	if len(state.weights()) != protocol.MaxRepresentatives {
		t.Fatal("rejected delegation mutated committee")
	}
}

func TestRestoredObserverCannotPublishSigningState(t *testing.T) {
	state, _, _ := fixture(t)
	s, err := Open(t.TempDir(), state.genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var archive bytes.Buffer
	if err = s.Backup(&archive); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "observer")
	if err = Restore(dir, bytes.NewReader(archive.Bytes()), state.genesis); err != nil {
		t.Fatal(err)
	}
	observer, err := Open(dir, state.genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close()
	if !errors.Is(observer.SaveSigningState([]byte("intent")), ErrObserverOnly) {
		t.Fatal("restored observer signed")
	}
	if err = RestoreWithOptions(filepath.Join(t.TempDir(), "bad"), bytes.NewReader(nil), state.genesis, Options{MaxHistoryBytes: 1}); err == nil {
		t.Fatal("restore accepted invalid capacity")
	}
	if err = Restore(filepath.Join(t.TempDir(), "bad"), bytes.NewReader(nil), protocol.Genesis{}); err == nil {
		t.Fatal("restore accepted invalid genesis")
	}
}

type duringRestoreReader struct {
	reader  *bytes.Reader
	before  func()
	started bool
}

func (r *duringRestoreReader) Read(p []byte) (int, error) {
	if !r.started {
		r.started = true
		r.before()
	}
	return r.reader.Read(p)
}
func TestRestoreRetainsFilesCreatedWhileBackupIsValidated(t *testing.T) {
	state, _, _ := fixture(t)
	s, err := Open(t.TempDir(), state.genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var archive bytes.Buffer
	if err = s.Backup(&archive); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target")
	concurrent := filepath.Join(target, "operator-file")
	reader := &duringRestoreReader{reader: bytes.NewReader(archive.Bytes()), before: func() {
		if err := os.Mkdir(target, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(concurrent, []byte("retain"), 0600); err != nil {
			t.Fatal(err)
		}
	}}
	if err = Restore(target, reader, state.genesis); err == nil {
		t.Fatal("restore overwrote a concurrently populated target")
	}
	raw, err := os.ReadFile(concurrent)
	if err != nil || string(raw) != "retain" {
		t.Fatal("concurrent operator data lost")
	}
}
