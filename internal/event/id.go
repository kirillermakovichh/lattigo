// Package event defines network-scoped durable stream positions.
package event

import (
	"errors"
	"example.com/go-block-lattice/internal/protocol"
	"strconv"
	"strings"
)

func ID(network protocol.Hash, height uint64) string {
	return string(network) + ":" + strconv.FormatUint(height, 10)
}
func ParseID(value string, network protocol.Hash) (uint64, error) {
	if len(value) > 85 {
		return 0, errors.New("event identifier exceeds limit")
	}
	prefix, raw, ok := strings.Cut(value, ":")
	if !ok || protocol.Hash(prefix) != network {
		return 0, errors.New("event identifier belongs to another network")
	}
	if _, err := network.Bytes(); err != nil {
		return 0, err
	}
	height, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || strconv.FormatUint(height, 10) != raw {
		return 0, errors.New("noncanonical event height")
	}
	return height, nil
}
