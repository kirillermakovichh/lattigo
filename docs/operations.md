# Operator requirements

Status: design requirements, not executable deployment instructions.
No node binary, ports, configuration schema or production network exists yet.

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
Concrete commands, configuration examples and service definitions will accompany implemented behavior.
