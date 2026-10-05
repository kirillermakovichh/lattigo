// Package protocol defines the canonical, network-separated payment format.
package protocol

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Hash string
type Account string
type Amount uint64

func ParseAmount(s string) (Amount, error) {
	if s == "" || (len(s) > 1 && s[0] == '0') {
		return 0, errors.New("amount must be canonical unsigned decimal")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("amount must be unsigned decimal")
		}
	}
	n, e := strconv.ParseUint(s, 10, 64)
	if e != nil {
		return 0, fmt.Errorf("amount: %w", e)
	}
	return Amount(n), nil
}
func (a Amount) String() string               { return strconv.FormatUint(uint64(a), 10) }
func (a Amount) MarshalJSON() ([]byte, error) { return json.Marshal(a.String()) }
func (a *Amount) UnmarshalJSON(b []byte) error {
	var s string
	if e := json.Unmarshal(b, &s); e != nil {
		return errors.New("amount JSON must be a decimal string")
	}
	n, e := ParseAmount(s)
	if e != nil {
		return e
	}
	*a = n
	return nil
}
func Address(pub ed25519.PublicKey) Account {
	if len(pub) != ed25519.PublicKeySize {
		return ""
	}
	h := sha256.Sum256(append([]byte("BL/address/v1"), pub...))
	return Account("bl_" + hex.EncodeToString(pub) + hex.EncodeToString(h[:4]))
}
func (a Account) PublicKey() (ed25519.PublicKey, error) {
	s := string(a)
	if len(s) != 75 || !strings.HasPrefix(s, "bl_") || s != strings.ToLower(s) {
		return nil, errors.New("invalid address format")
	}
	b, e := hex.DecodeString(s[3:67])
	if e != nil {
		return nil, errors.New("invalid address public key")
	}
	if Address(ed25519.PublicKey(b)) != a {
		return nil, errors.New("invalid address checksum")
	}
	return ed25519.PublicKey(b), nil
}
func (h Hash) Bytes() ([]byte, error) {
	if len(h) != 64 || string(h) != strings.ToLower(string(h)) {
		return nil, errors.New("invalid hash")
	}
	b, e := hex.DecodeString(string(h))
	if e != nil {
		return nil, errors.New("invalid hash")
	}
	return b, nil
}
func Digest(b []byte) Hash { h := sha256.Sum256(b); return Hash(hex.EncodeToString(h[:])) }
