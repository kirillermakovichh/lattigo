package wallet

import (
	"fmt"

	"example.com/go-block-lattice/internal/protocol"
)

type OutboxEntry struct {
	Hash  protocol.Hash  `json:"hash"`
	State string         `json:"state"`
	Block protocol.Block `json:"block"`
}

func outboxFile(path string, index uint64) (string, error) {
	base, err := canonicalWallet(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s.pending-%d.json", base, index), nil
}

// InspectOutbox verifies the stored signature. Stored does not imply RPC admission or confirmation.
func InspectOutbox(path string, index uint64) (OutboxEntry, error) {
	file, err := outboxFile(path, index)
	if err != nil {
		return OutboxEntry{}, err
	}
	b, err := loadBlock(file)
	if err != nil {
		return OutboxEntry{}, err
	}
	return OutboxEntry{Hash: b.ID(), State: "stored", Block: b}, nil
}
