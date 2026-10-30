package payment

import (
	"crypto/ed25519"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
)

func requestFixture() Request {
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	return Request{Address: protocol.Address(key.Public().(ed25519.PublicKey)), Network: protocol.Digest([]byte("payment-uri")), Amount: 25, Label: "Coffee", Message: "Order #12"}
}
func TestPaymentURIRoundTripUsesExactAmounts(t *testing.T) {
	original := requestFixture()
	encoded, err := original.URI()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseURI(encoded)
	if err != nil || got != original {
		t.Fatalf("request round trip: %+v %v", got, err)
	}
	canonical, err := got.URI()
	if err != nil || canonical != encoded || len(canonical) > MaxURIBytes {
		t.Fatal("request serialization is not stable and bounded")
	}
}
