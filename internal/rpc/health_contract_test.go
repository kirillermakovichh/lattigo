package rpc

import (
	"example.com/go-block-lattice/internal/protocol"
	"testing"
)

func TestReadinessAndMetricsUseStableEngineSchema(t *testing.T) {
	s, _ := branchServer(t)
	health := s.cfg.Engine.Health()
	if _, ok := health["ready"].(bool); !ok {
		t.Fatal("engine readiness omitted its bool contract")
	}
	for _, field := range []string{"pending", "round", "observed_voting_weight", "quorum_weight"} {
		switch health[field].(type) {
		case int, uint32, protocol.Amount:
		default:
			t.Fatalf("unsupported consensus metric %s", field)
		}
	}
	a := s.cfg.Store.Genesis().Allocations[0].Account
	if s.cfg.Store.History(a, 0, 1) == nil || s.cfg.Store.Receivables(a, 0, 1) == nil || s.cfg.Store.Records(0, 1) == nil {
		t.Fatal("empty pages must encode as arrays")
	}
}
