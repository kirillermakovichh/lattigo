//go:build faultaudit

package wallet

import (
	"errors"
	"testing"

	"example.com/go-block-lattice/internal/faultprobe"
)

func TestProfilePublicationFailuresKeepExistingContacts(t *testing.T) {
	for _, site := range []string{"profile.go/editProfile/json.MarshalIndent/1", "journal.go/AtomicWrite/os.Rename/1", "wallet.go/readPrivateLimit/io.ReadAll/1"} {
		t.Run(site, func(t *testing.T) {
			path, c := profileFixture(t)
			if err := PutContact(path, "Alice", c, false); err != nil {
				t.Fatal(err)
			}
			faultprobe.Arm(site, 1)
			defer faultprobe.Arm("", 0)
			err := PutContact(path, "Bob", c, false)
			if !faultprobe.Reached() || !errors.Is(err, faultprobe.Injected) {
				t.Fatalf("profile fault not propagated: %v", err)
			}
			faultprobe.Arm("", 0)
			rows, err := Contacts(path)
			if err != nil || len(rows) != 1 || rows[0].Name != "Alice" {
				t.Fatal("failed publication changed address book")
			}
		})
	}
}
