package storage

import (
	"bytes"
	"testing"
)

func FuzzJournalFrames(f *testing.F) {
	for _, payload := range [][]byte{[]byte("ledger"), []byte(`{"height":1}`)} {
		encoded, err := frame(payload)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(encoded)
	}
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 8192 {
			t.Skip()
		}
		reader := bytes.NewReader(data)
		payload, err := readFrame(reader)
		if err != nil {
			return
		}
		encoded, err := frame(payload)
		if err != nil {
			t.Fatal(err)
		}
		consumed := len(data) - reader.Len()
		if !bytes.Equal(encoded, data[:consumed]) {
			t.Fatal("accepted frame is not canonical")
		}
	})
}
