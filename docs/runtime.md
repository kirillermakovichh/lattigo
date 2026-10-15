# Node runtime and HTTP interfaces

The node runs ledger, confirmation, P2P and RPC in one process. Spending keys remain
in the separate local wallet. Use an explicit genesis manifest for every network.

## Configuration

Run `blnode --config node.json`. Paths resolve relative to the configuration file.
All unrecognized JSON fields are rejected. Defaults are shown below; genesis is required.

```json
{
  "data_dir": ".local/node",
  "genesis": "genesis.json",
  "rpc_listen": "127.0.0.1:7070",
  "p2p_listen": "127.0.0.1:7071",
  "peers": [],
  "rpc_token": "",
  "representative_key": "",
  "round_timeout_ms": 2000,
  "max_pending": 1024,
  "max_peers": 32,
  "history_limit_bytes": 268435456,
  "allow_remote_rpc": false
}
```

`BL_RPC_TOKEN` overrides the file token. An empty token makes RPC read-only.
`history_limit_bytes` bounds the durable history indexed in memory (default 256 MiB,
minimum 32 MiB, maximum 1 TiB). Reaching the bound stops admission and readiness;
increase it only after provisioning corresponding RAM and disk, then restart.
Status and metrics expose retained bytes and the configured bound.
Use at least 16 characters for a local token and at least 32 for remote RPC.
Remote RPC additionally requires `allow_remote_rpc=true`. The HTTP listener does not
terminate TLS; use an authenticated TLS proxy on untrusted networks.
Never expose a bearer token over an untrusted plaintext connection.
The representative key file contains a 32-byte Ed25519 seed in hexadecimal. It must be
a regular owner-only file. No representative key means a non-voting full node.
Do not clone a live representative signing identity or restore its state backwards.

## Payment RPC

Success responses are raw JSON objects or arrays. Errors have the shape
`{"error":{"code":"not_found","message":"..."}}`.

| Method and route | Response |
| --- | --- |
| GET /v1/status | Ledger network, height, tip, supply and state counts |
| GET /v1/genesis | Validated genesis manifest, including work difficulty |
| GET /v1/account?address= | Confirmed account state; 404/not_found if unopened |
| GET /v1/receivables?address=&offset=&limit= | Deterministically ordered receivable transfers |
| GET /v1/history?address=&offset=&limit= | Confirmed account records |
| GET /v1/block?id= | A confirmed block |
| POST /v1/blocks | Admit a signed block; returns hash and status |
| GET /v1/admin/backup | A consistent binary ledger backup |
| GET /health/live | Process liveness |
| GET /health/ready | Consensus readiness and peer synchronization state |
| GET /metrics | Prometheus text metrics without unbounded identity labels |

POST /v1/blocks and all administrative routes require `Authorization: Bearer TOKEN`.
A successful new submission returns HTTP 202 and `status=admitted`; this is not finality.
A retry of a confirmed signed block returns HTTP 200 and `status=confirmed`.
After a submission timeout, look up the block hash before replacing or retrying work.
Pagination defaults to 100; limits are 1..1000 and offsets 0..10000000.
Request bodies are at most 1 MiB. RPC admits at most 32 simultaneous handlers.
Overload returns HTTP 503. Invalid JSON and unknown fields are rejected.
The backup is a ledger backup, not a wallet or representative-signing-state backup.
Only one backup is staged at a time. It is completed in a private temporary file before
HTTP success headers are sent; the temporary file is deleted after transfer.

## Peer exchange

P2P uses a separate HTTP listener. Every request and response carries
`X-BL-Version: 1` and `X-BL-Network: GENESIS_HASH`. Mismatches are rejected.
A manifest change creates a different network. Consensus messages remain signed;
HTTP peer addresses alone grant no voting authority.

- GET /p2p/status advertises protocol version, network, tip, height and configured peer URLs.
- GET /p2p/records?after=HEIGHT&limit=N returns at most 16 certified records.
- POST /p2p/message admits one bounded consensus message or signed block.

Only configured peer URLs are dialed. Advertised URLs are diagnostic and are not
followed automatically. This deliberately avoids peer-driven arbitrary URL fetching.
HTTP redirects and ambient proxy settings are disabled. Configure HTTPS peer URLs
when transport confidentiality or server authentication is required.

P2P bodies are at most 20 MiB and fetched responses at most 32 MiB. There are at most
16 simultaneous inbound handlers. Per-source admission allows 50 requests/second
with a burst of 100; the source table retains at most 4096 entries.
Configured peers default to 32 and cannot exceed 128. Outbound broadcasts are
nonblocking with a global 256-message/32-MiB queue and a 32-message/24-MiB queue per peer.
Saturated gossip is dropped and counted. Certified-record polling and pending-block
retransmission (at most 32 pending blocks per peer tick) repair lost gossip. A slow peer has its own worker and HTTP deadlines.
Record pages enforce byte and count limits while encoding each record separately;
a record count cannot force allocation of an entire history-sized JSON response.
Pages that fail the response budget are retried as one certified record.
Transport limits exceed the maximum 16-MiB durable certificate frame.
Catchup never calls ledger commit without consensus validation.

## Lifecycle

Startup validates configuration, genesis, signing key and durable state before opening
listeners. Any partial startup failure releases already-opened resources.
Readiness is separate from liveness and includes consensus and observed catchup state.
When pending work lacks observed voting quorum, readiness returns HTTP 503. An idle
node can serve confirmed reads without promising that a voting quorum is currently online.
An isolated node cannot infer unseen network progress from its own ledger height.
Disconnected peers do not keep readiness blocked with old advertised heights. A peer
claiming to be ahead must return a nonempty certified page to remain connected.
SIGINT or SIGTERM stops admission, drains HTTP handlers for up to ten seconds,
cancels owned background workers, joins them, and closes durable state.
An expired drain deadline is an error; open connections are closed before storage release.

## Resource provisioning

For the default 32-peer configuration, provision at least 8 GiB RAM for conservative
headroom under saturating input and maximum-size certificates. This is an engineering
recommendation from configured bounds, not a measured throughput or memory guarantee.
Increasing peer count or retained history raises the required memory budget.
Monitor retained-history bytes, queue bytes and rejected requests under your workload.
