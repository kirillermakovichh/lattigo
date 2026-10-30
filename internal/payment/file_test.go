package payment

import (
	"bytes"
	"strings"
	"testing"
)

func TestRequestFileRoundTripRejectsAmbiguousFields(t *testing.T) {
	original := requestFixture()
	raw, err := EncodeFile(original)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeFile(raw)
	if err != nil || got != original {
		t.Fatalf("request file round trip: %v", err)
	}
	duplicate := bytes.Replace(raw, []byte(`"version": 1`), []byte(`"version": 1, "version": 1`), 1)
	for _, data := range [][]byte{nil, []byte("[]"), []byte("{}"), []byte("{"), duplicate, append(append([]byte{}, raw...), []byte("{}")...), []byte(strings.Repeat(" ", MaxFileBytes+1)), bytes.Replace(raw, []byte(`"label": "Coffee"`), []byte(`"label": null`), 1)} {
		if _, err := DecodeFile(data); err == nil {
			t.Fatal("malformed or ambiguous request file accepted")
		}
	}
}
