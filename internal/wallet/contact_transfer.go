package wallet

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
)

type contactArchive struct {
	Version  int            `json:"version"`
	Contacts []NamedContact `json:"contacts"`
}

func ExportContacts(path, destination string) error {
	rows, err := Contacts(path)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(contactArchive{Version: 1, Contacts: rows}, "", "  ")
	if err != nil {
		return err
	}
	if len(data)+1 > maxProfileBytes {
		return errors.New("contact export exceeds byte budget")
	}
	return WritePrivate(destination, append(data, '\n'))
}
func ImportContacts(path, source string, replace bool) error {
	raw, err := readPrivateLimit(source, maxProfileBytes)
	if err != nil {
		return err
	}
	var archive contactArchive
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&archive); err != nil {
		return err
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		return errors.New("contact archive contains trailing JSON")
	}
	if archive.Version != 1 || len(archive.Contacts) > maxProfileEntries {
		return errors.New("unsupported or oversized contact archive")
	}
	seen := map[string]bool{}
	for _, row := range archive.Contacts {
		if !validName(row.Name) || seen[row.Name] {
			return errors.New("invalid or repeated contact name")
		}
		seen[row.Name] = true
		if err := row.Contact.validate(); err != nil {
			return err
		}
	}
	return editProfile(path, func(p *profile) error {
		for _, row := range archive.Contacts {
			if old, ok := p.Contacts[row.Name]; ok && old != row.Contact && !replace {
				return ErrNameExists
			}
			p.Contacts[row.Name] = row.Contact
		}
		return nil
	})
}
