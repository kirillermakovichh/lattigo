// Package node owns process configuration and runtime resource lifetimes.
package node

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/p2p"
)

type Config struct {
	DataDir           string   `json:"data_dir"`
	Genesis           string   `json:"genesis"`
	RPCListen         string   `json:"rpc_listen"`
	P2PListen         string   `json:"p2p_listen"`
	Peers             []string `json:"peers"`
	RPCToken          string   `json:"rpc_token"`
	RepresentativeKey string   `json:"representative_key"`
	RoundTimeoutMS    int      `json:"round_timeout_ms"`
	MaxPending        int      `json:"max_pending"`
	MaxPeers          int      `json:"max_peers"`
	AllowRemoteRPC    bool     `json:"allow_remote_rpc"`
	HistoryLimitBytes uint64   `json:"history_limit_bytes"`
}

func (c *Config) defaults() {
	if c.HistoryLimitBytes == 0 {
		c.HistoryLimitBytes = ledger.DefaultMaxHistoryBytes
	}
	if c.DataDir == "" {
		c.DataDir = ".local/node"
	}
	if c.RPCListen == "" {
		c.RPCListen = "127.0.0.1:7070"
	}
	if c.P2PListen == "" {
		c.P2PListen = "127.0.0.1:7071"
	}
	if c.RoundTimeoutMS == 0 {
		c.RoundTimeoutMS = 2000
	}
	if c.MaxPending == 0 {
		c.MaxPending = 1024
	}
	if c.MaxPeers == 0 {
		c.MaxPeers = 32
	}
	if value := os.Getenv("BL_RPC_TOKEN"); value != "" {
		c.RPCToken = value
	}
}

func LoadConfig(path string) (Config, error) {
	var cfg Config
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return cfg, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return cfg, err
	}
	if !info.Mode().IsRegular() {
		return cfg, errors.New("configuration must be a regular file")
	}
	raw, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return cfg, err
	}
	if len(raw) > 1<<20 {
		return cfg, errors.New("configuration exceeds 1 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("decode config: %w", err)
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return cfg, errors.New("configuration must contain one JSON value")
	}
	cfg.defaults()
	absolute, err := filepath.Abs(path)
	if err != nil {
		return cfg, err
	}
	base := filepath.Dir(absolute)
	for _, target := range []*string{&cfg.DataDir, &cfg.Genesis, &cfg.RepresentativeKey} {
		if *target != "" && !filepath.IsAbs(*target) {
			*target = filepath.Join(base, *target)
		}
	}
	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	if c.HistoryLimitBytes != 0 && (c.HistoryLimitBytes < 32<<20 || c.HistoryLimitBytes > 1<<40) {
		return errors.New("history_limit_bytes must be 33554432..1099511627776")
	}
	if c.Genesis == "" {
		return errors.New("genesis file is required")
	}
	if c.DataDir == "" {
		return errors.New("data_dir is required")
	}
	if c.RoundTimeoutMS < 100 || c.RoundTimeoutMS > 60000 {
		return errors.New("round_timeout_ms must be 100..60000")
	}
	if c.MaxPending < 1 || c.MaxPending > 65536 {
		return errors.New("max_pending must be 1..65536")
	}
	if c.MaxPeers < 1 || c.MaxPeers > 128 {
		return errors.New("max_peers must be 1..128")
	}
	if len(c.Peers) > c.MaxPeers {
		return errors.New("configured peers exceed max_peers")
	}
	for _, address := range []string{c.RPCListen, c.P2PListen} {
		host, port, err := net.SplitHostPort(address)
		if err != nil || port == "" {
			return fmt.Errorf("invalid listener %q", address)
		}
		_, portErr := strconv.ParseUint(port, 10, 16) // Zero explicitly requests an ephemeral listener.
		if portErr != nil {
			return fmt.Errorf("listener port must be an unsigned decimal in0..65535: %q", address)
		}
		for _, digit := range port {
			if digit < '0' || digit > '9' {
				return fmt.Errorf("listener port must contain decimal digits: %q", address)
			}
		}
		if host != "" && net.ParseIP(host) == nil && host != "localhost" {
			return errors.New("listeners require an IP literal or localhost")
		}
	}
	host, _, _ := net.SplitHostPort(c.RPCListen)
	ip := net.ParseIP(host)
	loopback := host == "localhost" || (ip != nil && ip.IsLoopback())
	if !loopback && !c.AllowRemoteRPC {
		return errors.New("remote RPC listener requires allow_remote_rpc=true")
	}
	if !loopback && len(c.RPCToken) < 32 {
		return errors.New("remote RPC requires a token with at least 32 characters")
	}
	if c.RPCToken != "" && (len(c.RPCToken) < 16 || strings.ContainsAny(c.RPCToken, "\r\n")) {
		return errors.New("RPC token must contain at least 16 characters without line breaks")
	}
	if c.RPCListen == c.P2PListen {
		return errors.New("RPC and P2P listeners must differ")
	}
	for _, raw := range c.Peers {
		if _, err := p2p.ValidatePeer(raw); err != nil {
			return err
		}
	}
	return nil
}

func loadRepresentative(path string) (ed25519.PrivateKey, error) {
	if path == "" {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("representative key must be a regular file accessible only by its owner")
	}
	if info.Size() > 256 {
		return nil, errors.New("representative key file is too large")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	seed, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("representative key must contain a 32-byte hex Ed25519 seed")
	}
	key := ed25519.NewKeyFromSeed(seed)
	for i := range seed {
		seed[i] = 0
	}
	return key, nil
}

func (c Config) roundTimeout() time.Duration {
	return time.Duration(c.RoundTimeoutMS) * time.Millisecond
}
