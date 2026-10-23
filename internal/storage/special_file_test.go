package storage

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestSigningRecoveryRejectsFIFOWithoutWriter(t *testing.T) {
	dir := t.TempDir()
	j, err := Open(dir, []byte("genesis"), func([]byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	path := filepath.Join(dir, "signing.json")
	if err = syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := j.Signing(); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO accepted as signing state")
		}
	case <-time.After(time.Second):
		writer, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err != nil {
			t.Fatal(err)
		}
		writer.Close()
		<-done
		t.Fatal("signing recovery waited for a FIFO writer")
	}
}

func TestJournalRejectsSpecialLockBeforeCreatingWAL(t *testing.T) {
	dir := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(dir, "LOCK"), 0600); err != nil {
		t.Fatal(err)
	}
	journal, err := Open(dir, []byte("genesis"), func([]byte) error { return nil })
	if err == nil {
		journal.Close()
		t.Fatal("FIFO accepted as directory lock")
	}
	if _, err := os.Stat(filepath.Join(dir, "ledger.wal")); !os.IsNotExist(err) {
		t.Fatal("invalid lock created WAL")
	}
}
