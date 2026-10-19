package storage

import (
	"bytes"
	"testing"
)

func TestClosedJournalCannotAccessActiveSigningState(t *testing.T) {
	directory := t.TempDir()
	old, err := Open(directory, []byte("genesis"), func([]byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err = old.SaveSigning([]byte("old")); err != nil {
		t.Fatal(err)
	}
	if err = old.Close(); err != nil {
		t.Fatal(err)
	}
	active, err := Open(directory, []byte("genesis"), func([]byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer active.Close()
	if err = active.SaveSigning([]byte("active")); err != nil {
		t.Fatal(err)
	}
	if err = old.SaveSigning([]byte("stale")); err == nil {
		t.Error("closed journal changed active signing state")
	}
	if _, err = old.Signing(); err == nil {
		t.Error("closed journal retained signing access")
	}
	data, err := active.Signing()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, []byte("active")) {
		t.Fatal("active signing state overwritten after lock release")
	}
}
