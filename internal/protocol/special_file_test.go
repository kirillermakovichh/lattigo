package protocol

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestGenesisRejectsFIFOWithoutWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "genesis.fifo")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := LoadGenesis(path); done <- err }()
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
		t.Fatal("genesis input waited for writer")
	}
}
