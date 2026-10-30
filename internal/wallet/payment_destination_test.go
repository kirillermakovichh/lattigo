package wallet

import (
	"example.com/go-block-lattice/internal/payment"
	"testing"
)

func TestPaymentRequestDestinationPinsExactAmountAndNetwork(t *testing.T) {
	_, c := profileFixture(t)
	r := payment.Request{Address: c.Address, Network: c.Network, Amount: 42}
	var f commandFlags
	to, amount, err := paymentDestination(&f, r, "", "", "")
	if err != nil || to != string(c.Address) || amount != "42" || f.expectedNetwork != string(c.Network) {
		t.Fatal("payment request changed transfer fields")
	}
	for _, explicit := range [][3]string{{string(c.Address), "", ""}, {"", "42", ""}, {"", "", "Alice"}} {
		if _, _, err := paymentDestination(&f, r, explicit[0], explicit[1], explicit[2]); err == nil {
			t.Fatal("conflicting payment parameters accepted")
		}
	}
	f.expectedNetwork = "other"
	if _, _, err := paymentDestination(&f, r, "", "", ""); err == nil {
		t.Fatal("payment request ignored explicit network pin")
	}
}
