# Implementation contract

Status: historical cross-agent coordination contract. The implemented protocol and
runtime contracts are maintained in protocol.md, architecture.md and runtime.md;
actual Go declarations include subsequent capacity and restore-safety extensions.

Accepted implementation choices for this run under the user's implementation mandate:
independent protocol, Go standard library, uint64 raw amounts encoded as decimal strings,
SHA-256, standard Ed25519, checksummed public-key addresses, feeless PoW admission,
append-only durable storage, explicit genesis and weighted two-phase BFT confirmation.
This is Nano-inspired, not wire-compatible. Total confirmation order is deliberately
introduced to make dynamic delegated weights unambiguous between confirmed transitions.

## Frozen cross-agent Go API

Module: example.com/go-block-lattice. Do not change it during this run.

### internal/protocol (core agent)

```
type Hash string // lowercase 64-char hex; zero hash is ""
type Account string // bl_ + hex(pub32) + hex(sha256("BL/address/v1"||pub)[:4])
type Amount uint64 // JSON decimal string only
func Address(ed25519.PublicKey) Account
func (Account) PublicKey() (ed25519.PublicKey,error)
func ParseAmount(string) (Amount,error)
type Block struct {
 Version uint8 `json:"version"`
 Network Hash `json:"network"`
 Kind string `json:"kind"` // send, receive, change
 Account Account `json:"account"`
 Previous Hash `json:"previous"`
 Representative Account `json:"representative"`
 Balance Amount `json:"balance"`
 Link string `json:"link"` // send: destination address; receive: source hash; change: empty
 Work uint64 `json:"work"`
 Signature string `json:"signature"` // hex Ed25519
}
func (Block) ID() Hash // sha256(canonical domain-separated bytes); signature/work excluded
func (*Block) Sign(ed25519.PrivateKey) error
func (Block) Verify() error
func (Block) CheckWork(bits uint8) bool
func (*Block) GenerateWork(context.Context,bits uint8) error
// work: leading zero bits of SHA256("BL/work/v1" || decoded ID || bigendian nonce)
type Allocation struct { Account Account `json:"account"`; Balance Amount `json:"balance"`; Representative Account `json:"representative"` }
type Genesis struct { Version uint8 `json:"version"`; Name string `json:"name"`; Supply Amount `json:"supply"`; WorkBits uint8 `json:"work_bits"`; Allocations []Allocation `json:"allocations"` }
func (Genesis) Validate() error
func (Genesis) ID() Hash
func LoadGenesis(string) (Genesis,error)
```
Genesis ID canonical sorted allocations with domain separation; changes create a distinct
network. Allocation accounts must be unique; supply checked; positive total, WorkBits <=24.
Block canonical fields: domain "BL/block/v1", version u8, decoded network32, kind u8
(send=1 receive=2 change=3), public account32, previous32 (zeros if empty), representative32,
balance u64 BE, link32 (destination pubkey or source hash or zeros). Signature signs ID bytes.

### internal/ledger (core agent)

```
var ErrNotFound, ErrConflict, ErrDependency, ErrInvalid, ErrClosed error
// all structs JSON lower snake_case fields
type AccountState struct {Account protocol.Account; Frontier protocol.Hash; Balance protocol.Amount; Representative protocol.Account; Height uint64}
type Receivable struct {Source protocol.Hash; From protocol.Account; To protocol.Account; Amount protocol.Amount}
type Vote struct {Network protocol.Hash; Height uint64; Round uint32; Phase string; Parent protocol.Hash; Block protocol.Hash; Representative protocol.Account; Signature string}
func (*Vote) Sign(ed25519.PrivateKey) error
func (Vote) Verify() error
// vote signature domain "BL/vote/v1" encoded fixed fields phase=prevote/precommit, nil Block=""
type Record struct {Height uint64; Round uint32; Previous protocol.Hash; Block protocol.Block; Votes []Vote}
func (Record) ID() protocol.Hash // domain + height BE + Previous decoded32 + Block.ID decoded32
// Record.ID excludes round and certificate, making same chain commitment deterministic.
type Status struct {Network protocol.Hash; Height uint64; Tip protocol.Hash; Supply protocol.Amount; Accounts int; Receivables int}
type Store struct {...}
func Open(directory string,g protocol.Genesis) (*Store,error)
func (*Store) Close() error
func (*Store) Genesis() protocol.Genesis
func (*Store) Status() Status
func (*Store) Account(protocol.Account) (AccountState,error)
func (*Store) Receivables(protocol.Account,offset,limit int) []Receivable // deterministic order
func (*Store) History(protocol.Account,offset,limit int) []Record
func (*Store) Records(after uint64,limit int) []Record
func (*Store) Block(protocol.Hash) (protocol.Block,error)
func (*Store) Weights() map[protocol.Account]protocol.Amount
func (*Store) Validate(protocol.Block) error // at latest confirmed state; signature/work/network
func (*Store) Commit(Record) error // independently verify quorum & dependencies, atomic durable
func (*Store) SaveSigningState([]byte) error
func (*Store) SigningState() ([]byte,error) // ErrNotFound initially
func (*Store) Backup(io.Writer) error // consistent full framed journal including genesis header
func Restore(directory string,src io.Reader,g protocol.Genesis) error // refuses nonempty target
```
Weight = each account's balance + outstanding sends FROM that account, delegated to its
current representative. Pending tokens carry weight until received; total weight = supply.
Certificate: unique representatives precommit same network,height,round,parent,block;
strict > 2/3 of total supply using overflow-safe arithmetic; use pre-state Weights.
Each Record serially applies one block. Height starts 1, genesis height0 tip Genesis.ID.
Genesis allocations frontier empty Height0; opening receive Previous empty.
Ledger WAL must validate checksums+certificates during replay, lock directory, fsync before
publishing state, handle torn tail vs corruption. Do not add arbitrary checkpoints or bypasses.
Signing state MUST be atomically durable, exclusive same directory lock; no key in backups.

### internal/consensus (root)

```
type Config struct {Store *ledger.Store; PrivateKey ed25519.PrivateKey; RoundTimeout time.Duration; MaxPending int}
type Message struct {Type string `json:"type"`; Proposal *Proposal `json:"proposal,omitempty"`; Vote *ledger.Vote `json:"vote,omitempty"`; Record *ledger.Record `json:"record,omitempty"`}
type Proposal struct {Network protocol.Hash; Height uint64; Round uint32; Parent protocol.Hash; Block protocol.Block; ValidRound int64; Proof []ledger.Vote; Representative protocol.Account; Signature string}
type Engine struct {...}
func New(Config) (*Engine,error)
func (*Engine) Run(context.Context) error
func (*Engine) Submit(protocol.Block) (protocol.Hash,error)
func (*Engine) Handle(Message) error
func (*Engine) Subscribe(func(Message)) // nonblocking callback supplied by p2p
func (*Engine) Pending() []protocol.Block
func (*Engine) Health() map[string]any
```
Callbacks must run outside engine locks. Catchup via Engine.Handle commit not Store.Commit
while engine is voting. Root supplies consensus and tests.

### Runtime agent owns internal/node internal/p2p internal/rpc cmd/blnode

node.Config JSON fields: data_dir, genesis, rpc_listen, p2p_listen, peers ([]string full
HTTP URLs), rpc_token, representative_key (file containing hex Ed25519 seed),
round_timeout_ms, max_pending, max_peers, allow_remote_rpc.
func node.LoadConfig(path string) (Config,error); func node.Run(context.Context,Config) error.
RPC HTTP /v1/status GET, /v1/account?address= GET, /v1/receivables?address=&offset=&limit= GET,
/v1/history?address=&offset=&limit= GET, /v1/block?id= GET, /v1/blocks POST signed Block.
Responses successful raw struct/array JSON. Errors {"error":{"code":...,"message":...}}.
POST returns {"hash":...,"status":"admitted"}; confirmed duplicates status="confirmed".
/health/live /health/ready /metrics; authenticated admin /v1/admin/backup.
RPC auth Authorization Bearer token for writes/admin, default loopback; remote opt-in.
P2P separate HTTP listener with /p2p/status /p2p/records?after=&limit= /p2p/message POST;
strict bounded JSON, protocol network/version validation; bounded async broadcast and polling,
joined shutdown; verified bootstrap through commit Engine.Handle; dynamic peer discovery
bounded to configured peers/security policy (no unbounded arbitrary URL fetching/SSRF).
Follow exact endpoint response names for wallet agent. Document configuration and endpoints.

### Wallet agent owns internal/wallet internal/client cmd/blwallet cmd/blctl

Encrypted wallet with AES-256-GCM + crypto/pbkdf2 SHA256, salt random16, nonce random12,
600000 iterations, versioned format; passphrase via env/file (no flag plaintext), 0600 files,
refuse overwrite. Random seed32, deterministic accounts HMAC-SHA256(seed,"BL/wallet/v1"||indexBE).
CLI create/restore/backup/accounts/send/receive/delegate/sign/submit/status/history;
offline sign accepts explicit state fields and generates canonical signed JSON.
client RPC raw shapes as above. blctl genesis creates a private network manifest from
supplied allocations, inspect/backup/restore/config helpers; coordinate needed exact UX.
Never ship an unfunded fake payment path or predefined public secret keys.
Wallet cannot import node package or consensus; only protocol and narrow RPC client.
Focused meaningful tests at block boundaries; no repetitive full-suite runs after tiny edits.

## Integration workflow

Three isolated worktrees/branches. Agents own their listed paths, commit coherent blocks,
never merge into main themselves while another merge is underway. Root merges --no-ff
and asks each agent to merge main when an upstream dependency lands. This preserves all
real feature commits. Never manufacture commits for counts or claim tests not run.
Root owns shared docs/tasks, consensus, packaging and integration tests.
