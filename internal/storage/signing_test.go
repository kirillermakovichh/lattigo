package storage

import (
	"bytes"
	"testing"
)

func TestSigningStateHasIndependentTwoCertificateBudget(t *testing.T) {
	journal, err := Open(t.TempDir(), []byte("test-genesis"), func([]byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	// A legitimate signing state may exceed a single WAL record because both
	// the latest valid proof and the durable own proposal retain certificates.
	payload := bytes.Repeat([]byte{'x'}, MaxFrame+1)
	if err = journal.SaveSigning(payload); err != nil {
		t.Fatal(err)
	}
	restored, err := journal.Signing()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored, payload) {
		t.Fatal("large signing state changed")
	}
	if err = journal.SaveSigning(make([]byte, MaxSigningState+1)); err == nil {
		t.Fatal("unbounded signing state accepted")
	}
}

func TestJournalIOFailureLatchesAcrossWritesAndBackup(t *testing.T) {
	journal, err := Open(t.TempDir(), []byte("failure-test-genesis"), func([]byte) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	// A revoked file descriptor deterministically exercises the write-error path
	// without relying on filesystem permissions or filling the host disk.
	if err = journal.file.Close(); err != nil {
		t.Fatal(err)
	}
	first := journal.Append([]byte("must not be acknowledged"))
	if first == nil {
		t.Fatal("failed write reported success")
	}
	if err = journal.Append([]byte("retry")); err != first {
		t.Fatal("failed journal resumed writes")
	}
	if err = journal.SaveSigning([]byte("vote")); err != first {
		t.Fatal("failed journal allowed signing")
	}
	if err = journal.Backup(&bytes.Buffer{}); err != first {
		t.Fatal("failed journal produced backup")
	}
}
