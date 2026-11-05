// Package ledger owns confirmed account transitions and their quorum certificates.
package ledger

import (
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"

	"example.com/go-block-lattice/internal/protocol"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrDependency   = errors.New("missing dependency")
	ErrInvalid      = errors.New("invalid")
	ErrClosed       = errors.New("store closed")
	ErrCapacity     = errors.New("configured history capacity reached")
	ErrObserverOnly = errors.New("restored ledger is observer-only; use a fresh signing identity in a fresh directory")
)

type AccountState struct {
	Account        protocol.Account `json:"account"`
	Frontier       protocol.Hash    `json:"frontier"`
	Balance        protocol.Amount  `json:"balance"`
	Representative protocol.Account `json:"representative"`
	Height         uint64           `json:"height"`
}
type Receivable struct {
	Source protocol.Hash    `json:"source"`
	From   protocol.Account `json:"from"`
	To     protocol.Account `json:"to"`
	Amount protocol.Amount  `json:"amount"`
}
type Vote struct {
	Network        protocol.Hash    `json:"network"`
	Height         uint64           `json:"height"`
	Round          uint32           `json:"round"`
	Phase          string           `json:"phase"`
	Parent         protocol.Hash    `json:"parent"`
	Block          protocol.Hash    `json:"block"`
	Representative protocol.Account `json:"representative"`
	Signature      string           `json:"signature"`
}

func (v Vote) signingBytes() ([]byte, error) {
	n, e := v.Network.Bytes()
	if e != nil {
		return nil, e
	}
	p, e := v.Parent.Bytes()
	if e != nil {
		return nil, e
	}
	if v.Height == 0 {
		return nil, errors.New("zero vote height")
	}
	var phase byte
	switch v.Phase {
	case "prevote":
		phase = 1
	case "precommit":
		phase = 2
	default:
		return nil, errors.New("invalid vote phase")
	}
	block := make([]byte, 32)
	if v.Block != "" {
		block, e = v.Block.Bytes()
		if e != nil {
			return nil, e
		}
	}
	rep, e := v.Representative.PublicKey()
	if e != nil {
		return nil, e
	}
	b := append([]byte("BL/vote/v1"), n...)
	b = binary.BigEndian.AppendUint64(b, v.Height)
	b = binary.BigEndian.AppendUint32(b, v.Round)
	b = append(b, phase)
	b = append(b, p...)
	b = append(b, block...)
	b = append(b, rep...)
	return b, nil
}
func (v *Vote) Sign(key ed25519.PrivateKey) error {
	if len(key) != 64 {
		return errors.New("invalid voting key")
	}
	if protocol.Address(key.Public().(ed25519.PublicKey)) != v.Representative {
		return errors.New("voting key mismatch")
	}
	b, e := v.signingBytes()
	if e != nil {
		return e
	}
	v.Signature = hex.EncodeToString(ed25519.Sign(key, b))
	return nil
}
func (v Vote) Verify() error {
	b, e := v.signingBytes()
	if e != nil {
		return e
	}
	sig, e := hex.DecodeString(v.Signature)
	if e != nil || len(sig) != 64 || hex.EncodeToString(sig) != v.Signature {
		return errors.New("invalid vote signature encoding")
	}
	key, _ := v.Representative.PublicKey()
	if !ed25519.Verify(key, b, sig) {
		return errors.New("invalid vote signature")
	}
	return nil
}

type Record struct {
	Height   uint64         `json:"height"`
	Round    uint32         `json:"round"`
	Previous protocol.Hash  `json:"previous"`
	Block    protocol.Block `json:"block"`
	Votes    []Vote         `json:"votes"`
}

func (r Record) ID() protocol.Hash {
	parent, e := r.Previous.Bytes()
	if e != nil {
		return ""
	}
	block, e := r.Block.ID().Bytes()
	if e != nil || r.Height == 0 {
		return ""
	}
	b := binary.BigEndian.AppendUint64([]byte("BL/record/v1"), r.Height)
	b = append(b, parent...)
	b = append(b, block...)
	return protocol.Digest(b)
}

type Status struct {
	HistoryBytes      uint64          `json:"history_bytes"`
	HistoryLimitBytes uint64          `json:"history_limit_bytes"`
	CapacityReached   bool            `json:"capacity_reached"`
	Network           protocol.Hash   `json:"network"`
	Height            uint64          `json:"height"`
	Tip               protocol.Hash   `json:"tip"`
	Supply            protocol.Amount `json:"supply"`
	Accounts          int             `json:"accounts"`
	Receivables       int             `json:"receivables"`
}

// HasQuorum computes strict >2/3 without multiplying a possibly maximal supply.
func HasQuorum(weight, total protocol.Amount) bool {
	n := uint64(total)
	threshold := (n/3)*2 + (n%3)*2/3
	return total > 0 && weight <= total && uint64(weight) > threshold
}
func certificate(r Record, network protocol.Hash, weights map[protocol.Account]protocol.Amount, total protocol.Amount) error {
	if len(r.Votes) == 0 || len(r.Votes) > protocol.MaxRepresentatives {
		return fmt.Errorf("%w: certificate size", ErrInvalid)
	}
	blockID := r.Block.ID()
	seen := make(map[protocol.Account]bool, len(r.Votes))
	var sum uint64
	for _, v := range r.Votes {
		if v.Network != network || v.Height != r.Height || v.Round != r.Round || v.Parent != r.Previous || v.Block != blockID || v.Phase != "precommit" {
			return fmt.Errorf("%w: certificate vote binding", ErrInvalid)
		}
		if seen[v.Representative] {
			return fmt.Errorf("%w: repeated representative", ErrInvalid)
		}
		seen[v.Representative] = true
		if e := v.Verify(); e != nil {
			return fmt.Errorf("%w: %v", ErrInvalid, e)
		}
		w := uint64(weights[v.Representative])
		if w == 0 || math.MaxUint64-sum < w {
			return fmt.Errorf("%w: voting weight", ErrInvalid)
		}
		sum += w
	}
	if !HasQuorum(protocol.Amount(sum), total) {
		return fmt.Errorf("%w: insufficient quorum", ErrInvalid)
	}
	return nil
}
