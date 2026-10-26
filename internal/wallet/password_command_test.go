package wallet

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestPasswordCommandUsesPrivateSecretFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wallet")
	old := []byte("old command passphrase")
	next := []byte("new command passphrase")
	w, err := Import(path, make([]byte, 32), old)
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	before := filepath.Join(dir, "old")
	after := filepath.Join(dir, "new")
	for file, data := range map[string][]byte{before: old, after: next} {
		if err := os.WriteFile(file, append(data, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var out bytes.Buffer
	if err = RunCommand(context.Background(), []string{"password", "--wallet", path, "--password-file", before, "--new-password-file", after}, &out, io.Discard); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out.Bytes(), next) || !bytes.Contains(out.Bytes(), []byte("complete")) {
		t.Fatal("unsafe password acknowledgement")
	}
	restored, err := Open(path, next)
	if err != nil {
		t.Fatal(err)
	}
	restored.Close()
}
