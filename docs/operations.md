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

## Container execution

Build with `docker build -t block-lattice:local .`. The multi-stage image contains
only the static node and CA roots and runs as UID/GID65532. Mount an explicit
read-only configuration directory at `/config` and a writable private data directory
at `/data`, owned by65532. Representative seeds and configuration are mounted at
runtime, never copied into the build context. Set `data_dir` to `/data`.
Use `--read-only --cap-drop ALL --security-opt no-new-privileges` and publish only
required listeners. Remote RPC still requires explicit configuration opt-in and a
strong token. Pin base image digests in your deployment's supply-chain policy.

## Systemd execution

`deploy/systemd/blnode.service` uses a dynamic unprivileged identity, a private
`StateDirectory`, a strict read-only filesystem, no capabilities and bounded file
and task counts. Install the binary at `/usr/local/bin/blnode` and configuration at
`/etc/block-lattice/node.json`; set `data_dir` to `/var/lib/block-lattice`.
Use systemd credentials or an appropriately restricted service-readable mount for
representative keys and tokens. Do not weaken key-file permissions to make a host
user's seed readable. The pre-start command validates inputs without opening the
ledger. Stop allows30seconds for the node's owned shutdown. Review resource limits
against your deployment workload; they do not establish hardware qualification.

`blctl ready --rpc http://127.0.0.1:7070 --timeout 5s` prints the current readiness
body and exits nonzero when admission is unavailable. The Go client `Readiness`
method returns `Ready=false` for a valid503 readiness response; malformed bodies,
status/body contradictions and transport failures return errors.

`blctl doctor --config node.json --live --timeout 5s` additionally probes the local
RPC status/readiness and opens a TCP connection to the configured P2P listener.
Wildcard listeners map to loopback for these probes. Output includes current
height, history bytes/limit, capacity, syncing and observed quorum. The P2P check
proves listener reachability only; it does not authenticate peers or prove quorum.
Offline doctor performs no network operations. Probe failures return nonzero with
redacted errors and any already collected diagnostics.

Run `python3 tests/container_smoke.py --binary-dir bin` after building host CLIs.
The tool checks Docker availability before creating anything, builds a temporary
image, generates a new wallet/genesis in a private temporary directory and starts
one randomly named container with read-only root/config, dropped capabilities and
a bounded64MiB tmpfs data mount. It checks unprivileged image identity, reads the
ledger and verifies clean shutdown. Cleanup targets only its own container/image;
`--image existing-tag` retains the supplied image. No Docker daemon means an
explicit nonzero unavailable result, never a passing smoke claim.
