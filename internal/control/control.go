// Package control implements explicit local network administration.
package control

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"example.com/go-block-lattice/internal/client"
	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/node"
	"example.com/go-block-lattice/internal/protocol"
)

type peerFlags []string

func (p *peerFlags) String() string     { return strings.Join(*p, ",") }
func (p *peerFlags) Set(s string) error { *p = append(*p, s); return nil }

// RunControl executes local network administration and authenticated backup workflows.
func RunControl(ctx context.Context, args []string, out, errout io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		_, e := fmt.Fprintln(out, "Usage: blctl <genesis|config|token|status|peers|backup|restore|inspect|verify|doctor> [flags]\nUse blctl <command> --help for flags. Runtime secrets are never printed.")
		return e
	}
	if args[0] == "doctor" {
		return runDoctor(ctx, args[1:], out, errout)
	}
	cmd := args[0]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(errout)
	var genesis, dataDir, output, backup, allocations, name, account, balance, representative, rpc, p2p, repKey, tokenFile string
	var workBits uint
	var maxBytes int64
	var historyLimit uint64
	var timeout time.Duration
	var peers peerFlags
	var remote bool
	switch cmd {
	case "token":
		fs.StringVar(&output, "out", "", "exclusive private token output")
	case "genesis":
		fs.StringVar(&name, "name", "", "explicit network name")
		fs.StringVar(&allocations, "allocations", "", "JSON array of allocation objects")
		fs.StringVar(&account, "account", "", "single funded account (alternative to --allocations)")
		fs.StringVar(&balance, "balance", "", "single raw allocation amount")
		fs.StringVar(&representative, "representative", "", "single allocation representative (defaults to account)")
		fs.UintVar(&workBits, "work-bits", 12, "anti-spam work bits (0..24)")
		fs.StringVar(&output, "out", "genesis.json", "exclusive output manifest")
	case "config":
		fs.StringVar(&genesis, "genesis", "genesis.json", "genesis path, resolved relative to output config")
		fs.StringVar(&dataDir, "data-dir", "data", "node data path, resolved relative to output config")
		fs.StringVar(&rpc, "rpc-listen", "127.0.0.1:7070", "RPC listen address")
		fs.StringVar(&p2p, "p2p-listen", "127.0.0.1:7071", "peer listen address")
		fs.StringVar(&repKey, "representative-key", "", "0600 file containing hex representative seed")
		fs.StringVar(&tokenFile, "token-file", "", "0600 token file; otherwise BL_RPC_TOKEN or random token")
		fs.Var(&peers, "peer", "configured peer HTTP origin (repeatable)")
		fs.BoolVar(&remote, "allow-remote-rpc", false, "explicit remote RPC listener opt-in")
		fs.StringVar(&output, "out", "node.json", "exclusive private config output")
	case "status", "peers", "backup":
		fs.StringVar(&rpc, "rpc", "http://127.0.0.1:7070", "RPC origin")
		fs.StringVar(&tokenFile, "token-file", "", "0600 file containing RPC token; otherwise BL_RPC_TOKEN")
		fs.DurationVar(&timeout, "timeout", 2*time.Minute, "request deadline")
		if cmd == "backup" {
			fs.StringVar(&output, "out", "", "exclusive backup output")
			fs.Int64Var(&maxBytes, "max-bytes", 1<<30, "maximum backup bytes")
		}
	case "restore", "inspect", "verify":
		fs.Uint64Var(&historyLimit, "history-limit-bytes", ledger.DefaultMaxHistoryBytes, "configured ledger history byte capacity (32MiB..1TiB)")
		fs.StringVar(&genesis, "genesis", "genesis.json", "network manifest")
		fs.StringVar(&dataDir, "data-dir", "data", "node data directory")
		if cmd == "restore" {
			fs.StringVar(&backup, "backup", "", "backup input; target must be empty")
		}
	default:
		return fmt.Errorf("unknown control command %q", cmd)
	}
	if e := fs.Parse(args[1:]); e != nil {
		if errors.Is(e, flag.ErrHelp) {
			return nil
		}
		return e
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	switch cmd {
	case "token":
		if output == "" {
			return errors.New("--out is required")
		}
		b := make([]byte, 32)
		if _, e := rand.Read(b); e != nil {
			return e
		}
		defer clear(b)
		if e := writeNew(output, func(w io.Writer) error { _, e := fmt.Fprintln(w, hex.EncodeToString(b)); return e }); e != nil {
			return e
		}
		return controlOutput(out, map[string]any{"path": output, "status": "created"})
	case "genesis":
		g := protocol.Genesis{Version: 1, Name: name}
		if workBits > 24 {
			return errors.New("work bits must be 0..24")
		}
		g.WorkBits = uint8(workBits)
		if allocations != "" {
			if account != "" || balance != "" || representative != "" {
				return errors.New("--allocations cannot be combined with single-allocation flags")
			}
			f, e := os.OpenFile(allocations, os.O_RDONLY|syscall.O_NONBLOCK, 0)
			if e != nil {
				return e
			}
			defer f.Close()
			info, e := f.Stat()
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() {
				return errors.New("allocations must be a regular file")
			}
			raw, e := io.ReadAll(io.LimitReader(f, (1<<20)+1))
			if e != nil {
				return e
			}
			if len(raw) > 1<<20 {
				return errors.New("allocations file exceeds1MiB")
			}
			d := json.NewDecoder(bytes.NewReader(raw))
			d.DisallowUnknownFields()
			if e = d.Decode(&g.Allocations); e != nil {
				return e
			}
			if e = d.Decode(new(any)); e != io.EOF {
				return errors.New("trailing allocations JSON")
			}
		} else {
			value, e := protocol.ParseAmount(balance)
			if e != nil {
				return e
			}
			if representative == "" {
				representative = account
			}
			g.Allocations = []protocol.Allocation{{Account: protocol.Account(account), Balance: value, Representative: protocol.Account(representative)}}
		}
		var sum uint64
		for _, a := range g.Allocations {
			if math.MaxUint64-sum < uint64(a.Balance) {
				return errors.New("allocation supply overflow")
			}
			sum += uint64(a.Balance)
		}
		g.Supply = protocol.Amount(sum)
		if e := g.Validate(); e != nil {
			return e
		}
		if e := writeJSONNew(output, g); e != nil {
			return e
		}
		return controlOutput(out, map[string]any{"path": output, "network": g.ID(), "supply": g.Supply})
	case "config":
		token := os.Getenv("BL_RPC_TOKEN")
		if tokenFile != "" {
			b, e := controlSecret(tokenFile)
			if e != nil {
				return e
			}
			token = string(b)
			clear(b)
		}
		if token == "" {
			b := make([]byte, 32)
			if _, e := rand.Read(b); e != nil {
				return e
			}
			token = hex.EncodeToString(b)
			clear(b)
		}
		v := node.Config{DataDir: dataDir, Genesis: genesis, RPCListen: rpc, P2PListen: p2p, Peers: []string(peers), RPCToken: token, RepresentativeKey: repKey, RoundTimeoutMS: 2000, MaxPending: 4096, MaxPeers: 32, AllowRemoteRPC: remote, HistoryLimitBytes: ledger.DefaultMaxHistoryBytes}
		if e := v.Validate(); e != nil {
			return fmt.Errorf("node configuration: %w", e)
		}
		if e := writeJSONNew(output, v); e != nil {
			return e
		}
		return controlOutput(out, map[string]any{"path": output, "status": "created", "rpc_token": "stored in private config; not printed"})
	case "status", "peers", "backup":
		if timeout <= 0 {
			return errors.New("timeout must be positive")
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		token := os.Getenv("BL_RPC_TOKEN")
		if tokenFile != "" {
			b, e := controlSecret(tokenFile)
			if e != nil {
				return e
			}
			token = string(b)
			clear(b)
		}
		c, e := client.New(rpc, token)
		if e != nil {
			return e
		}
		if cmd == "peers" {
			peers, err := c.Peers(ctx)
			if err != nil {
				return err
			}
			return controlOutput(out, peers)
		}
		if cmd == "status" {
			s, e := c.Status(ctx)
			if e != nil {
				return e
			}
			return controlOutput(out, s)
		}
		if output == "" {
			return errors.New("--out is required")
		}
		if maxBytes <= 0 || maxBytes == math.MaxInt64 {
			return errors.New("max-bytes must be positive and below int64 maximum")
		}
		if e = writeNew(output, func(w io.Writer) error { return c.Backup(ctx, w, maxBytes) }); e != nil {
			return e
		}
		return controlOutput(out, map[string]any{"path": output, "status": "complete"})
	case "restore", "inspect", "verify":
		g, e := protocol.LoadGenesis(genesis)
		if e != nil {
			return e
		}
		if cmd == "restore" {
			if backup == "" {
				return errors.New("--backup is required")
			}
			f, e := os.OpenFile(backup, os.O_RDONLY|syscall.O_NONBLOCK, 0)
			if e != nil {
				return e
			}
			defer f.Close()
			info, e := f.Stat()
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() {
				return errors.New("backup input must be a regular file")
			}
			if e = ledger.RestoreWithOptions(dataDir, f, g, ledger.Options{MaxHistoryBytes: historyLimit}); e != nil {
				return e
			}
			return controlOutput(out, map[string]any{"data_dir": dataDir, "status": "restored_observer_only"})
		}
		st, e := ledger.OpenExisting(dataDir, g, ledger.Options{MaxHistoryBytes: historyLimit})
		if e != nil {
			return e
		}
		defer st.Close()
		return controlOutput(out, st.Status())
	}
	return errors.New("unsupported control operation")
}

func controlOutput(out io.Writer, v any) error {
	e := json.NewEncoder(out)
	e.SetIndent("", "  ")
	return e.Encode(v)
}
func writeJSONNew(path string, v any) error {
	return writeNew(path, func(w io.Writer) error { return controlOutput(w, v) })
}
func writeNew(path string, write func(io.Writer) error) error {
	dir := filepath.Dir(path)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".bl-output-*")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e != nil {
		f.Close()
		return e
	}
	if e = write(f); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Link(name, path); e != nil {
		return fmt.Errorf("create output without overwrite: %w", e)
	}
	d, e := os.Open(dir)
	if e != nil {
		return e
	}
	defer d.Close()
	return d.Sync()
}
func controlSecret(path string) ([]byte, error) {
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > 16<<10 {
		return nil, errors.New("token must be a private regular file below 16 KiB")
	}
	b, e := io.ReadAll(io.LimitReader(f, 16<<10+1))
	if e != nil {
		return nil, e
	}
	if len(b) > 16<<10 {
		return nil, errors.New("token file exceeds limit")
	}
	return []byte(strings.TrimSpace(string(b))), nil
}
