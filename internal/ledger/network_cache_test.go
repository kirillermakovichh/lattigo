package ledger

import "testing"

func TestGenesisIdentityRemainsOwnedByLedger(t *testing.T) {
	state, _, _ := fixture(t)
	g := state.genesis
	owned := newState(g)
	want := owned.network
	g.Allocations[0].Balance--
	if owned.genesis.ID() != want || owned.network != want || owned.tip != want {
		t.Fatal("caller mutated ledger genesis")
	}
	s, _, _, _ := viewFixture(t)
	copy := s.Genesis()
	before := s.Status().Network
	copy.Allocations[0].Balance--
	if s.Status().Network != before || s.Genesis().ID() != before {
		t.Fatal("exported genesis aliases store")
	}
}
