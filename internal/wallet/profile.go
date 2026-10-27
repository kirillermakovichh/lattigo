package wallet

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"example.com/go-block-lattice/internal/protocol"
	"example.com/go-block-lattice/internal/storage"
)

const maxProfileBytes = 256 << 10
const maxProfileEntries = 1024

var ErrNameNotFound = errors.New("wallet name not found")
var ErrNameExists = errors.New("wallet name already exists")

type Contact struct {
	Address protocol.Account `json:"address"`
	Network protocol.Hash    `json:"network"`
}
type NamedContact struct {
	Name string `json:"name"`
	Contact
}
type profile struct {
	Version  int                `json:"version"`
	Contacts map[string]Contact `json:"contacts"`
	Accounts map[string]uint64  `json:"accounts"`
}

func validName(name string) bool {
	if len(name) == 0 || len(name) > 64 || !utf8.ValidString(name) || strings.TrimSpace(name) != name {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func (c Contact) validate() error {
	if _, err := c.Address.PublicKey(); err != nil {
		return err
	}
	_, err := c.Network.Bytes()
	return err
}
func (p profile) validate() error {
	if p.Version != 1 || len(p.Contacts) > maxProfileEntries || len(p.Accounts) > maxProfileEntries {
		return errors.New("unsupported or oversized wallet profile")
	}
	for name, c := range p.Contacts {
		if !validName(name) {
			return errors.New("invalid contact name")
		}
		if err := c.validate(); err != nil {
			return err
		}
	}
	for name := range p.Accounts {
		if !validName(name) {
			return errors.New("invalid account name")
		}
	}
	return nil
}
func canonicalWallet(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(resolved)
}
func readProfile(base string) (profile, error) {
	p := profile{Version: 1, Contacts: map[string]Contact{}, Accounts: map[string]uint64{}}
	data, err := readPrivateLimit(base+".profile.json", maxProfileBytes)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	p = profile{}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&p); err != nil {
		return p, err
	}
	if err = dec.Decode(new(any)); err != io.EOF {
		return p, errors.New("wallet profile contains trailing JSON")
	}
	if p.Contacts == nil {
		p.Contacts = map[string]Contact{}
	}
	if p.Accounts == nil {
		p.Accounts = map[string]uint64{}
	}
	return p, p.validate()
}
func editProfile(path string, edit func(*profile) error) error {
	base, err := canonicalWallet(path)
	if err != nil {
		return err
	}
	lock, err := lockPrivate(base + ".profile.lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	p, err := readProfile(base)
	if err != nil {
		return err
	}
	if err = edit(&p); err != nil {
		return err
	}
	if err = p.validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > maxProfileBytes {
		return errors.New("wallet profile exceeds byte budget")
	}
	return storage.AtomicWrite(filepath.Dir(base), filepath.Base(base)+".profile.json", data)
}
func Contacts(path string) ([]NamedContact, error) {
	base, err := canonicalWallet(path)
	if err != nil {
		return nil, err
	}
	p, err := readProfile(base)
	if err != nil {
		return nil, err
	}
	out := make([]NamedContact, 0, len(p.Contacts))
	for name, c := range p.Contacts {
		out = append(out, NamedContact{Name: name, Contact: c})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
func PutContact(path, name string, contact Contact, replace bool) error {
	return editProfile(path, func(p *profile) error {
		if _, exists := p.Contacts[name]; exists && !replace {
			return ErrNameExists
		}
		p.Contacts[name] = contact
		return nil
	})
}
func DeleteContact(path, name string) error {
	return editProfile(path, func(p *profile) error {
		if _, exists := p.Contacts[name]; !exists {
			return ErrNameNotFound
		}
		delete(p.Contacts, name)
		return nil
	})
}
