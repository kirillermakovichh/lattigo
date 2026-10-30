package wallet

import (
	"errors"
	"io"
	"os"
	"syscall"

	"example.com/go-block-lattice/internal/payment"
)

func readPaymentFile(path string) (payment.Request, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return payment.Request{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return payment.Request{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > payment.MaxFileBytes {
		return payment.Request{}, errors.New("payment request input must be a bounded regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, payment.MaxFileBytes+1))
	if err != nil {
		return payment.Request{}, err
	}
	return payment.DecodeFile(raw)
}
func paymentInput(uri, file string) (payment.Request, bool, error) {
	if uri != "" && file != "" {
		return payment.Request{}, true, errors.New("--uri and --request-file cannot be combined")
	}
	if file != "" {
		r, err := readPaymentFile(file)
		return r, true, err
	}
	if uri != "" {
		r, err := payment.ParseURI(uri)
		return r, true, err
	}
	return payment.Request{}, false, nil
}
