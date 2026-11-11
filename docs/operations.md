# Operator requirements

Status: maintained operator policy. Executable commands and configuration live in
runtime.md and wallet.md. No predefined public network is shipped.

## Configuration and isolation

Select the network explicitly. Separate devnet, testnet and public network identities,
data directories, signing domains and peer sets. Validate configuration before opening listeners.
Bind administrative endpoints to loopback by default; remote exposure requires authenticated,
authorized transport with bounded input. Separate operator actions from public read RPC.
Do not place wallet spending keys in node configuration.

## State and recovery

Document data-directory ownership, exclusive access and schema compatibility.
Produce backups with a consistent database view and verify restoration in isolation.
Representative signing records require an explicit rollback-resistant recovery procedure.
Do not run two representative instances with the same signing identity from cloned backups.
A backup that restores balances but permits conflicting final votes is not a valid recovery design.
Define corruption detection, repair limitations and full resynchronization trust assumptions.

## Observability

Expose sync status, confirmed-state progress, peer count, elections, voting activity,
queue bytes/items, rejected work, disk health and persistence failures.
Use bounded-cardinality metrics. Avoid account/hash labels with unbounded values.
Readiness distinguishes syncing, quorum unavailable, overloaded and storage-unhealthy states.
Logs contain correlation identifiers and stable error classes without seeds or private keys.

## Upgrades and incidents

Document supported upgrade paths, database migrations, protocol activation and downgrade limits.
During suspected conflicting confirmation, preserve evidence and stop unsafe signing.
During disk failure, stop admitting work that requires unavailable durability.
Do not conceal uncertainty behind a healthy status or automatic reset.
Use blctl backup for an authenticated consistent journal and blctl restore for an
empty observer-only data directory. Stop a node before offline inspect/verify.
Compatible binary upgrades: take a verified backup, stop gracefully, replace binaries,
check their source/version, restart against the same genesis/config and verify readiness.
WAL1/protocol1 are the only supported schemas. There is no older application schema to
migrate. Unknown formats are rejected; do not edit magic bytes to force an upgrade.
For an incompatible future version, a reviewed migration/activation plan must precede use.
Set history_limit_bytes before reaching capacity; replay can require raising it for an
existing valid ledger. Raising serialized limits also raises memory requirements.
Run the service as an unprivileged user with an owner-only data directory. Use a service
manager to supervise blnode --config /absolute/path/node.json and deliver SIGTERM.
Keep RPC on loopback unless explicitly enabling a secured remote deployment.

## Startup preflight

`blnode --config node.json --validate-config` validates configuration, genesis and
representative-key inputs without opening storage or listeners. `blctl doctor
--config node.json` emits a redacted local summary: listener addresses, configured
capacity and boolean authentication/representative state. It never prints token
values, key paths or data-directory paths. Neither command proves live readiness.

`blctl profile --rpc http://127.0.0.1:7070 --kind heap --out heap.pprof`
downloads an authenticated profile to an exclusive0600 file. Use `--kind cpu
--seconds 5` for a bounded CPU sample and `go tool pprof heap.pprof` to inspect.
Failed or oversized downloads do not publish the destination; existing files are
never overwritten. The default download deadline is15seconds.
