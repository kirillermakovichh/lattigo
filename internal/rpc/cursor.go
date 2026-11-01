package rpc

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"

	"example.com/go-block-lattice/internal/protocol"
)

const maxCursorBytes = 1024

var errCursor = errors.New("invalid page cursor; restart pagination")

type pageCursor struct {
	Version int              `json:"version"`
	Network protocol.Hash    `json:"network"`
	Kind    string           `json:"kind"`
	Account protocol.Account `json:"account"`
	Height  uint64           `json:"height"`
	Tip     protocol.Hash    `json:"tip"`
	Before  uint64           `json:"before,omitempty"`
	After   protocol.Hash    `json:"after,omitempty"`
	Limit   int              `json:"limit"`
}
type cursorCodec [32]byte

func (p pageCursor) validate() error {
	if p.Version != 1 || p.Limit < 1 || p.Limit > 1000 {
		return errCursor
	}
	if _, err := p.Network.Bytes(); err != nil {
		return errCursor
	}
	if _, err := p.Account.PublicKey(); err != nil {
		return errCursor
	}
	if _, err := p.Tip.Bytes(); err != nil {
		return errCursor
	}
	switch p.Kind {
	case "history":
		if p.Before > p.Height || p.After != "" {
			return errCursor
		}
	case "receivables":
		if p.Before != 0 {
			return errCursor
		}
		if p.After != "" {
			if _, err := p.After.Bytes(); err != nil {
				return errCursor
			}
		}
	default:
		return errCursor
	}
	return nil
}
func (c cursorCodec) tag(raw []byte) []byte {
	mac := hmac.New(sha256.New, c[:])
	mac.Write([]byte("BL/rpc/cursor/v1"))
	mac.Write(raw)
	return mac.Sum(nil)
}
func (c cursorCodec) encode(p pageCursor) (string, error) {
	if err := p.validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(append(raw, c.tag(raw)...)), nil
}
func (c cursorCodec) decode(token string) (pageCursor, error) {
	var p pageCursor
	if len(token) == 0 || len(token) > maxCursorBytes {
		return p, errCursor
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) <= sha256.Size || base64.RawURLEncoding.EncodeToString(raw) != token {
		return p, errCursor
	}
	payload, tag := raw[:len(raw)-sha256.Size], raw[len(raw)-sha256.Size:]
	if !hmac.Equal(tag, c.tag(payload)) {
		return p, errCursor
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&p); err != nil {
		return p, errCursor
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return p, errCursor
	}
	return p, p.validate()
}
