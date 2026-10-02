# Go Block Lattice

A Nano-inspired payment network written in Go. The working name is Go Block Lattice;
branding, ticker and public module path remain open.

Each account has its own signed chain. Transfers connect account chains.
Representatives vote to resolve conflicts and establish confirmation.
The target is a complete product with a node, representative mode, local wallet,
CLI, versioned RPC, network synchronization and operator tooling.

**Current state:** repository foundation and design documents. No ledger, node,
wallet or consensus implementation exists yet. This is not a Nano-compatible client.

## Development

Go 1.26 is the initial development baseline. See [development](docs/development.md).

```sh
make check
```

The Go module is initialized without application packages. Empty package sets are
reported explicitly until real Go behavior is added.

## Project map

- [Documentation](docs/README.md): product, protocol, architecture and operations.
- [Domain vocabulary](CONTEXT.md): canonical terms.
- [Work plan](tasks/plan.md): dependencies and delivery approach.
- [Task tracker](tasks/todo.md): acceptance criteria and completion status.
- [Session handoff](tasks/implementation-progress.md): current evidence and next work.
- [Contributor rules](AGENTS.md): structure, skills, verification and commits.

Public network parameters and licensing must be decided before external release.
