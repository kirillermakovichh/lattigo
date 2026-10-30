# Wallet and operator commands

The wallet signs locally. A node receives signed public blocks, never the wallet seed.
`blwallet`, `blctl` and `blnode` are separate executables.
Amounts are unsigned 64-bit integer raw units, represented as decimal strings in JSON.
No decimal currency conversion or external network compatibility is implied.

## Build

```sh
mkdir -p bin
go build -o bin/blwallet ./cmd/blwallet
go build -o bin/blctl ./cmd/blctl
```

Use `blwallet help`, `blctl help` or `<command> --help` for exact flags.
Flags follow the command name. Unknown positional arguments are errors.

## Secret input

Use `--password-file` pointing at a regular private file, or `BL_WALLET_PASSWORD`.
Use `--token-file` for the node write token, or `BL_RPC_TOKEN`.
Private input files must have permissions 0600 or stricter. A trailing newline is removed
from password/token files. New passphrases must contain at least 12 bytes.
There is no plaintext password flag and no implicit interactive prompt.
Protect the parent directory and use a secret manager to populate these inputs.
Environment strings and temporary Go runtime copies cannot be guaranteed erased.

## Create, derive and recover

```sh
bin/blwallet create --wallet .local/alice.json --password-file .local/password
bin/blwallet accounts --wallet .local/alice.json --password-file .local/password --count 3
bin/blwallet backup --wallet .local/alice.json --password-file .local/password --out .local/alice.backup
bin/blwallet restore --wallet .local/alice.restored.json --password-file .local/password --backup .local/alice.backup
```

Creation uses a random 32-byte master seed. `create --seed-file` explicitly imports a
private file containing 64 hex seed characters. Account index `i` derives an Ed25519 seed
with HMAC-SHA256(master seed, `BL/wallet/v1` followed by `i` as uint64 big-endian).
The same master seed and index reproduce the same account. Indexes are not stored or
incremented automatically; record the indexes in use alongside the encrypted backup.

Wallet files use PBKDF2-HMAC-SHA256 with 600000 iterations, a random 16-byte salt,
AES-256-GCM with a random 12-byte nonce and versioned authenticated data.
Encrypted backup/restore verifies the password and preserves the encrypted envelope.
Creation, backup, restore and explicit exports refuse to overwrite existing files.
Writes sync the file and directory. Unsupported envelope/KDF versions fail closed.
Use `blwallet password --wallet PATH --password-file OLD --new-password-file NEW`
to change a passphrase. Environment alternatives are `BL_WALLET_PASSWORD` and
`BL_WALLET_NEW_PASSWORD`; no passphrases are accepted as command-line literals.
Password rotation atomically re-encrypts the same seed under a custody lock. Account
identities stay unchanged; earlier backups retain their earlier passphrases.
Losing the seed and all backups permanently loses access to funds.

## Configure an isolated network

Choose the funded account from `accounts` output. The corresponding index can also act
as the initial representative. Exporting one representative key does not export the master seed.

```sh
bin/blwallet export-key --wallet .local/alice.json --password-file .local/password --index 0 --out .local/representative.seed
bin/blctl token --out .local/rpc.token
bin/blctl genesis --name private-lattice --account "$ACCOUNT" --balance 1000000000 --work-bits 12 --out .local/genesis.json
bin/blctl config --genesis genesis.json --data-dir data --representative-key representative.seed --token-file .local/rpc.token --out .local/node.json
bin/blnode --config .local/node.json
```

`ACCOUNT` must contain the actual checksummed account address from your wallet.
The genesis command defaults its single allocation's representative to that account.
Multiple initial allocations use `--allocations allocations.json`, a complete file
limited to1MiB containing an array of objects:
`account`, `balance` (decimal string) and `representative`. Supply is checked and summed.
Network identity includes genesis parameters. Different manifests create different networks.
No embedded secret key or pre-funded external network is shipped.

Configuration paths resolve relative to the configuration file. `--peer` is repeatable.
Without `--token-file` or `BL_RPC_TOKEN`, `config` generates a random token and stores it
only in the private config; it is not printed. Administrative and write requests require
the same token. See runtime documentation for listener and network restrictions.

## Send, receive and delegate

```sh
bin/blwallet send --wallet .local/alice.json --password-file .local/password --token-file .local/rpc.token --to "$DESTINATION" --amount 25 --wait
bin/blwallet receivables --wallet .local/bob.json --password-file .local/password
bin/blwallet receive --wallet .local/bob.json --password-file .local/password --token-file .local/rpc.token --source "$SEND_HASH" --wait
bin/blwallet delegate --wallet .local/bob.json --password-file .local/password --token-file .local/rpc.token --representative "$REPRESENTATIVE" --wait
```

`--rpc` selects an HTTP(S) origin (default `http://127.0.0.1:7070`).
Use `--expected-network GENESIS_HASH` to pin online payment creation to a known genesis.
The CLI compares RPC balance metadata with the owner's signed frontier, or the explicit
genesis allocation, before signing. RPC state still requires a trusted node: the wallet
is not a standalone light-client verifier of all confirmation certificates.
Opening a new account is a receive. Its representative defaults to itself unless supplied
with `receive --representative`. For an existing account, use `delegate` separately.
`--wait` polls the exact block hash until confirmed or `--timeout` expires.
An admitted block is not represented as confirmed until lookup succeeds.

## Uncertain submission and durable outbox

Online transaction creation first saves the exact signed block to
`<wallet path>.pending-<index>.json`. Optional `--out` saves an additional exclusive copy.
If submission times out, inspect or resubmit that same signed file:

```sh
bin/blwallet block --hash "$HASH"
bin/blwallet submit --file .local/alice.json.pending-0.json --token-file .local/rpc.token --wait
```

The CLI refuses to create another online block for that account until the previous
outbox block is confirmed. It never invents an automatic conflicting replacement.
If a signed candidate is permanently rejected, inspect its error and account frontier
before explicitly removing the outbox. The CLI cannot infer that a timeout means rejection.
Concurrent online commands for an account within one wallet file are rejected while
another holds its account lock. Symbolic-link aliases share that lock and outbox.
Restored/imported copies at different paths or hosts have independent locks and must
not create concurrent payments for the same account. Keep lock files in
place; removing one while a command runs would break mutual exclusion.
Keep pending files with an incident backup; encrypted wallet backup alone contains keys,
not pending transaction intent. Online creation combines an account lock with exclusive outbox publication.

## Offline signing

`sign` accepts explicit `--network`, `--kind`, `--previous`, `--representative`,
`--balance`, `--link` and `--work-bits` values. It emits signed block JSON, or writes an
exclusive `--out` file. `--balance` is the resulting balance, not a transfer amount.
For a send, link is the destination; for a receive it is the source hash; for a change
it is empty. Offline signing cannot validate a frontier or receivable against live state.
It deliberately requires explicit state; it must not be used to sign competing candidates.
Submit the exact saved file from a connected machine using `submit --file`.
Signed block input files are limited to64KiB in their entirety, including whitespace.

## Query and operator recovery

`status`, `account`, `history`, `receivables` and `block` print structured JSON.
Queries accepting `--address` do not require opening a wallet. History/receivables support
`--offset` and `--limit` (1..1000). RPC responses are bounded to 32 MiB so a valid
certificate-bearing history page fits; redirects are disabled. Status includes optional
`history_bytes`, `history_limit_bytes` and `capacity_reached` fields when reported by the node.

```sh
bin/blctl status
bin/blctl backup --token-file .local/rpc.token --out .local/ledger.backup
bin/blctl restore --genesis .local/genesis.json --backup .local/ledger.backup --data-dir .local/restored-data
bin/blctl inspect --genesis .local/genesis.json --data-dir .local/restored-data
bin/blctl verify --genesis .local/genesis.json --data-dir .local/restored-data
```

Live backup is authenticated and streamed into an exclusive durable file.
Its default byte limit is 1 GiB; `--max-bytes` changes that bound.
Offline inspect/verify require an existing journal, acquire the data lock and replay it
without repairing incomplete records. They do not create a missing ledger. Stop the node first.
Restore refuses a nonempty target and verifies the backup against the supplied genesis.
For a node with a nondefault history ceiling, pass the same byte value through
`--history-limit-bytes` to restore, inspect and verify. Otherwise they use256MiB.
Restored data is **observer-only**: a ledger backup is not authorization to roll back or
reactivate an old representative signing identity. Do not clone active signing keys.

## Full CLI/network check

Run `python3 tests/network_smoke.py` to build temporary binaries and exercise one complete
local scenario. Use `--binary-dir bin` to exercise already-built release binaries.
The script generates random identities and credentials, launches four representative
processes, confirms send/receive/delegation, checks idempotent retry and unauthorized
write rejection, takes one representative offline, verifies restart catch-up, streams a
backup, restores an observer and confirms another transfer across all five processes.
It uses bounded deadlines and cleans up child processes and temporary secrets.
This scenario passed at confirmed height 5 with recipient balance 30 raw units. It is
a functional integration check, not an external review or sustained capacity benchmark.

## Contacts, account names and outbox administration

`blwallet contacts --wallet PATH` lists private named recipients in name order.
Use `--action add --name NAME --address ADDRESS --network HASH` to add one;
`--replace` is required to change an existing recipient. `--action remove --name NAME`
deletes it. `--action export --file FILE` creates a private exclusive contact archive;
`--action import --file FILE` merges it atomically. Conflicting names require `--replace`.
`send --contact NAME --amount VALUE` uses the saved destination and pins its network.
It cannot be combined with `--to` or a conflicting `--expected-network`.

`blwallet names --action add --name Savings --index 9 --wallet PATH` records a local
account label. List with `names`, remove with `names --action remove --name NAME`, and
select with `--account-name Savings` on account/payment commands. An explicit `--index`
cannot be combined with `--account-name`. Contacts and account labels have separate namespaces.
Metadata lives in the private, bounded `PATH.profile.json` sidecar. Wallet backup/restore
continues to protect the encrypted seed; keep metadata separately when preserving names.
Contact archives do not contain seeds, passphrases or account labels.

`blwallet outbox --wallet PATH --index N` verifies and inspects saved signed intent,
without claiming admission or confirmation and without requiring the wallet password.
`outbox --action retry` authenticates the selected wallet, checks the RPC network and
resubmits the same signed block under the account lock. Optional `--wait` waits for confirmation.
`outbox --action clear` requires successful confirmed lookup before removing and syncing
that account's outbox. Neither command invents a replacement for uncertain intent.

## Payment request format

An off-chain payment request uses `blpay:ADDRESS?amount=RAW&network=HASH`, with optional
`label` (64 UTF-8 bytes) and `message` (256 UTF-8 bytes). Amount is a positive canonical
raw-unit integer; network is the explicit genesis hash. Repeated or unknown fields,
authorities, fragments, control characters and inputs above2048 bytes are rejected.
Formatting sorts and escapes query fields deterministically. Labels and messages are
request metadata and do not become fields of signed blocks or ledger history.
