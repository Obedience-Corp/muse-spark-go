# Releasing

This repository publishes a Go module. A semantic Git tag is the distribution artifact; compiled binaries are unnecessary because Muse is installed separately.

1. Update `SDKVersion`, the changelog, installation version, and compatibility notes in a reviewed PR.
2. Run `just release check`, the Go 1.24/1.26 container race checks, and native echo checks when behavior changes. Inspect release-bound history with a redacted secret scan.
3. Merge the release preparation into `main`. Verify the exact remote main commit and the personal GitHub identity before publishing.
4. Create an annotated `vMAJOR.MINOR.PATCH` tag at that reviewed main commit, push that exact tag, and create a GitHub Release with the changelog's tested scope and known limits. Never move or overwrite a published tag.
5. In a clean container without Git credentials or `GOPRIVATE` overrides, install the tagged module through `proxy.golang.org`, compile a consumer importing `pkg/muse` for all six supported OS/architecture targets, and confirm it runs without Muse installed until discovery/execution is explicitly requested.

A pre-v1 release does not imply native platform certification. Keep build support, actual runtime evidence, and remaining feature gaps separate in the release notes. CI service failures must be identified separately from local check results.

Inside the owning camp, use `camp p commit` for product changes and `camp commit` for scoped planning updates. The release tag and GitHub Release are deliberate publication steps, not part of the ordinary check recipe.
