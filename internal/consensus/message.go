// Package consensus orders account-chain confirmations using weighted two-phase voting.
package consensus

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

type Message struct {
	Type     string          `json:"type"`
	Proposal *Proposal       `json:"proposal,omitempty"`
	Vote     *ledger.Vote    `json:"vote,omitempty"`
	Record   *ledger.Record  `json:"record,omitempty"`
	Block    *protocol.Block `json:"block,omitempty"`
}

type Proposal struct {
	Network        protocol.Hash    `json:"network"`
	Height         uint64           `json:"height"`
	Round          uint32           `json:"round"`
	Parent         protocol.Hash    `json:"parent"`
	Block          protocol.Block   `json:"block"`
	ValidRound     int64            `json:"valid_round"`
	Proof          []ledger.Vote    `json:"proof,omitempty"`
	Representative protocol.Account `json:"representative"`
	Signature      string           `json:"signature"`
}

func (p Proposal) signingBytes() []byte {
	// All fields have a fixed struct order. Proof order is signed, not reinterpreted.
	p.Signature = ""
	b, _ := json.Marshal(p)
	return append([]byte("BL/proposal/v1"), b...)
}
func (p *Proposal) Sign(key ed25519.PrivateKey) error {
	if len(key) != ed25519.PrivateKeySize || protocol.Address(key.Public().(ed25519.PublicKey)) != p.Representative {
		return errors.New("proposal signing identity mismatch")
	}
	p.Signature = hex.EncodeToString(ed25519.Sign(key, p.signingBytes()))
	return nil
}
func (p Proposal) Verify() error {
	pub, err := p.Representative.PublicKey()
	if err != nil {
		return err
	}
	sig, err := hex.DecodeString(p.Signature)
	if err != nil || hex.EncodeToString(sig) != p.Signature || !ed25519.Verify(pub, p.signingBytes(), sig) {
		return errors.New("invalid proposal signature")
	}
	return nil
}

// quorum avoids multiplication overflow for all uint64 supplies.
func quorum(weight, total protocol.Amount) bool {
	threshold := uint64(total)/3*2 + (uint64(total)%3*2)/3
	return uint64(weight) > threshold
}
func sortedRepresentatives(weights map[protocol.Account]protocol.Amount) []protocol.Account {
	out := make([]protocol.Account, 0, len(weights))
	for a, w := range weights {
		if w > 0 {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
func proposer(height uint64, round uint32, weights map[protocol.Account]protocol.Amount) protocol.Account {
	reps := sortedRepresentatives(weights)
	if len(reps) == 0 {
		return ""
	}
	return reps[((height-1)%uint64(len(reps))+uint64(round)%uint64(len(reps)))%uint64(len(reps))]
}
