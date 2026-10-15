package storage

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type failedReader struct{ err error }

func (r failedReader) Read([]byte) (int, error) { return 0, r.err }

func TestJournalFramePreservesUnderlyingReadErrors(t *testing.T) {
	payload := []byte("confirmed journal payload")
	encoded, err := frame(payload)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("injected disk read error")
	for _, offset := range []int{0, 10, 40, 45} {
		reader := io.MultiReader(bytes.NewReader(encoded[:offset]), failedReader{failure})
		_, err = readFrame(reader)
		if !errors.Is(err, failure) {
			t.Errorf("offset%d: disk error changed to %v", offset, err)
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			t.Errorf("offset%d: disk error permits torn-tail repair", offset)
		}
	}
}
