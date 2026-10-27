package wallet

import (
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"example.com/go-block-lattice/internal/protocol"
)

func profileFixture(t *testing.T) (string, Contact) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "wallet")
	// The profile is independent of encrypted seed contents, but tied to an existing file.
	if err := os.WriteFile(path, []byte("wallet fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	key := ed25519.NewKeyFromSeed(make([]byte, 32))
	return path, Contact{Address: protocol.Address(key.Public().(ed25519.PublicKey)), Network: protocol.Digest([]byte("profile-network"))}
}
func TestAddressBookPersistsPrivateSortedContacts(t *testing.T) {
	path, c := profileFixture(t)
	for _, name := range []string{"Zulu", "Alice"} {
		if err := PutContact(path, name, c, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := PutContact(path, "Alice", c, false); !errors.Is(err, ErrNameExists) {
		t.Fatal("implicit contact overwrite")
	}
	rows, err := Contacts(path)
	if err != nil || len(rows) != 2 || rows[0].Name != "Alice" {
		t.Fatalf("contacts: %v %v", rows, err)
	}
	if err = DeleteContact(path, "Zulu"); err != nil {
		t.Fatal(err)
	}
	if err = DeleteContact(path, "absent"); !errors.Is(err, ErrNameNotFound) {
		t.Fatal("missing contact deletion acknowledged")
	}
	info, err := os.Stat(path + ".profile.json")
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("profile not private")
	}
	if err = PutContact(path, "invalid\nname", c, false); err == nil {
		t.Fatal("invalid contact persisted")
	}
}
