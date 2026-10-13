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
