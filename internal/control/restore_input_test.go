package control

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"example.com/go-block-lattice/internal/protocol"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestRestoreRejectsFIFOWithoutWriter(t *testing.T) {
	dir := t.TempDir()
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	a := protocol.Address(key.Public().(ed25519.PublicKey))
	g := protocol.Genesis{Version: 1, Name: "restore-input", Supply: 100, Allocations: []protocol.Allocation{{Account: a, Balance: 100, Representative: a}}}
	raw, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "genesis.json")
	if err = os.WriteFile(manifest, raw, 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "backup.fifo")
	if err = syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- RunControl(context.Background(), []string{"restore", "--genesis", manifest, "--backup", path, "--data-dir", filepath.Join(dir, "restored")}, io.Discard, &bytes.Buffer{})
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO accepted")
		}
	case <-time.After(time.Second):
		writer, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			t.Fatal(err)
		}
		writer.Close()
		<-done
		t.Fatal("restore waited for a FIFO writer")
	}
}
