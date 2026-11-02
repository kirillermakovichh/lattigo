package rpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"unicode"
	"unicode/utf8"

	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

const maxBatchQueries = 32

type readQuery struct {
	ID        string           `json:"id"`
	Operation string           `json:"operation"`
	Address   protocol.Account `json:"address,omitempty"`
	Hash      protocol.Hash    `json:"hash,omitempty"`
}
type queryList []readQuery

func (q *queryList) UnmarshalJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	start, err := d.Token()
	if err != nil {
		return err
	}
	if start != json.Delim('[') {
		return errors.New("queries must be an array")
	}
	rows := queryList{}
	for d.More() {
		if len(rows) >= maxBatchQueries {
			return errors.New("batch exceeds 32 queries")
		}
		var row readQuery
		if err := d.Decode(&row); err != nil {
			return err
		}
		rows = append(rows, row)
	}
	if _, err := d.Token(); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("trailing query array JSON")
	}
	*q = rows
	return nil
}

type readError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type readResult struct {
	ID    string     `json:"id"`
	Data  any        `json:"data,omitempty"`
	Error *readError `json:"error,omitempty"`
}

func validQueryID(id string) bool {
	if len(id) == 0 || len(id) > 64 || !utf8.ValidString(id) {
		return false
	}
	for _, r := range id {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func (s *Server) readOne(q readQuery) readResult {
	result := readResult{ID: q.ID}
	invalid := func(code string) readResult {
		result.Error = &readError{Code: code, Message: "invalid read query"}
		return result
	}
	var err error
	switch q.Operation {
	case "status":
		if q.Address != "" || q.Hash != "" {
			return invalid("invalid_query")
		}
		result.Data = s.cfg.Store.Status()
	case "account":
		if _, err := q.Address.PublicKey(); err != nil || q.Hash != "" {
			return invalid("invalid_address")
		}
		result.Data, err = s.cfg.Store.Account(q.Address)
	case "block":
		if _, err := q.Hash.Bytes(); err != nil || q.Address != "" {
			return invalid("invalid_hash")
		}
		result.Data, err = s.cfg.Store.Block(q.Hash)
	default:
		return invalid("invalid_operation")
	}
	if err != nil {
		result.Data = nil
		if errors.Is(err, ledger.ErrNotFound) {
			result.Error = &readError{Code: "not_found", Message: "confirmed data not found"}
		} else {
			result.Error = &readError{Code: "unavailable", Message: "confirmed data is unavailable"}
		}
	}
	return result
}
func (s *Server) batchRead(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Queries queryList `json:"queries"`
	}
	if err := decode(w, r, &request); err != nil {
		writeError(w, 400, "invalid_batch", "invalid or oversized read batch")
		return
	}
	if len(request.Queries) == 0 {
		writeError(w, 400, "invalid_batch", "batch needs 1..32 queries")
		return
	}
	seen := map[string]bool{}
	for _, q := range request.Queries {
		if !validQueryID(q.ID) || seen[q.ID] {
			writeError(w, 400, "invalid_batch", "query IDs must be unique bounded strings")
			return
		}
		seen[q.ID] = true
	}
	results := make([]readResult, 0, len(request.Queries))
	for _, q := range request.Queries {
		results = append(results, s.readOne(q))
	}
	writeJSON(w, 200, results)
}
