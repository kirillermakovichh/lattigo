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
	for _, kind := range []string{"fifo", "directory", "symlink"} {
		for _, strict := range []bool{false, true} {
			t.Run(kind+map[bool]string{false: "/open", true: "/strict"}[strict], func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, "LOCK")
				var err error
				switch kind {
				case "fifo":
					err = syscall.Mkfifo(path, 0600)
				case "directory":
					err = os.Mkdir(path, 0700)
				case "symlink":
					err = os.Symlink(filepath.Join(t.TempDir(), "missing"), path)
				}
				if err != nil {
					t.Fatal(err)
				}
				open := Open
				if strict {
					open = OpenStrict
				}
				journal, err := open(dir, []byte("genesis"), func([]byte) error { return nil })
				if err == nil {
					journal.Close()
					t.Fatal("special file accepted as directory lock")
				}
				if _, err := os.Stat(filepath.Join(dir, "ledger.wal")); !os.IsNotExist(err) {
					t.Fatal("invalid lock created WAL")
				}
			})
		}
	}
}
