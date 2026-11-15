package payment

import (
	"testing"
)

func TestRequestDecoderFailureBoundaries(t *testing.T) {
	for _, raw := range []string{`"unterminated`, `{"unknown":1}`, `{"version":`, `{"version":true}`, `{"version":1,`, `{"version":1`, `{"version":1,"address"`, `{"version":1,"amount":`, `{"version":1,"label":`, `{"version":1,"amount":"00"}`} {
		if _, err := DecodeFile([]byte(raw)); err == nil {
			t.Fatal("malformed request accepted", raw)
		}
	}
	if _, err := EncodeFile(Request{}); err == nil {
		t.Fatal("invalid request encoded")
	}
	valid := requestFixture()
	valid.Label = ""
	valid.Message = ""
	raw, err := valid.URI()
	if err != nil {
		t.Fatal(err)
	}
	if parsed, err := ParseURI(raw); err != nil || parsed != valid {
		t.Fatal(parsed, err)
	}
	for _, raw := range []string{":bad", "blpay:address", "blpay:%zz?amount=1", "blpay:address?amount=1&network=%zz", "blpay:address?amount=1;network=x"} {
		if _, err := ParseURI(raw); err == nil {
			t.Fatal(raw)
		}
	}
}
