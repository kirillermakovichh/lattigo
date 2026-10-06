package wallet

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptedCustodyBackupAndRestore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wallet.json")
	password := []byte("correct horse battery staple")
	seed := bytes.Repeat([]byte{0x42}, 32)
	w, err := Import(path, seed, password)
	if err != nil {
		t.Fatal(err)
	}
	k0, err := w.PrivateKey(0)
	if err != nil {
		t.Fatal(err)
	}
	k1, _ := w.PrivateKey(1)
	if bytes.Equal(k0, k1) {
		t.Fatal("account derivation reused key")
	}
	w.Close()
	if _, err = w.PrivateKey(0); err == nil {
		t.Fatal("closed wallet derives keys")
	}
	data, _ := os.ReadFile(path)
	if bytes.Contains(data, seed) {
		t.Fatal("plaintext seed in envelope")
	}
	if _, err = Open(path, []byte("incorrect password")); !errors.Is(err, ErrPassword) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, err = Import(path, seed, password); err == nil {
		t.Fatal("wallet overwritten")
	}
	backup := filepath.Join(dir, "backup.json")
	restored := filepath.Join(dir, "restored.json")
	if err = Backup(path, backup, password); err != nil {
		t.Fatal(err)
	}
	if err = Restore(backup, restored, password); err != nil {
		t.Fatal(err)
	}
	w, err = Open(restored, password)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	key, _ := w.PrivateKey(0)
	if !bytes.Equal(k0, key) {
		t.Fatal("restored identity differs")
	}
	if err = os.Chmod(restored, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(restored, password); err == nil {
		t.Fatal("accepted accessible secrets")
	}
}
