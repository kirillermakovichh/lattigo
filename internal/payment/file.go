package payment

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

const MaxFileBytes = 8 << 10

type requestFile struct {
	Version int `json:"version"`
	Request
}

func EncodeFile(r Request) ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return json.MarshalIndent(requestFile{Version: 1, Request: r}, "", "  ")
}
func DecodeFile(raw []byte) (Request, error) {
	var file requestFile
	if len(raw) == 0 || len(raw) > MaxFileBytes || !utf8.Valid(raw) {
		return file.Request, errors.New("invalid or oversized payment request file")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	start, err := dec.Token()
	if err != nil {
		return file.Request, err
	}
	if start != json.Delim('{') {
		return file.Request, errors.New("payment request file must contain an object")
	}
	seen := map[string]bool{}
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return file.Request, err
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return file.Request, errors.New("invalid or repeated payment request field")
		}
		seen[key] = true
		var target any
		switch key {
		case "version":
			target = &file.Version
		case "address":
			target = &file.Address
		case "network":
			target = &file.Network
		case "amount":
			target = &file.Amount
		case "label":
			target = &file.Label
		case "message":
			target = &file.Message
		default:
			return file.Request, errors.New("unknown payment request field")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return file.Request, err
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return file.Request, errors.New("payment request fields cannot be null")
		}
		if err := json.Unmarshal(value, target); err != nil {
			return file.Request, err
		}
	}
	if _, err := dec.Token(); err != nil {
		return file.Request, err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return file.Request, errors.New("payment request file has trailing JSON")
	}
	if file.Version != 1 {
		return file.Request, errors.New("unsupported payment request version")
	}
	return file.Request, file.Request.Validate()
}
