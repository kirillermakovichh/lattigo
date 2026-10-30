package wallet

import (
	"errors"
	"example.com/go-block-lattice/internal/payment"
)

func paymentDestination(f *commandFlags, r payment.Request, to, amount, contact string) (string, string, error) {
	if to != "" || amount != "" || contact != "" {
		return "", "", errors.New("payment request cannot be combined with --to, --amount or --contact")
	}
	if err := r.Validate(); err != nil {
		return "", "", err
	}
	if f.expectedNetwork != "" && f.expectedNetwork != string(r.Network) {
		return "", "", errors.New("payment request differs from --expected-network")
	}
	f.expectedNetwork = string(r.Network)
	return string(r.Address), r.Amount.String(), nil
}
