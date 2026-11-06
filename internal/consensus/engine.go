package consensus

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"example.com/go-block-lattice/internal/ledger"
	"example.com/go-block-lattice/internal/protocol"
)

var ErrBusy = errors.New("consensus capacity reached")
var ErrStale = errors.New("stale or out-of-window consensus message")

type Config struct {
	Store        *ledger.Store
	PrivateKey   ed25519.PrivateKey
	RoundTimeout time.Duration
	MaxPending   int
}

type signingState struct {
	Version        int                    `json:"version"`
	Network        protocol.Hash          `json:"network"`
	Representative protocol.Account       `json:"representative"`
	Height         uint64                 `json:"height"`
	Round          uint32                 `json:"round"`
	Locked         *protocol.Block        `json:"locked,omitempty"`
	LockedRound    int64                  `json:"locked_round"`
	Valid          *protocol.Block        `json:"valid,omitempty"`
	ValidRound     int64                  `json:"valid_round"`
	Proof          []ledger.Vote          `json:"proof,omitempty"`
	Signed         map[string]ledger.Vote `json:"signed"`
	Proposal       *Proposal              `json:"proposal,omitempty"`
}

type futureVotes struct {
	Round uint32
	Votes map[string]ledger.Vote
}

type Engine struct {
	lastBroadcast   time.Time
	broadcastCursor int
	ahead           map[protocol.Account]futureVotes
	mu              sync.Mutex
	cfg             Config
	state           signingState
	pending         map[protocol.Hash]protocol.Block
	votes           map[uint32]map[string]map[protocol.Account]ledger.Vote
	proposals       map[uint32]Proposal
	weights         map[protocol.Account]protocol.Amount
	deadline        time.Time
	subscribers     []func(Message)
	outgoing        []Message
	failure         error
	running         bool
}

func New(cfg Config) (*Engine, error) {
	if cfg.Store == nil {
		return nil, errors.New("consensus requires ledger store")
	}
	if len(cfg.PrivateKey) != 0 && len(cfg.PrivateKey) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid representative private key")
	}
	if cfg.MaxPending == 0 {
		cfg.MaxPending = 4096
	}
	if cfg.MaxPending < 1 || cfg.MaxPending > 65536 {
		return nil, errors.New("max_pending must be 1..65536")
	}
	if cfg.RoundTimeout == 0 {
		cfg.RoundTimeout = 2 * time.Second
	}
	if cfg.RoundTimeout < 100*time.Millisecond || cfg.RoundTimeout > time.Minute {
		return nil, errors.New("round timeout must be 100ms..1m")
	}
	e := &Engine{cfg: cfg, pending: map[protocol.Hash]protocol.Block{}, votes: map[uint32]map[string]map[protocol.Account]ledger.Vote{}, proposals: map[uint32]Proposal{}}
	st := cfg.Store.Status()
	if st.Height == math.MaxUint64 {
		return nil, errors.New("consensus height exhausted")
	}
	rep := protocol.Account("")
	if len(cfg.PrivateKey) > 0 {
		rep = protocol.Address(cfg.PrivateKey.Public().(ed25519.PublicKey))
	}
	e.state = signingState{Version: 1, Network: st.Network, Representative: rep, Height: st.Height + 1, LockedRound: -1, ValidRound: -1, Signed: map[string]ledger.Vote{}}
	data, err := cfg.Store.SigningState()
	if err == nil {
		var saved signingState
		if err = json.Unmarshal(data, &saved); err != nil {
			return nil, fmt.Errorf("read signing state: %w", err)
		}
		if saved.Version != 1 || saved.Network != st.Network || saved.Representative != rep {
			return nil, errors.New("signing state network, version or identity mismatch; do not reuse another representative's data directory")
		}
		if saved.Height > st.Height+1 {
			return nil, errors.New("signing state ahead of ledger; rollback recovery refused")
		}
		if saved.Height == st.Height+1 {
			e.state = saved
		}
	} else if errors.Is(err, ledger.ErrObserverOnly) && len(cfg.PrivateKey) == 0 {
		// Restored ledgers may verify and relay, but never sign with historical identities.
	} else if !errors.Is(err, ledger.ErrNotFound) {
		return nil, err
	}
	if e.state.Signed == nil {
		e.state.Signed = map[string]ledger.Vote{}
	}
	e.weights = cfg.Store.Weights()
	e.ahead = map[protocol.Account]futureVotes{}
	if err = e.validateRecovered(); err != nil {
		return nil, err
	}
	for _, v := range e.state.Signed {
		e.remember(v)
	}
	if e.state.Proposal != nil {
		e.proposals[e.state.Round] = *e.state.Proposal
	}
	e.resetDeadline()
	if err = e.persist(); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *Engine) persist() error {
	if len(e.cfg.PrivateKey) == 0 {
		return nil
	}
	b, err := json.Marshal(e.state)
	if err == nil {
		err = e.cfg.Store.SaveSigningState(b)
	}
	if err != nil {
		e.failure = fmt.Errorf("signing persistence: %w", err)
	}
	return err
}
func (e *Engine) resetDeadline() {
	multiplier := time.Duration(e.state.Round) + 1
	if multiplier > 15 {
		multiplier = 15
	}
	e.deadline = time.Now().Add(e.cfg.RoundTimeout * multiplier)
}
func (e *Engine) Subscribe(fn func(Message)) {
	if fn != nil {
		e.mu.Lock()
		e.subscribers = append(e.subscribers, fn)
		e.mu.Unlock()
	}
}

// flush transfers messages under the lock and calls transports outside it.
func (e *Engine) flush() {
	e.mu.Lock()
	out := e.outgoing
	e.outgoing = nil
	subs := append([]func(Message){}, e.subscribers...)
	e.mu.Unlock()
	for _, m := range out {
		for _, fn := range subs {
			fn(m)
		}
	}
}
func (e *Engine) emit(m Message) { e.outgoing = append(e.outgoing, m) }

func (e *Engine) Submit(b protocol.Block) (protocol.Hash, error) {
	e.mu.Lock()
	id, err := e.submit(b, true)
	if err == nil {
		err = e.drive()
	}
	e.mu.Unlock()
	e.flush()
	return id, err
}
func (e *Engine) submit(b protocol.Block, broadcast bool) (protocol.Hash, error) {
	if e.failure != nil {
		return "", e.failure
	}
	if err := e.cfg.Store.Validate(b); err != nil {
		return "", err
	}
	id := b.ID()
	if _, err := e.cfg.Store.Block(id); err == nil {
		return id, nil
	}
	if _, ok := e.pending[id]; ok {
		return id, nil
	}
	if len(e.pending) >= e.cfg.MaxPending {
		return "", ErrBusy
	}
	e.pending[id] = b
	if broadcast {
		e.emit(Message{Type: "block", Block: &b})
	}
	return id, nil
}
func (e *Engine) Handle(m Message) error {
	e.mu.Lock()
	var err error
	if e.failure != nil {
		err = e.failure
	} else {
		switch m.Type {
		case "block":
			if m.Block == nil {
				err = errors.New("block required")
			} else {
				_, err = e.submit(*m.Block, true)
			}
		case "proposal":
			if m.Proposal == nil {
				err = errors.New("proposal required")
			} else {
				err = e.acceptProposal(*m.Proposal)
			}
		case "vote":
			if m.Vote == nil {
				err = errors.New("vote required")
			} else {
				err = e.acceptVote(*m.Vote)
			}
		case "commit":
			if m.Record == nil {
				err = errors.New("record required")
			} else {
				err = e.commit(*m.Record, true)
			}
		default:
			err = errors.New("unsupported consensus message")
		}
		if err == nil {
			err = e.drive()
		}
	}
	e.mu.Unlock()
	e.flush()
	return err
}
func (e *Engine) Pending() []protocol.Block {
	e.mu.Lock()
	defer e.mu.Unlock()
	ids := make([]protocol.Hash, 0, len(e.pending))
	for id := range e.pending {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]protocol.Block, 0, len(ids))
	for _, id := range ids {
		out = append(out, e.pending[id])
	}
	return out
}
func (e *Engine) Health() map[string]any {
	e.mu.Lock()
	defer e.mu.Unlock()
	var online protocol.Amount
	seen := map[protocol.Account]bool{}
	for _, phases := range e.votes {
		for _, vs := range phases {
			for rep := range vs {
				if !seen[rep] {
					online += e.weights[rep]
					seen[rep] = true
				}
			}
		}
	}
	status := e.cfg.Store.Status()
	errText := ""
	if e.failure != nil {
		errText = e.failure.Error()
	}
	return map[string]any{"ready": e.failure == nil && !status.CapacityReached, "error": errText, "capacity_reached": status.CapacityReached, "height": e.state.Height, "round": e.state.Round, "pending": len(e.pending), "representative": e.state.Representative, "observed_voting_weight": online, "quorum_weight": protocol.Amount(uint64(status.Supply)/3*2 + (uint64(status.Supply)%3*2)/3 + 1)}
}
func (e *Engine) Run(ctx context.Context) error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return errors.New("consensus already running")
	}
	e.running = true
	e.mu.Unlock()
	defer func() { e.mu.Lock(); e.running = false; e.mu.Unlock() }()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			e.mu.Lock()
			err := e.failure
			if err == nil {
				if time.Now().After(e.deadline) {
					err = e.timeout()
				}
				if err == nil {
					err = e.drive()
				}
			}
			if err == nil && time.Since(e.lastBroadcast) >= time.Second {
				e.rebroadcast()
				e.lastBroadcast = time.Now()
			}
			e.mu.Unlock()
			e.flush()
			if err != nil {
				return err
			}
		}
	}
}
func (e *Engine) roundNext() error {
	if e.state.Round == math.MaxUint32 {
		return errors.New("consensus round exhausted")
	}
	return e.advanceRound(e.state.Round + 1)
}
func (e *Engine) advanceRound(round uint32) error {
	if round <= e.state.Round {
		return nil
	}
	e.state.Round = round
	e.state.Proposal = nil
	for r := range e.votes {
		if uint64(r)+2 < uint64(round) {
			delete(e.votes, r)
		}
	}
	for r := range e.proposals {
		if uint64(r)+2 < uint64(round) {
			delete(e.proposals, r)
		}
	}
	for k, v := range e.state.Signed {
		if v.Round < round {
			delete(e.state.Signed, k)
		}
	}
	e.resetDeadline()
	return e.persist()
}
func (e *Engine) timeout() error {
	if !e.hasSigned("prevote") {
		if err := e.signVote("prevote", ""); err != nil {
			return err
		}
	} else if !e.hasSigned("precommit") {
		if err := e.signVote("precommit", ""); err != nil {
			return err
		}
	} else {
		return e.roundNext()
	}
	e.resetDeadline()
	return nil
}

// rebroadcast repairs dropped best-effort gossip without unbounded queues.
func (e *Engine) rebroadcast() {
	if p, ok := e.proposals[e.state.Round]; ok && p.Representative == e.state.Representative {
		copy := p
		e.emit(Message{Type: "proposal", Proposal: &copy})
	}
	for _, v := range e.state.Signed {
		copy := v
		e.emit(Message{Type: "vote", Vote: &copy})
	}
	ids := make([]protocol.Hash, 0, len(e.pending))
	for id := range e.pending {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) == 0 {
		return
	}
	n := len(ids)
	if n > 32 {
		n = 32
	}
	for i := 0; i < n; i++ {
		b := e.pending[ids[(e.broadcastCursor+i)%len(ids)]]
		e.emit(Message{Type: "block", Block: &b})
	}
	e.broadcastCursor = (e.broadcastCursor + n) % len(ids)
}
