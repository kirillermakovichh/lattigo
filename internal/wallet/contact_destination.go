package wallet

import (
	"errors"

	"example.com/go-block-lattice/internal/protocol"
)

func LookupContact(path, name string) (Contact, error) {
	if !validName(name) {
		return Contact{}, errors.New("invalid contact name")
	}
	base, err := canonicalWallet(path)
	if err != nil {
		return Contact{}, err
	}
	p, err := readProfile(base)
	if err != nil {
		return Contact{}, err
	}
	c, ok := p.Contacts[name]
	if !ok {
		return Contact{}, ErrNameNotFound
	}
	return c, nil
}
func contactDestination(f commandFlags, name, to string) (protocol.Account, protocol.Hash, error) {
	if to != "" {
		return "", "", errors.New("--contact and --to cannot be combined")
	}
	c, err := LookupContact(f.path, name)
	if err != nil {
		return "", "", err
	}
	if f.expectedNetwork != "" && protocol.Hash(f.expectedNetwork) != c.Network {
		return "", "", errors.New("contact network differs from --expected-network")
	}
	return c.Address, c.Network, nil
}
