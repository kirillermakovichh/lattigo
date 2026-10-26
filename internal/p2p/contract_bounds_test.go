package p2p

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
	"example.com/go-block-lattice/internal/storage"
)

func TestMaximumCertificateEncodingFitsJournalAndPeerBudgets(t *testing.T) {
	// An upper bound on every accepted record: hashes, signatures, identities and
	// phases have validated fixed lengths; integers use their widest decimal form.
	account := protocol.Account("bl_" + strings.Repeat("f", 72))
	hash := protocol.Hash(strings.Repeat("f", 64))
	signature := strings.Repeat("f", 128)
	record := ledger.Record{Height: math.MaxUint64, Round: math.MaxUint32, Previous: hash, Block: protocol.Block{Version: 1, Network: hash, Kind: "receive", Account: account, Previous: hash, Representative: account, Balance: protocol.Amount(math.MaxUint64), Link: string(account), Work: math.MaxUint64, Signature: signature}}
	vote := ledger.Vote{Network: hash, Height: math.MaxUint64, Round: math.MaxUint32, Phase: "precommit", Parent: hash, Block: hash, Representative: account, Signature: signature}
	record.Votes = make([]ledger.Vote, protocol.MaxRepresentatives)
	for i := range record.Votes {
		record.Votes[i] = vote
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > storage.MaxFrame || len(raw)+2 > maxResponse {
		t.Fatal("protocol certificate cannot fit a journal frame and a one-record peer response")
	}
	// Records caps multi-record pages at 28MiB and permits one oversized record.
	// Thus the 32MiB transport guard cannot trip with an accepted ledger state.
	if 28<<20 >= maxResponse || storage.MaxFrame+2 > maxResponse {
		t.Fatal("ledger page contract exceeds peer transport budget")
	}
	for _, origin := range []string{"http://host\r:123", "http://host\n:123", "http://host%0a:123"} {
		if _, err := ValidatePeer(origin); err == nil {
			t.Fatal("control character admitted in peer origin")
		}
	}
}
