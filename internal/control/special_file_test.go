package control

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestAdministrativeInputRejectsFIFOWithoutWriter(t *testing.T) {
	for _, kind := range []string{"token", "allocations"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "input.fifo")
			if err := syscall.Mkfifo(path, 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"config", "--token-file", path, "--out", filepath.Join(dir, "config.json")}
			if kind == "allocations" {
				args = []string{"genesis", "--name", "fifo-test", "--allocations", path, "--out", filepath.Join(dir, "genesis.json")}
			}
			done := make(chan error, 1)
			go func() { done <- RunControl(context.Background(), args, io.Discard, io.Discard) }()
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
				t.Fatal("administration input waited for writer")
			}
		})
	}
}
