# Local release packaging

`make binaries` builds the three local executables in bin/.
`make release VERSION=local` packages the committed host-platform source.
`make release-all VERSION=local` builds Linux/macOS amd64/arm64 archives.
These commands never publish an artifact or create a public network.

The release command requires a clean Git tree. Artifacts include a build manifest,
source revision, operator/protocol docs and the project MIT license.
The runtime uses only the Go standard library. Imported development skills retain
separate notices in .agents/; those skills are not linked into shipped executables.
Archives normalize timestamps, ownership and ordering and include SHA-256 checksums.
Use the same Go toolchain, source commit and SOURCE_DATE_EPOCH for reproduction.
If SOURCE_DATE_EPOCH is absent, the commit timestamp is used.
The manifest's exact Go toolchain string makes differing toolchains visible.

Verify checksums before extraction. `blnode --version`, `blwallet --version` and
`blctl --version` report the packaged version and source revision.
A reproducible archive is not evidence that all release gates in docs/quality.md pass.
