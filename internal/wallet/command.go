package wallet

import (
	"bytes"
	"context"
	"crypto/ed25519"
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
	"example.com/go-block-lattice/internal/protocol"
)

type commandFlags struct {
	path, passwordFile, rpc, tokenFile, out, expectedNetwork, accountName string
	index                                                                 uint64
	timeout                                                               time.Duration
}

func (f *commandFlags) add(fs *flag.FlagSet) {
	fs.StringVar(&f.path, "wallet", "wallet.json", "encrypted wallet path")
	fs.StringVar(&f.passwordFile, "password-file", "", "0600 file containing passphrase; otherwise BL_WALLET_PASSWORD")
	fs.StringVar(&f.rpc, "rpc", envDefault("BL_RPC", "http://127.0.0.1:7070"), "RPC origin")
	fs.StringVar(&f.tokenFile, "token-file", "", "0600 file containing RPC token; otherwise BL_RPC_TOKEN")
	fs.StringVar(&f.out, "out", "", "exclusive output file")
	fs.StringVar(&f.expectedNetwork, "expected-network", "", "optional pinned genesis hash for online payments")
	fs.StringVar(&f.accountName, "account-name", "", "named local account instead of --index")
	fs.Uint64Var(&f.index, "index", 0, "deterministic account index")
	fs.DurationVar(&f.timeout, "timeout", 30*time.Second, "operation timeout")
}
func envDefault(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
func secret(file, environment string) ([]byte, error) {
	if file != "" {
		b, e := ReadSecret(file)
		if e != nil {
			return nil, e
		}
		return []byte(strings.TrimRight(string(b), "\r\n")), nil
	}
	value := os.Getenv(environment)
	if value == "" {
		return nil, fmt.Errorf("set %s or provide the corresponding secret file", environment)
	}
	return []byte(value), nil
}
func (f commandFlags) open() (*Wallet, error) {
	p, e := secret(f.passwordFile, "BL_WALLET_PASSWORD")
	if e != nil {
		return nil, e
	}
	defer clear(p)
	return Open(f.path, p)
}
func (f commandFlags) client() (*client.Client, error) {
	token := os.Getenv("BL_RPC_TOKEN")
	if f.tokenFile != "" {
		b, e := secret(f.tokenFile, "BL_RPC_TOKEN")
		if e != nil {
			return nil, e
		}
		token = string(b)
		clear(b)
	}
	return client.New(f.rpc, token)
}
func output(w io.Writer, v any) error {
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	return e.Encode(v)
}
func keyAccount(w *Wallet, index uint64) (protocol.Account, error) {
	k, e := w.PrivateKey(index)
	if e != nil {
		return "", e
	}
	defer clear(k)
	return protocol.Address(k.Public().(ed25519.PublicKey)), nil
}

// RunCommand executes the wallet CLI. Errors never include secret values.
func RunCommand(ctx context.Context, args []string, out, errout io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		_, e := fmt.Fprintln(out, "Usage: blwallet <create|restore|backup|password|contacts|names|outbox|request|accounts|export-key|send|receive|delegate|sign|submit|status|account|receivables|history|block> [flags]\nSecrets: BL_WALLET_PASSWORD / --password-file, BL_RPC_TOKEN / --token-file.\nUse blwallet <command> --help for command flags.")
		return e
	}
	if args[0] == "request" {
		return requestCommand(ctx, args[1:], out, errout)
	}
	if args[0] == "outbox" {
		return outboxCommand(ctx, args[1:], out, errout)
	}
	if args[0] == "names" {
		return namesCommand(args[1:], out, errout)
	}
	if args[0] == "contacts" {
		return contactsCommand(args[1:], out, errout)
	}
	if args[0] == "password" {
		return passwordCommand(ctx, args[1:], out, errout)
	}
	command := args[0]
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(errout)
	var f commandFlags
	f.add(fs)
	var seedFile, backup, address, to, amount, source, representative, kind, previous, balance, network, link, file, contact string
	var count, offset, limit int
	var workBits uint
	var wait bool
	switch command {
	case "create":
		fs.StringVar(&seedFile, "seed-file", "", "optional 0600 file containing a hex master seed")
	case "restore":
		fs.StringVar(&backup, "backup", "", "encrypted backup file")
	case "backup", "export-key":
	case "accounts":
		fs.IntVar(&count, "count", 1, "number of accounts (1..1000)")
	case "send":
		fs.StringVar(&contact, "contact", "", "named recipient from the private address book")
		fs.StringVar(&to, "to", "", "destination account")
		fs.StringVar(&amount, "amount", "", "positive raw amount")
		fs.BoolVar(&wait, "wait", false, "wait until confirmed")
	case "receive":
		fs.StringVar(&source, "source", "", "source send block hash")
		fs.StringVar(&representative, "representative", "", "representative for a new account")
		fs.BoolVar(&wait, "wait", false, "wait until confirmed")
	case "delegate":
		fs.StringVar(&representative, "representative", "", "new representative")
		fs.BoolVar(&wait, "wait", false, "wait until confirmed")
	case "sign":
		fs.StringVar(&kind, "kind", "", "send, receive or change")
		fs.StringVar(&previous, "previous", "", "previous account frontier; empty for opening")
		fs.StringVar(&balance, "balance", "", "resulting raw balance")
		fs.StringVar(&network, "network", "", "genesis network hash")
		fs.StringVar(&representative, "representative", "", "representative account")
		fs.StringVar(&link, "link", "", "destination account or receive source hash")
		fs.UintVar(&workBits, "work-bits", 12, "network proof-of-work bits (0..24)")
	case "submit":
		fs.StringVar(&file, "file", "", "signed block JSON file")
		fs.BoolVar(&wait, "wait", false, "wait until confirmed")
	case "status":
	case "account", "receivables", "history":
		fs.StringVar(&address, "address", "", "account address; defaults to wallet account")
		fs.IntVar(&offset, "offset", 0, "pagination offset")
		fs.IntVar(&limit, "limit", 100, "page limit (1..1000)")
	case "block":
		fs.StringVar(&source, "hash", "", "block hash")
	default:
		return fmt.Errorf("unknown wallet command %q", command)
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
	if err := f.resolveAccountName(fs); err != nil {
		return err
	}
	if f.timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	if contact != "" {
		account, network, err := contactDestination(f, contact, to)
		if err != nil {
			return err
		}
		to = string(account)
		f.expectedNetwork = string(network)
	}
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	switch command {
	case "create":
		password, e := secret(f.passwordFile, "BL_WALLET_PASSWORD")
		if e != nil {
			return e
		}
		defer clear(password)
		var w *Wallet
		if seedFile != "" {
			data, e := ReadSecret(seedFile)
			if e != nil {
				return e
			}
			seed, e := hex.DecodeString(strings.TrimSpace(string(data)))
			clear(data)
			if e != nil {
				return errors.New("seed file must contain 64 hex characters")
			}
			defer clear(seed)
			w, e = Import(f.path, seed, password)
			if e != nil {
				return e
			}
		} else {
			w, e = Create(f.path, password)
			if e != nil {
				return e
			}
		}
		defer w.Close()
		a, e := keyAccount(w, f.index)
		if e != nil {
			return e
		}
		return output(out, map[string]any{"wallet": f.path, "index": f.index, "account": a})
	case "restore", "backup":
		p, e := secret(f.passwordFile, "BL_WALLET_PASSWORD")
		if e != nil {
			return e
		}
		defer clear(p)
		if command == "restore" {
			if backup == "" {
				return errors.New("--backup is required")
			}
			e = Restore(backup, f.path, p)
		} else {
			if f.out == "" {
				return errors.New("--out is required")
			}
			e = Backup(f.path, f.out, p)
		}
		if e != nil {
			return e
		}
		return output(out, map[string]any{"status": "complete", "operation": command})
	case "accounts", "export-key", "sign":
		w, e := f.open()
		if e != nil {
			return e
		}
		defer w.Close()
		if command == "export-key" {
			if f.out == "" {
				return errors.New("--out is required")
			}
			if e = w.ExportAccountSeed(f.out, f.index); e != nil {
				return e
			}
			return output(out, map[string]any{"path": f.out, "index": f.index})
		}
		if command == "accounts" {
			if count < 1 || count > 1000 || uint64(count-1) > math.MaxUint64-f.index {
				return errors.New("invalid account range")
			}
			rows := make([]map[string]any, 0, count)
			for i := 0; i < count; i++ {
				a, e := keyAccount(w, f.index+uint64(i))
				if e != nil {
					return e
				}
				rows = append(rows, map[string]any{"index": f.index + uint64(i), "account": a})
			}
			return output(out, rows)
		}
		value, e := protocol.ParseAmount(balance)
		if e != nil {
			return e
		}
		if workBits > 24 {
			return errors.New("work bits must be 0..24")
		}
		a, e := keyAccount(w, f.index)
		if e != nil {
			return e
		}
		b := protocol.Block{Version: 1, Network: protocol.Hash(network), Kind: kind, Account: a, Previous: protocol.Hash(previous), Representative: protocol.Account(representative), Balance: value, Link: link}
		if e = signWork(ctx, w, f.index, &b, uint8(workBits)); e != nil {
			return e
		}
		return writeBlock(out, f.out, b)
	}
	c, e := f.client()
	if e != nil {
		return e
	}
	switch command {
	case "status":
		s, e := c.Status(ctx)
		if e != nil {
			return e
		}
		return output(out, s)
	case "block":
		b, e := c.Block(ctx, protocol.Hash(source))
		if e != nil {
			return e
		}
		return output(out, b)
	case "submit":
		if file == "" {
			return errors.New("--file is required")
		}
		b, e := loadBlock(file)
		if e != nil {
			return e
		}
		return submit(ctx, c, b, wait, out)
	case "account", "receivables", "history":
		if offset < 0 || limit < 1 || limit > 1000 {
			return errors.New("invalid pagination range")
		}
		a := protocol.Account(address)
		if a == "" {
			w, e := f.open()
			if e != nil {
				return e
			}
			a, e = keyAccount(w, f.index)
			w.Close()
			if e != nil {
				return e
			}
		}
		if _, e = a.PublicKey(); e != nil {
			return e
		}
		if command == "account" {
			s, e := c.Account(ctx, a)
			if e != nil {
				return e
			}
			return output(out, s)
		}
		if command == "receivables" {
			s, e := c.Receivables(ctx, a, offset, limit)
			if e != nil {
				return e
			}
			return output(out, s)
		}
		s, e := c.History(ctx, a, offset, limit)
		if e != nil {
			return e
		}
		return output(out, s)
	}
	w, e := f.open()
	if e != nil {
		return e
	}
	defer w.Close()
	a, e := keyAccount(w, f.index)
	if e != nil {
		return e
	}
	// Resolve aliases so commands for the same wallet share one outbox and lock.
	f.path, e = filepath.EvalSymlinks(f.path)
	if e != nil {
		return e
	}
	accountLock, e := lockAccount(f.path, f.index)
	if e != nil {
		return e
	}
	defer accountLock.Close() // Closing the descriptor releases Flock, including on process exit.
	// A durable outbox prevents an uncertain prior submission from being silently replaced.
	pending := fmt.Sprintf("%s.pending-%d.json", f.path, f.index)
	if existing, e := loadBlock(pending); e == nil {
		if _, e = c.Block(ctx, existing.ID()); e != nil {
			return fmt.Errorf("unresolved signed block %s in %s: inspect or resubmit that file before creating another block", existing.ID(), pending)
		}
		if e = os.Remove(pending); e != nil {
			return e
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	g, e := c.Genesis(ctx)
	if e != nil {
		return e
	}
	status, e := c.Status(ctx)
	if e != nil {
		return e
	}
	if f.expectedNetwork != "" && g.ID() != protocol.Hash(f.expectedNetwork) {
		return errors.New("RPC network differs from pinned --expected-network")
	}
	if status.Network != g.ID() {
		return errors.New("RPC genesis does not match status network")
	}
	state, e := c.Account(ctx, a)
	if e != nil && !(command == "receive" && client.IsNotFound(e)) {
		return e
	}
	if e == nil {
		if e = verifyAccountState(ctx, c, g, state); e != nil {
			return e
		}
	}
	b := protocol.Block{Version: 1, Network: g.ID(), Kind: command, Account: a, Previous: state.Frontier, Representative: state.Representative, Balance: state.Balance}
	switch command {
	case "send":
		value, e := protocol.ParseAmount(amount)
		if e != nil {
			return e
		}
		if value == 0 || value > state.Balance {
			return errors.New("send amount must be positive and no greater than confirmed balance")
		}
		if _, e = protocol.Account(to).PublicKey(); e != nil {
			return e
		}
		b.Balance -= value
		b.Link = to
	case "delegate":
		b.Kind = "change"
		b.Representative = protocol.Account(representative)
	case "receive":
		if source == "" {
			return errors.New("--source is required")
		}
		// Validate the actual source block; this also supports sources beyond one RPC page.
		sending, e := c.Block(ctx, protocol.Hash(source))
		if e != nil {
			return e
		}
		if sending.Kind != "send" || sending.Link != string(a) || sending.Network != g.ID() {
			return errors.New("source is not a confirmed send to this account on this network")
		}
		var amount protocol.Amount
		for offset := 0; ; offset += 1000 {
			rows, e := c.Receivables(ctx, a, offset, 1000)
			if e != nil {
				return e
			}
			for _, r := range rows {
				if r.Source == protocol.Hash(source) {
					amount = r.Amount
				}
			}
			if amount != 0 || len(rows) < 1000 {
				break
			}
			if offset >= 1_000_000 {
				return errors.New("receivable search limit exceeded; use offline signing")
			}
		}
		if amount == 0 {
			return errors.New("source is not receivable; it may already be consumed")
		}
		if uint64(amount) > math.MaxUint64-uint64(state.Balance) {
			return errors.New("receive amount overflows balance")
		}
		b.Balance += amount
		b.Link = source
		if b.Representative == "" {
			if representative == "" {
				b.Representative = a
			} else {
				b.Representative = protocol.Account(representative)
			}
		} else if representative != "" {
			return errors.New("--representative is only accepted when opening an account; use delegate otherwise")
		}
	default:
		return errors.New("unsupported transaction command")
	}
	if e = signWork(ctx, w, f.index, &b, g.WorkBits); e != nil {
		return e
	}
	encoded, e := json.MarshalIndent(b, "", "  ")
	if e != nil {
		return e
	}
	if e = WritePrivate(pending, append(encoded, '\n')); e != nil {
		return fmt.Errorf("reserve durable signed block: %w", e)
	}
	if f.out != "" {
		if e = WritePrivate(f.out, append(encoded, '\n')); e != nil {
			return fmt.Errorf("block remains in %s: %w", pending, e)
		}
	}
	return submit(ctx, c, b, wait, out)
}

func signWork(ctx context.Context, w *Wallet, index uint64, b *protocol.Block, bits uint8) error {
	key, e := w.PrivateKey(index)
	if e != nil {
		return e
	}
	defer clear(key)
	if e = b.Sign(key); e != nil {
		return e
	}
	return b.GenerateWork(ctx, bits)
}
func writeBlock(out io.Writer, path string, b protocol.Block) error {
	if path != "" {
		data, e := json.MarshalIndent(b, "", "  ")
		if e != nil {
			return e
		}
		if e = WritePrivate(path, append(data, '\n')); e != nil {
			return e
		}
		return output(out, map[string]any{"hash": b.ID(), "path": path})
	}
	return output(out, b)
}
func loadBlock(path string) (protocol.Block, error) {
	var b protocol.Block
	f, e := os.OpenFile(filepath.Clean(path), os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if e != nil {
		return b, e
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil {
		return b, e
	}
	if !info.Mode().IsRegular() {
		return b, errors.New("signed block input must be a regular file")
	}
	data, e := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if e != nil {
		return b, e
	}
	if len(data) > 64<<10 {
		return b, errors.New("signed block file exceeds64KiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if e = d.Decode(&b); e != nil {
		return b, e
	}
	if e = d.Decode(new(any)); e != io.EOF {
		return b, errors.New("trailing block JSON")
	}
	return b, b.Verify()
}
func submit(ctx context.Context, c *client.Client, b protocol.Block, wait bool, out io.Writer) error {
	result, e := c.Submit(ctx, b)
	if e != nil {
		return e
	}
	if wait && result.Status != "confirmed" {
		if e = c.Wait(ctx, b.ID()); e != nil {
			return fmt.Errorf("block %s was admitted; confirmation wait ended: %w; query its hash before retry", b.ID(), e)
		}
		result.Status = "confirmed"
	}
	return output(out, result)
}

// Compare RPC balance metadata with the owner's signed frontier (or explicit genesis).
// This prevents a dishonest RPC from making a requested transfer deduct a different amount.
func verifyAccountState(ctx context.Context, c *client.Client, g protocol.Genesis, state client.AccountState) error {
	if state.Frontier == "" {
		for _, a := range g.Allocations {
			if a.Account == state.Account && a.Balance == state.Balance && a.Representative == state.Representative {
				return nil
			}
		}
		return errors.New("RPC genesis account metadata is inconsistent")
	}
	b, err := c.Block(ctx, state.Frontier)
	if err != nil {
		return err
	}
	if b.Network != g.ID() || b.Account != state.Account || b.Balance != state.Balance || b.Representative != state.Representative {
		return errors.New("RPC account metadata differs from its signed frontier")
	}
	return nil
}
