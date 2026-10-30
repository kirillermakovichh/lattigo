// Package payment validates off-chain requests without changing signed ledger formats.
package payment

import (
	"errors"
	"net/url"
	"unicode"
	"unicode/utf8"

	"example.com/go-block-lattice/internal/protocol"
)

const MaxURIBytes = 2048

type Request struct {
	Address protocol.Account `json:"address"`
	Network protocol.Hash    `json:"network"`
	Amount  protocol.Amount  `json:"amount"`
	Label   string           `json:"label,omitempty"`
	Message string           `json:"message,omitempty"`
}

func validText(s string, limit int) bool {
	if len(s) > limit || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func (r Request) Validate() error {
	if _, err := r.Address.PublicKey(); err != nil {
		return err
	}
	if _, err := r.Network.Bytes(); err != nil {
		return err
	}
	if r.Amount == 0 {
		return errors.New("payment amount must be positive")
	}
	if !validText(r.Label, 64) || !validText(r.Message, 256) {
		return errors.New("payment label or message is invalid or oversized")
	}
	return nil
}
func (r Request) URI() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	query := url.Values{"network": {string(r.Network)}, "amount": {r.Amount.String()}}
	if r.Label != "" {
		query.Set("label", r.Label)
	}
	if r.Message != "" {
		query.Set("message", r.Message)
	}
	return "blpay:" + string(r.Address) + "?" + query.Encode(), nil
}
func ParseURI(raw string) (Request, error) {
	var r Request
	if len(raw) == 0 || len(raw) > MaxURIBytes {
		return r, errors.New("payment URI exceeds input budget")
	}
	uri, err := url.Parse(raw)
	if err != nil {
		return r, err
	}
	if uri.Scheme != "blpay" || uri.Opaque == "" || uri.Host != "" || uri.User != nil || uri.Path != "" || uri.Fragment != "" {
		return r, errors.New("payment URI requires blpay:ADDRESS and no authority or fragment")
	}
	query, err := url.ParseQuery(uri.RawQuery)
	if err != nil {
		return r, err
	}
	for key, values := range query {
		if (key != "network" && key != "amount" && key != "label" && key != "message") || len(values) != 1 {
			return r, errors.New("unknown or repeated payment URI field")
		}
	}
	amount, err := protocol.ParseAmount(query.Get("amount"))
	if err != nil {
		return r, err
	}
	r = Request{Address: protocol.Account(uri.Opaque), Network: protocol.Hash(query.Get("network")), Amount: amount, Label: query.Get("label"), Message: query.Get("message")}
	return r, r.Validate()
}
