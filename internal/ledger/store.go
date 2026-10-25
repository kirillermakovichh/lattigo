package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"

	"example.com/go-block-lattice/internal/protocol"
	"example.com/go-block-lattice/internal/storage"
)

const DefaultMaxHistoryBytes uint64 = 256 << 20

type Options struct{ MaxHistoryBytes uint64 }

func normalizeOptions(options Options) (Options, error) {
	if options.MaxHistoryBytes == 0 {
		options.MaxHistoryBytes = DefaultMaxHistoryBytes
	}
	if options.MaxHistoryBytes < 32<<20 || options.MaxHistoryBytes > 1<<40 {
		return options, errors.New("history limit must be32MiB..1TiB")
	}
	return options, nil
}

type Store struct {
	historyBytes uint64
	historyLimit uint64
	mu           sync.RWMutex
	state        *state
	journal      *storage.Journal
	closed       bool
	failed       error
	observerOnly bool
}

func Open(directory string, g protocol.Genesis) (*Store, error) {
	return OpenWithOptions(directory, g, Options{})
}

// OpenExisting verifies an existing journal without creating a ledger or
// repairing an incomplete tail. The exclusive data lock is still acquired.
func OpenExisting(directory string, g protocol.Genesis, options Options) (*Store, error) {
	info, err := os.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("inspection requires a real data directory")
	}
	info, err = os.Lstat(filepath.Join(directory, "ledger.wal"))
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("inspection requires an existing regular journal")
	}
	return open(directory, g, true, options)
}

func OpenWithOptions(directory string, g protocol.Genesis, options Options) (*Store, error) {
	return open(directory, g, false, options)
}
func open(directory string, g protocol.Genesis, strict bool, options Options) (*Store, error) {
	options, e := normalizeOptions(options)
	if e != nil {
		return nil, e
	}
	if e := g.Validate(); e != nil {
		return nil, e
	}
	g.Allocations = append([]protocol.Allocation(nil), g.Allocations...)
	sort.Slice(g.Allocations, func(i, j int) bool { return g.Allocations[i].Account < g.Allocations[j].Account })
	header, e := json.Marshal(g)
	if e != nil {
		return nil, e
	}
	s := &Store{state: newState(g), historyLimit: options.MaxHistoryBytes}
	if _, err := os.Stat(filepath.Join(directory, "RESTORED")); err == nil {
		s.observerOnly = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	apply := func(p []byte) error {
		if uint64(len(p)) > s.historyLimit-s.historyBytes {
			return ErrCapacity
		}
		var r Record
		if e := json.Unmarshal(p, &r); e != nil {
			return e
		}
		if e := s.state.validateRecord(r); e != nil {
			return e
		}
		s.state.apply(r, len(p))
		s.historyBytes += uint64(len(p))
		return nil
	}
	if strict {
		s.journal, e = storage.OpenStrict(directory, header, apply)
	} else {
		s.journal, e = storage.Open(directory, header, apply)
	}
	if e != nil {
		return nil, e
	}
	return s, nil
}
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.journal.Close()
}
func (s *Store) Genesis() protocol.Genesis {
	s.mu.RLock()
	defer s.mu.RUnlock()
	g := s.state.genesis
	g.Allocations = append([]protocol.Allocation(nil), g.Allocations...)
	return g
}
func (s *Store) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a := s.state
	return Status{HistoryBytes: s.historyBytes, HistoryLimitBytes: s.historyLimit, CapacityReached: s.historyBytes+storage.MaxFrame > s.historyLimit, Network: a.genesis.ID(), Height: uint64(len(a.records)), Tip: a.tip, Supply: a.genesis.Supply, Accounts: len(a.accounts), Receivables: len(a.receivables)}
}
func (s *Store) Account(a protocol.Account) (AccountState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return AccountState{}, ErrClosed
	}
	v, ok := s.state.accounts[a]
	if !ok {
		return AccountState{}, ErrNotFound
	}
	return v, nil
}
func page(offset, limit, n int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if offset > n {
		offset = n
	}
	if limit < 1 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	end := offset + limit
	if end > n {
		end = n
	}
	return offset, end
}
func (s *Store) Receivables(a protocol.Account, offset, limit int) []Receivable {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v := make([]Receivable, 0)
	for _, r := range s.state.receivables {
		if r.To == a {
			v = append(v, r)
		}
	}
	sort.Slice(v, func(i, j int) bool { return v[i].Source < v[j].Source })
	start, end := page(offset, limit, len(v))
	return v[start:end]
}
func copyRecord(r Record) Record { r.Votes = append([]Vote(nil), r.Votes...); return r }

// History returns a reverse chronological page bounded by count and 16MiB of
// serialized record payloads. A single valid record is always returned in full.
func (s *Store) History(a protocol.Account, offset, limit int) []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if offset < 0 {
		offset = 0
	}
	if limit < 1 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	out := make([]Record, 0)
	seen := 0
	bytes := uint64(2)
	for i := len(s.state.records) - 1; i >= 0; i-- {
		r := s.state.records[i]
		if r.Block.Account != a {
			continue
		}
		if seen < offset {
			seen++
			continue
		}
		size := uint64(s.state.recordSizes[i]) + 1
		if len(out) > 0 && bytes+size > 16<<20 {
			break
		}
		out = append(out, copyRecord(r))
		bytes += size
		if len(out) >= limit {
			break
		}
	}
	return out
}

// Records returns an ascending, byte-bounded certified bootstrap page.
func (s *Store) Records(after uint64, limit int) []Record {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if after >= uint64(len(s.state.records)) {
		return []Record{}
	}
	if limit < 1 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	out := make([]Record, 0)
	bytes := uint64(2)
	for i := int(after); i < len(s.state.records) && len(out) < limit; i++ {
		size := uint64(s.state.recordSizes[i]) + 1
		if len(out) > 0 && bytes+size > 28<<20 {
			break
		}
		out = append(out, copyRecord(s.state.records[i]))
		bytes += size
	}
	return out
}
func (s *Store) Block(h protocol.Hash) (protocol.Block, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return protocol.Block{}, ErrClosed
	}
	b, ok := s.state.blocks[h]
	if !ok {
		return protocol.Block{}, ErrNotFound
	}
	return b, nil
}
func (s *Store) Weights() map[protocol.Account]protocol.Amount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state.weights()
}
func (s *Store) Validate(b protocol.Block) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrClosed
	}
	if s.failed != nil {
		return s.failed
	}

	if err := s.state.validate(b); err != nil {
		return err
	}
	if _, exists := s.state.blocks[b.ID()]; !exists && s.historyBytes+storage.MaxFrame > s.historyLimit {
		return ErrCapacity
	}
	return nil
}
func (s *Store) Commit(r Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	if s.failed != nil {
		return s.failed
	}
	if r.Height <= uint64(len(s.state.records)) && r.Height > 0 {
		existing := s.state.records[r.Height-1]
		if existing.ID() == r.ID() {
			return nil
		}
		return fmt.Errorf("%w: confirmed record cannot be replaced", ErrConflict)
	}
	r = copyRecord(r)
	if e := s.state.validateRecord(r); e != nil {
		return e
	}
	p, e := json.Marshal(r)
	if e != nil {
		return e
	}
	if uint64(len(p)) > s.historyLimit-s.historyBytes {
		s.failed = ErrCapacity
		return ErrCapacity
	}
	if e = s.journal.Append(p); e != nil {
		s.failed = e
		return e
	}
	s.state.apply(r, len(p))
	s.historyBytes += uint64(len(p))
	return nil
}
func (s *Store) SaveSigningState(b []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.observerOnly {
		return ErrObserverOnly
	}
	if s.closed {
		return ErrClosed
	}
	if s.failed != nil {
		return s.failed
	}
	if e := s.journal.SaveSigning(b); e != nil {
		s.failed = e
		return e
	}
	return nil
}
func (s *Store) SigningState() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.observerOnly {
		return nil, ErrObserverOnly
	}
	if s.closed {
		return nil, ErrClosed
	}
	v, e := s.journal.Signing()
	if errors.Is(e, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return v, e
}
func (s *Store) Backup(w io.Writer) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return ErrClosed
	}
	return s.journal.Backup(w)
}

// Restore imports a complete journal into an empty directory and independently replays
// every certificate before publishing it. Signing state is deliberately never restored.
func Restore(directory string, src io.Reader, g protocol.Genesis) error {
	return RestoreWithOptions(directory, src, g, Options{})
}

// RestoreWithOptions permits explicitly provisioned history capacity during import.
func RestoreWithOptions(directory string, src io.Reader, g protocol.Genesis, options Options) error {
	options, e := normalizeOptions(options)
	if e != nil {
		return e
	}
	if info, err := os.Lstat(directory); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("restore target must be a real directory")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if e := g.Validate(); e != nil {
		return e
	}
	if e := os.MkdirAll(filepath.Dir(directory), 0700); e != nil {
		return e
	}
	if entries, e := os.ReadDir(directory); e == nil && len(entries) > 0 {
		return errors.New("restore target must be empty")
	} else if e != nil && !errors.Is(e, os.ErrNotExist) {
		return e
	}
	stage, e := os.MkdirTemp(filepath.Dir(directory), ".restore-*")
	if e != nil {
		return e
	}
	defer os.RemoveAll(stage)
	f, e := os.OpenFile(filepath.Join(stage, "ledger.wal"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	// Valid records contain signatures larger than their48-byte frame overhead.
	// Twice the history budget plus1MiB for genesis bounds every valid archive.
	backupLimit := int64(options.MaxHistoryBytes)*2 + (1 << 20)
	var copied int64
	copied, e = io.Copy(f, io.LimitReader(src, backupLimit+1))
	if e == nil && copied > backupLimit {
		e = errors.New("backup exceeds configured history and framing budget")
	}
	if e == nil {
		e = f.Sync()
	}
	e = errors.Join(e, f.Close())
	if e != nil {
		return e
	}
	st, e := open(stage, g, true, options)
	if e != nil {
		return fmt.Errorf("invalid backup: %w", e)
	}
	if e = st.Close(); e != nil {
		return e
	}
	// The same LOCK used by Open serializes restore against an active node.
	if e = os.Mkdir(directory, 0700); e != nil && !errors.Is(e, os.ErrExist) {
		return e
	}
	lock, e := os.OpenFile(filepath.Join(directory, "LOCK"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return fmt.Errorf("restore target locked: %w", e)
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	entries, e := os.ReadDir(directory)
	if e != nil {
		return e
	}
	for _, entry := range entries {
		if entry.Name() != "LOCK" {
			return errors.New("restore target became nonempty")
		}
	}
	lockInfo, e := lock.Stat()
	if e != nil {
		return e
	}
	currentLock, e := os.Lstat(filepath.Join(directory, "LOCK"))
	if e != nil {
		return e
	}
	if !currentLock.Mode().IsRegular() || !os.SameFile(lockInfo, currentLock) {
		return errors.New("restore directory lock changed before publication")
	}
	if e = storage.AtomicWrite(directory, "RESTORED", []byte("observer-only backup restore\n")); e != nil {
		return e
	}
	if e = os.Rename(filepath.Join(stage, "ledger.wal"), filepath.Join(directory, "ledger.wal")); e != nil {
		return e
	}
	return storage.SyncDirectory(directory)
}
