package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"example.com/go-block-lattice/internal/event"
	"example.com/go-block-lattice/internal/protocol"
)

const MaxEventBytes = (16 << 20) + 1024

var ErrEventProtocol = errors.New("invalid event stream")

type Event struct {
	ID     string          `json:"id"`
	Height uint64          `json:"height"`
	Block  protocol.Block  `json:"block"`
	Record json.RawMessage `json:"record"`
}

type EventOptions struct {
	Network protocol.Hash
	After   string
}

// Events consumes one connection. The returned position advances only after a
// callback succeeds. Callers may persist it and resume without losing delivery.
func (c *Client) Events(ctx context.Context, options EventOptions, consume func(Event) error) (string, error) {
	if consume == nil {
		return options.After, errors.New("event callback required")
	}
	if _, err := options.Network.Bytes(); err != nil {
		return options.After, err
	}
	if options.After != "" {
		if _, err := event.ParseID(options.After, options.Network); err != nil {
			return options.After, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", c.base+"/v1/events", nil)
	if err != nil {
		return options.After, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", c.agent)
	if options.After != "" {
		req.Header.Set("Last-Event-ID", options.After)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.http.Do(req)
	if err != nil {
		return options.After, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		data, err := io.ReadAll(io.LimitReader(response.Body, 8193))
		if err != nil {
			return options.After, err
		}
		if len(data) > 8192 {
			return options.After, fmt.Errorf("%w: error response budget", ErrEventProtocol)
		}
		var envelope struct {
			Error Error `json:"error"`
		}
		if json.Unmarshal(data, &envelope) != nil || envelope.Error.Code == "" {
			envelope.Error.Code = "http_error"
			envelope.Error.Message = http.StatusText(response.StatusCode)
		}
		envelope.Error.Status = response.StatusCode
		return options.After, &envelope.Error
	}
	if strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]) != "text/event-stream" {
		return options.After, fmt.Errorf("%w: content type", ErrEventProtocol)
	}
	return parseEvents(ctx, response.Body, options, consume)
}

// readEventLine bounds both fragmented lines and frames before appending bytes.
func readEventLine(reader *bufio.Reader, remaining int) ([]byte, error) {
	var line []byte
	for {
		part, err := reader.ReadSlice('\n')
		if len(part) > remaining-len(line) {
			return nil, fmt.Errorf("%w: frame exceeds limit", ErrEventProtocol)
		}
		line = append(line, part...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return line, err
	}
}
func parseEvents(ctx context.Context, source io.Reader, options EventOptions, consume func(Event) error) (string, error) {
	reader := bufio.NewReaderSize(source, 4096)
	last := options.After
	previous := uint64(0)
	if last != "" {
		previous, _ = event.ParseID(last, options.Network)
	}
	used := 0
	var id, kind string
	var data []byte
	for {
		if err := ctx.Err(); err != nil {
			return last, err
		}
		line, err := readEventLine(reader, MaxEventBytes-used)
		if err != nil {
			return last, err
		}
		used += len(line)
		line = bytes.TrimSuffix(bytes.TrimSuffix(line, []byte{'\n'}), []byte{'\r'})
		if len(line) == 0 {
			if id == "" && kind == "" && len(data) == 0 {
				used = 0
				continue
			}
			height, err := event.ParseID(id, options.Network)
			if err != nil || height <= previous || kind != "confirmed" {
				return last, fmt.Errorf("%w: event scope or order", ErrEventProtocol)
			}
			var record struct {
				Height uint64         `json:"height"`
				Block  protocol.Block `json:"block"`
			}
			if json.Unmarshal(data, &record) != nil || record.Height != height || record.Block.Network != options.Network || record.Block.Verify() != nil {
				return last, fmt.Errorf("%w: confirmed record", ErrEventProtocol)
			}
			if err := consume(Event{ID: id, Height: height, Block: record.Block, Record: append(json.RawMessage(nil), data...)}); err != nil {
				return last, &eventConsumerError{err}
			}
			last = id
			previous = height
			id = ""
			kind = ""
			data = nil
			used = 0
			continue
		}
		if line[0] == ':' {
			continue
		}
		field, value, _ := bytes.Cut(line, []byte{':'})
		value = bytes.TrimPrefix(value, []byte{' '})
		switch string(field) {
		case "id":
			if id != "" {
				return last, fmt.Errorf("%w: repeated id", ErrEventProtocol)
			}
			id = string(value)
		case "event":
			if kind != "" {
				return last, fmt.Errorf("%w: repeated type", ErrEventProtocol)
			}
			kind = string(value)
		case "data":
			if len(data) > 0 {
				data = append(data, '\n')
			}
			data = append(data, value...)
		default:
			return last, fmt.Errorf("%w: unsupported field", ErrEventProtocol)
		}
	}
}

type eventConsumerError struct{ cause error }

func (e *eventConsumerError) Error() string { return e.cause.Error() }
func (e *eventConsumerError) Unwrap() error { return e.cause }
