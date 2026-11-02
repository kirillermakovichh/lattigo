package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"unicode"
	"unicode/utf8"

	"example.com/go-block-lattice/internal/protocol"
)

const MaxBatchQueries = 32

type ReadOperation string

const (
	ReadStatus  ReadOperation = "status"
	ReadAccount ReadOperation = "account"
	ReadBlock   ReadOperation = "block"
)

type ReadQuery struct {
	ID        string           `json:"id"`
	Operation ReadOperation    `json:"operation"`
	Address   protocol.Account `json:"address,omitempty"`
	Hash      protocol.Hash    `json:"hash,omitempty"`
}
type ReadResult struct {
	ID    string          `json:"id"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error *Error          `json:"error,omitempty"`
}

func (r ReadResult) Decode(v any) error {
	if r.Error != nil {
		return r.Error
	}
	return json.Unmarshal(r.Data, v)
}

type readResults []ReadResult

func (rows *readResults) UnmarshalJSON(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	start, err := d.Token()
	if err != nil {
		return err
	}
	if start != json.Delim('[') {
		return errors.New("batch response must be an array")
	}
	out := readResults{}
	for d.More() {
		if len(out) >= MaxBatchQueries {
			return errors.New("batch response exceeds row budget")
		}
		var row ReadResult
		if err := d.Decode(&row); err != nil {
			return err
		}
		if len(row.Data) > 1<<20 {
			return errors.New("batch row exceeds data budget")
		}
		out = append(out, row)
	}
	if _, err := d.Token(); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return errors.New("trailing batch response")
	}
	*rows = out
	return nil
}
func validReadID(id string) bool {
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
func validateReadData(query ReadQuery, raw json.RawMessage) error {
	switch query.Operation {
	case ReadAccount:
		var state AccountState
		if err := json.Unmarshal(raw, &state); err != nil {
			return err
		}
		if state.Account != query.Address {
			return errors.New("batch returned another account")
		}
	case ReadBlock:
		var b protocol.Block
		if err := json.Unmarshal(raw, &b); err != nil {
			return err
		}
		if b.ID() != query.Hash {
			return errors.New("batch returned another block")
		}
		return b.Verify()
	case ReadStatus:
		var status Status
		if err := json.Unmarshal(raw, &status); err != nil {
			return err
		}
		if _, err := status.Network.Bytes(); err != nil {
			return err
		}
		if _, err := status.Tip.Bytes(); err != nil {
			return err
		}
	default:
		return errors.New("unsupported operation reported success")
	}
	return nil
}
func (c *Client) Batch(ctx context.Context, queries []ReadQuery) ([]ReadResult, error) {
	if len(queries) == 0 || len(queries) > MaxBatchQueries {
		return nil, errors.New("batch needs 1..32 queries")
	}
	seen := map[string]bool{}
	for _, query := range queries {
		if !validReadID(query.ID) || seen[query.ID] {
			return nil, errors.New("batch IDs must be unique bounded strings")
		}
		seen[query.ID] = true
	}
	var rows readResults
	if err := c.request(ctx, "POST", "/v1/query", struct {
		Queries []ReadQuery `json:"queries"`
	}{queries}, &rows); err != nil {
		return nil, err
	}
	if len(rows) != len(queries) {
		return nil, errors.New("batch response has wrong row count")
	}
	for i, row := range rows {
		if row.ID != queries[i].ID {
			return nil, errors.New("batch response changed correlation order")
		}
		if (len(row.Data) == 0) == (row.Error == nil) {
			return nil, errors.New("batch row must contain exactly one data or error result")
		}
		if row.Error != nil {
			if len(row.Error.Code) == 0 || len(row.Error.Code) > 64 || len(row.Error.Message) > 1024 {
				return nil, errors.New("invalid batch error")
			}
		} else if err := validateReadData(queries[i], row.Data); err != nil {
			return nil, err
		}
	}
	return []ReadResult(rows), nil
}
