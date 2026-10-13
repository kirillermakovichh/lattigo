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
