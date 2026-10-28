package wallet

import (
	"encoding/json"
	"errors"
	"example.com/go-block-lattice/internal/protocol"
	"os"
	"path/filepath"
	"testing"
)

func TestContactArchivesMergeAtomically(t *testing.T) {
	source, c := profileFixture(t)
	target, _ := profileFixture(t)
	if err := PutContact(source, "Alice", c, false); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "contacts")
	if err := ExportContacts(source, archive); err != nil {
		t.Fatal(err)
	}
	if err := ExportContacts(source, archive); err == nil {
		t.Fatal("contact export overwrote existing path")
	}
	for i := 0; i < 2; i++ {
		if err := ImportContacts(target, archive, false); err != nil {
			t.Fatal("idempotent import failed", err)
		}
	}
	changed := c
	changed.Network = protocol.Digest([]byte("other network"))
	if err := PutContact(target, "Alice", changed, true); err != nil {
		t.Fatal(err)
	}
	if err := ImportContacts(target, archive, false); !errors.Is(err, ErrNameExists) {
		t.Fatal("implicit import overwrite")
	}
	got, err := LookupContact(target, "Alice")
	if err != nil || got != changed {
		t.Fatal("failed merge modified recipient")
	}
	if err := ImportContacts(target, archive, true); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(contactArchive{Version: 1, Contacts: []NamedContact{{Name: "New", Contact: c}, {Name: "New", Contact: c}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(archive, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if err = ImportContacts(target, archive, true); err == nil {
		t.Fatal("duplicate names accepted")
	}
}
