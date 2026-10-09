package ledger

import (
	"bytes"
	"crypto/ed25519"
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
