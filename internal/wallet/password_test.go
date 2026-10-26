package wallet

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPasswordRotationPreservesIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wallet")
	old := []byte("old private passphrase")
	next := []byte("new private passphrase")
	w, err := Import(path, bytes.Repeat([]byte{7}, 32), old)
	if err != nil {
		t.Fatal(err)
	}
	key, err := w.PrivateKey(19)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	w.Close()
	if err = ChangePassword(path, old, next); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(path, old); err == nil {
		t.Fatal("old password still accepted")
	}
	restored, err := Open(path, next)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	got, err := restored.PrivateKey(19)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(got)
	if !bytes.Equal(got, key) {
		t.Fatal("rotation changed account")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("replacement is not private")
	}
}
