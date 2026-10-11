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

func TestDurableStoreRecoveryAndBackup(t *testing.T) {
	state, key, other := fixture(t)
	g := state.genesis
	directory := t.TempDir()
	store, e := Open(directory, g)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := Open(directory, g); e == nil {
		t.Fatal("second writer admitted")
	}
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	b := protocol.Address(other.Public().(ed25519.PublicKey))
	r := nextRecord(t, state, protocol.Block{Version: 1, Network: g.ID(), Kind: "send", Account: a, Representative: a, Balance: 70, Link: string(b)}, key)
	if e = store.Commit(r); e != nil {
		t.Fatal(e)
	}
	if e = store.SaveSigningState([]byte(`{"height":2}`)); e != nil {
		t.Fatal(e)
	}
	var backup bytes.Buffer
	if e = store.Backup(&backup); e != nil {
		t.Fatal(e)
	}
	if e = store.Close(); e != nil {
		t.Fatal(e)
	}
	store, e = Open(directory, g)
	if e != nil {
		t.Fatal(e)
	}
	if store.Status().Height != 1 || len(store.Receivables(b, 0, 100)) != 1 {
		t.Fatal("recovery lost transition")
	}
	signing, e := store.SigningState()
	if e != nil || string(signing) != `{"height":2}` {
		t.Fatal("signing recovery")
	}
	store.Close()
	target := filepath.Join(t.TempDir(), "restored")
	if e = Restore(target, bytes.NewReader(backup.Bytes()), g); e != nil {
		t.Fatal(e)
	}
	restored, e := Open(target, g)
	if e != nil {
		t.Fatal(e)
	}
	defer restored.Close()
	if restored.Status().Tip != r.ID() {
		t.Fatal("restored tip")
	}
	if _, e = restored.SigningState(); e != ErrObserverOnly {
		t.Fatal("signing material in ledger backup")
	}
	if e = Restore(target, bytes.NewReader(backup.Bytes()), g); e == nil {
		t.Fatal("restore overwrote directory")
	}
}
func TestTornTailRecoveryAndCorruption(t *testing.T) {
	state, key, other := fixture(t)
	g := state.genesis
	directory := t.TempDir()
	store, e := Open(directory, g)
	if e != nil {
		t.Fatal(e)
	}
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	b := protocol.Address(other.Public().(ed25519.PublicKey))
	r := nextRecord(t, state, protocol.Block{Version: 1, Network: g.ID(), Kind: "send", Account: a, Representative: a, Balance: 99, Link: string(b)}, key)
	if e = store.Commit(r); e != nil {
		t.Fatal(e)
	}
	store.Close()
	path := filepath.Join(directory, "ledger.wal")
	good, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	f, e := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	f.Write([]byte{0, 0, 0})
	f.Close()
	store, e = Open(directory, g)
	if e != nil {
		t.Fatal(e)
	}
	if store.Status().Height != 1 {
		t.Fatal("torn tail lost valid record")
	}
	store.Close()
	corrupt := append([]byte(nil), good...)
	corrupt[len(corrupt)-10] ^= 1
	if e = os.WriteFile(path, corrupt, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = Open(directory, g); e == nil {
		t.Fatal("checksum corruption accepted")
	}
	if e = os.WriteFile(path, nil, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = Open(directory, g); e == nil {
		t.Fatal("empty existing WAL silently reset")
	}
	target := filepath.Join(t.TempDir(), "partial")
	if e = Restore(target, bytes.NewReader(append(good, 1, 2)), g); e == nil {
		t.Fatal("partial backup accepted")
	}
}

func TestSigningStateLossAndCorruptionFailClosed(t *testing.T) {
	state, _, _ := fixture(t)
	dir := t.TempDir()
	store, err := Open(dir, state.genesis)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SaveSigningState([]byte(`{"round":3}`)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "signing.json")
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SigningState(); err == nil || err == ErrNotFound {
		t.Fatal("lost initialized signing state treated as fresh signer")
	}
	if err = store.SaveSigningState([]byte(`{"round":3}`)); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	b[len(b)-1] ^= 1
	if err = os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SigningState(); err == nil {
		t.Fatal("signing state corruption accepted")
	}
	store.Close()
}

func TestConfiguredHistoryCapacityStopsAdmission(t *testing.T) {
	state, key, other := fixture(t)
	store, err := OpenWithOptions(t.TempDir(), state.genesis, Options{MaxHistoryBytes: 32 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	b := protocol.Address(other.Public().(ed25519.PublicKey))
	block := nextRecord(t, state, protocol.Block{Version: 1, Network: state.genesis.ID(), Kind: "send", Account: a, Representative: a, Balance: 99, Link: string(b)}, key).Block
	store.historyBytes = store.historyLimit - (16 << 20) + 1
	if err = store.Validate(block); !errors.Is(err, ErrCapacity) {
		t.Fatalf("capacity admission: %v", err)
	}
	if !store.Status().CapacityReached {
		t.Fatal("capacity not exposed")
	}
	if _, err = OpenWithOptions(t.TempDir(), state.genesis, Options{MaxHistoryBytes: 1}); err == nil {
		t.Fatal("impossible capacity accepted")
	}
}

func TestHistoryAndBootstrapPagesBoundCopiedBytes(t *testing.T) {
	state, key, other := fixture(t)
	store, err := Open(t.TempDir(), state.genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	b := protocol.Address(other.Public().(ed25519.PublicKey))
	for i := 0; i < 3; i++ {
		account, _ := store.Account(a)
		block := protocol.Block{Version: 1, Network: state.genesis.ID(), Kind: "send", Account: a, Previous: account.Frontier, Representative: a, Balance: account.Balance - 1, Link: string(b)}
		r := nextRecord(t, store.state, block, key)
		if err = store.Commit(r); err != nil {
			t.Fatal(err)
		}
	}
	// Inject size metadata to exercise budgets without allocating large certificates.
	for i := range store.state.recordSizes {
		store.state.recordSizes[i] = 10 << 20
	}
	if got := len(store.History(a, 0, 1000)); got != 1 {
		t.Fatalf("history copied %d oversized records", got)
	}
	if got := len(store.Records(0, 1000)); got != 2 {
		t.Fatalf("bootstrap copied %d oversized records", got)
	}
	if got := len(store.History(a, 2, 1000)); got != 1 {
		t.Fatal("history offset lost")
	}
}
