package protocol

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
)

type Block struct {
	Version        uint8   `json:"version"`
	Network        Hash    `json:"network"`
	Kind           string  `json:"kind"`
	Account        Account `json:"account"`
	Previous       Hash    `json:"previous"`
	Representative Account `json:"representative"`
	Balance        Amount  `json:"balance"`
	Link           string  `json:"link"`
	Work           uint64  `json:"work"`
	Signature      string  `json:"signature"`
}

func (b Block) Canonical() ([]byte, error) {
	if b.Version != 1 {
		return nil, errors.New("unsupported block version")
	}
	network, e := b.Network.Bytes()
	if e != nil {
		return nil, fmt.Errorf("network: %w", e)
	}
	account, e := b.Account.PublicKey()
	if e != nil {
		return nil, fmt.Errorf("account: %w", e)
	}
	representative, e := b.Representative.PublicKey()
	if e != nil {
		return nil, fmt.Errorf("representative: %w", e)
	}
	previous := make([]byte, 32)
	if b.Previous != "" {
		previous, e = b.Previous.Bytes()
		if e != nil {
			return nil, fmt.Errorf("previous: %w", e)
		}
	}
	var kind byte
	var link []byte
	switch b.Kind {
	case "send":
		kind = 1
		p, e := Account(b.Link).PublicKey()
		if e != nil {
			return nil, fmt.Errorf("destination: %w", e)
		}
		link = p
	case "receive":
		kind = 2
		link, e = Hash(b.Link).Bytes()
		if e != nil {
			return nil, fmt.Errorf("source: %w", e)
		}
	case "change":
		kind = 3
		if b.Link != "" {
			return nil, errors.New("change link must be empty")
		}
		link = make([]byte, 32)
	default:
		return nil, errors.New("unsupported block kind")
	}
	result := make([]byte, 0, 179)
	result = append(result, []byte("BL/block/v1")...)
	result = append(result, b.Version)
	result = append(result, network...)
	result = append(result, kind)
	result = append(result, account...)
	result = append(result, previous...)
	result = append(result, representative...)
	result = binary.BigEndian.AppendUint64(result, uint64(b.Balance))
	result = append(result, link...)
	return result, nil
}
func (b Block) ID() Hash {
	v, e := b.Canonical()
	if e != nil {
		return ""
	}
	return Digest(v)
}
func (b *Block) Sign(key ed25519.PrivateKey) error {
	if len(key) != ed25519.PrivateKeySize {
		return errors.New("invalid private key length")
	}
	if Address(key.Public().(ed25519.PublicKey)) != b.Account {
		return errors.New("private key does not own block account")
	}
	data, e := b.Canonical()
	if e != nil {
		return e
	}
	h := sha256.Sum256(data)
	b.Signature = hex.EncodeToString(ed25519.Sign(key, h[:]))
	return nil
}
func (b Block) Verify() error {
	data, e := b.Canonical()
	if e != nil {
		return e
	}
	sig, e := hex.DecodeString(b.Signature)
	if e != nil || len(sig) != ed25519.SignatureSize || hex.EncodeToString(sig) != b.Signature {
		return errors.New("invalid signature encoding")
	}
	pub, _ := b.Account.PublicKey()
	h := sha256.Sum256(data)
	if !ed25519.Verify(pub, h[:], sig) {
		return errors.New("invalid block signature")
	}
	return nil
}

const workPayloadSize = len("BL/work/v1") + 32 + 8

func workOK(id []byte, nonce uint64, bits uint8) bool {
	if len(id) != 32 {
		return false
	}
	var scratch [workPayloadSize]byte
	copy(scratch[:], "BL/work/v1")
	copy(scratch[len("BL/work/v1"):], id)
	return workScratchOK(&scratch, nonce, bits)
}
func workScratchOK(scratch *[workPayloadSize]byte, nonce uint64, bits uint8) bool {
	if bits > 24 {
		return false
	}
	binary.BigEndian.PutUint64(scratch[workPayloadSize-8:], nonce)
	h := sha256.Sum256(scratch[:])
	whole := int(bits / 8)
	for i := 0; i < whole; i++ {
		if h[i] != 0 {
			return false
		}
	}
	rest := bits % 8
	return rest == 0 || h[whole]>>(8-rest) == 0
}
func (b Block) CheckWork(bits uint8) bool {
	id, e := b.ID().Bytes()
	return e == nil && workOK(id, b.Work, bits)
}
func (b *Block) GenerateWork(ctx context.Context, bits uint8) error {
	if bits > 24 {
		return errors.New("work bits exceed 24")
	}
	id, e := b.ID().Bytes()
	if e != nil {
		return e
	}
	var scratch [workPayloadSize]byte
	copy(scratch[:], "BL/work/v1")
	copy(scratch[len("BL/work/v1"):], id)
	for nonce := uint64(0); ; nonce++ {
		if nonce%1024 == 0 {
			if e := ctx.Err(); e != nil {
				return e
			}
		}
		if workScratchOK(&scratch, nonce, bits) {
			b.Work = nonce
			return nil
		}
		if nonce == math.MaxUint64 {
			return errors.New("work nonce exhausted")
		}
	}
}
