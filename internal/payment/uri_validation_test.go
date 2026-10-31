package payment

import (
	"net/url"
	"strings"
	"testing"
)

func TestPaymentURIRejectsAmbiguityAndUnsafeText(t *testing.T) {
	base := requestFixture()
	canonical, err := base.URI()
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"", "https://host.invalid/", strings.Repeat("x", MaxURIBytes+1), canonical + "#fragment", canonical + "&unknown=x", canonical + "&amount=1", canonical + "&network=" + string(base.Network), canonical + "&label=Other", strings.Replace(canonical, "amount=25", "amount=025", 1), strings.Replace(canonical, "amount=25", "amount=0", 1), strings.Replace(canonical, "amount=25", "amount=18446744073709551616", 1), strings.Replace(canonical, "blpay:", "blpay://", 1), strings.Replace(canonical, "message=Order+%2312", "message=%00", 1), canonical + ";invalid", strings.Replace(canonical, "label=Coffee", "label="+url.QueryEscape(strings.Repeat("x", 65)), 1)} {
		if _, err := ParseURI(raw); err == nil {
			t.Fatalf("invalid URI accepted: %q", raw)
		}
	}
	for _, mutate := range []func(*Request){func(r *Request) { r.Address = "bad" }, func(r *Request) { r.Network = "bad" }, func(r *Request) { r.Amount = 0 }, func(r *Request) { r.Label = "\n" }, func(r *Request) { r.Message = string([]byte{255}) }, func(r *Request) { r.Message = strings.Repeat("x", 257) }} {
		r := base
		mutate(&r)
		if _, err := r.URI(); err == nil {
			t.Fatal("invalid request formatted")
		}
	}
}
func FuzzURI(f *testing.F) {
	raw, _ := requestFixture().URI()
	f.Add(raw)
	f.Add("")
	f.Add("blpay:bad?amount=1&network=bad")
	f.Fuzz(func(t *testing.T, input string) {
		r, err := ParseURI(input)
		if err != nil {
			return
		}
		encoded, err := r.URI()
		if err != nil || len(encoded) > MaxURIBytes {
			t.Fatal("accepted request cannot be bounded and serialized")
		}
		round, err := ParseURI(encoded)
		if err != nil || round != r {
			t.Fatal("request canonicalization changed fields")
		}
	})
}
