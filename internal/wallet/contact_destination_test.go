package wallet

import (
	"errors"
	"testing"
)

func TestContactDestinationEnforcesNetworkAndExplicitRecipient(t *testing.T) {
	path, c := profileFixture(t)
	if err := PutContact(path, "Alice", c, false); err != nil {
		t.Fatal(err)
	}
	f := commandFlags{path: path}
	account, network, err := contactDestination(f, "Alice", "")
	if err != nil || account != c.Address || network != c.Network {
		t.Fatal("contact destination lost its network pin")
	}
	if _, _, err = contactDestination(f, "Alice", string(c.Address)); err == nil {
		t.Fatal("ambiguous explicit destination accepted")
	}
	f.expectedNetwork = "other"
	if _, _, err = contactDestination(f, "Alice", ""); err == nil {
		t.Fatal("conflicting network pin accepted")
	}
	if _, err := LookupContact(path, "absent"); !errors.Is(err, ErrNameNotFound) {
		t.Fatal("absent contact accepted")
	}
}
