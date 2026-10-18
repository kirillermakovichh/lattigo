# Integrated verification record

Date:2026-11-15.

Packaged source revision: `9b1faea0cbaf05b2d789e8a483342d4546ddd33d`.
Toolchain: `go version go1.26.0 darwin/arm64`.

## Observed results

- make check passed:47 relative documentation links, Go formatting, vet, package tests and race detection.
- Final docs-only link check passed with49 relative file links.
- Four Linux/macOS amd64/arm64 targets cross-compiled and packaged.
- Rebuilding every archive from the same committed source reproduced every byte.
- All three extracted host binaries reported the matching source/version.
- The archive-binary network scenario passed in 4.018 seconds on the local host.
- Four representative processes and one restored observer converged at height5.
- The recipient confirmed balance was30 raw units.
- Verified flows: send, receive, delegation, duplicate retry, unauthorized write rejection, quorum with one representative offline, restart catchup, backup and observer-only restore.

Only the host darwin/arm64 binaries were executed. Other targets were cross-compiled,
not run on their target OS/hardware. The scenario duration is not a throughput or finality SLA.

## Artifact checksums

```text
4cb763922442535768d6d57d45db680a37da267395fd87ec8dd3fb6801a3f2f0  go-block-lattice-local-darwin-amd64.tar.gz
8decd5d75f6eb9278930704105d248c8116ab6285957b5e2163289988eac7239  go-block-lattice-local-darwin-arm64.tar.gz
68f843e2aa0dcf3042ef166143f7a0391ee0cbc057620ba7ec3d3ef0a92d1af6  go-block-lattice-local-linux-amd64.tar.gz
9187bd6ad0a6288e2421d9fa18288a0f6102ec14505f359f47d5d7780a806ace  go-block-lattice-local-linux-arm64.tar.gz
```

Archives and SHA256SUMS-local are generated under dist/ and intentionally not committed.
The artifact source precedes this documentation-only verification commit.

## Qualification still outstanding

No external independent security review, prolonged hostile public-network soak, hardware
power-loss/full-disk drill, production capacity SLA or public-network launch is claimed.
The constrained adversarial consensus trace is regression evidence, not an exhaustive proof.
See docs/quality.md and BL-020/BL-021 for those remaining gates.

## Whole-project regression pass

Date:2026-11-15. Final tested source: `c8bcacce52a52b50b0c28c1c01db24f46dd53b8a`.
Every behavioral change below was reproduced with a failing test before changing code,
verified afterward, and committed separately. Commit subjects describe changed behavior.

| Commit | Changed behavior |
| --- | --- |
| `cc6397d` | Existing ledger inspection does not create data or repair incomplete records. |
| `a616018` | Per-account wallet locking preserves pending intent during concurrent commands. |
| `596cbf0` | Authenticated future-round votes propagate once through relay nodes. |
| `12ac273` | Disconnected peers do not retain stale synchronization-readiness blockers. |
| `6fe44dc` | Journal recovery preserves disk read errors instead of treating them as torn tails. |
| `109bb0d` | Signed-block files are bounded in full before JSON decoding. |
| `7ef79af` | Genesis allocation files are bounded in full before JSON decoding. |
| `a368bea` | Restore bounds staged input and validates configured capacity first. |
| `63a05c5` | Restore/inspect/verify accept the node’s provisioned history capacity. |
| `7277428` | Explicit node help exits successfully. |
| `96cb6ab` | Listener ports are checked before opening resources. |
| `1410c48` | Configuration generation validates the actual node contract before publishing a file. |

### Observed verification

- All13 Go packages, including all three command entrypoints, compiled.
- Formatting, documentation links and go vet ./... passed.
- go test -race -count=1 -coverprofile=.cache/unit-coverage.out ./... passed.
- The public HTTP contract covers payments, history, receivables, authorization,
  quorum readiness, metrics, idempotent submission and verified streamed backup.
- CLI file-input flow covers password/seed files, account derivation, backup/restore,
  representative export, offline signing and rejection of exposed secret-file permissions.
- Three instrumented process scenarios passed at an intermediate checkpoint.
- An instrumented final-source scenario and the ordinary final archive scenario both
  passed with4 representatives,1 restored observer, height5 and recipient balance30.
- All command --help and --version exits passed on final binaries.
- Five bounded fuzz targets passed:257287 amount inputs,206746 block inputs,
  274005 vote inputs,512873 journal inputs and99334 encrypted-envelope inputs
  (1350245 total executions,2 workers per target,3-second requested intervals).
- Linux/macOS amd64/arm64 archives built and reproduced byte-for-byte.
- Only darwin/arm64 binaries were executed locally; other targets were cross-compiled.

### Combined statement coverage

Unit and instrumented CLI/network profiles were combined by source block, using the
maximum observed counter per block, without adding duplicate statements.
Coverage is 2204/2913 statements (75.7%).
Fuzz-exploration counters are not included in this coverage result.

| Package | Statements covered |
| --- | --- |
| `cmd/blctl` | 75.0% |
| `cmd/blnode` | 73.9% |
| `cmd/blwallet` | 75.0% |
| `internal/client` | 73.7% |
| `internal/consensus` | 79.5% |
| `internal/control` | 64.0% |
| `internal/ledger` | 78.7% |
| `internal/node` | 80.7% |
| `internal/p2p` | 73.2% |
| `internal/protocol` | 80.9% |
| `internal/rpc` | 84.0% |
| `internal/storage` | 73.5% |
| `internal/wallet` | 69.6% |

Uncovered code still includes error branches and OS-dependent failures. These checks
are not exhaustive scheduler exploration, long-duration hostile-load qualification,
a hardware durability drill or independent external consensus/security review.
Previously recorded BL-020/BL-021 release gates remain open.

### Final archive checksums

```text
42bf9f8e49fbca13fb62056f3ffe94e37958ba50f94ca1b4489107c1be9672a0  go-block-lattice-verified-darwin-amd64.tar.gz
8f3b4edac8f134da0bf003632381824ce88df75a64abe485d98607e2383cde89  go-block-lattice-verified-darwin-arm64.tar.gz
6c98ed9e02672cca1f267e9fc77662e220b1a2fb1ff439f0d352a14350c0d654  go-block-lattice-verified-linux-amd64.tar.gz
10bdd96a1116533a2deabf8fac6c595691daddcbd556afcc19dac28589623fc4  go-block-lattice-verified-linux-arm64.tar.gz
```

Generated archives, instrumented binaries, profiles, fuzz/build caches and raw coverage
files were removed after this report. Rebuild them using the documented source commands.
