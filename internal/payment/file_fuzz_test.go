package payment

import (
	"bytes"
	"testing"
)

func FuzzRequestFile(f *testing.F) {
	valid, err := EncodeFile(requestFixture())
	if err != nil {
		f.Fatal(err)
	}
	for _, seed := range [][]byte{valid, {}, []byte("null"), []byte(`{"version":1,"version":1}`), bytes.Repeat([]byte("x"), MaxFileBytes+1), {0xff}, append(append([]byte{}, valid...), []byte(" {}")...)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		request, err := DecodeFile(raw)
		if err != nil {
			return
		}
		if len(raw) > MaxFileBytes {
			t.Fatal("oversized request accepted")
		}
		canonical, err := EncodeFile(request)
		if err != nil || len(canonical) > MaxFileBytes {
			t.Fatal("accepted request cannot be bounded and encoded")
		}
		again, err := DecodeFile(canonical)
		if err != nil || again != request {
			t.Fatal("request changed on canonical round trip")
		}
		stable, err := EncodeFile(again)
		if err != nil || !bytes.Equal(stable, canonical) {
			t.Fatal("request encoding is not stable")
		}
	})
}
