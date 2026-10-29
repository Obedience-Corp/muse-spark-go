# Changelog

## v0.1.0 — 2026-10-29

Initial public release of the Go SDK for the Muse CLI and Muse Session Protocol. Requires Go 1.24 or newer and a separately installed Muse executable for runtime operations. Tested against Muse Code 1.4.14.

### Included

- One-shot prompt execution and JSONL streaming.
- MSP host lifecycle, concurrent RPC calls, session start and durable resume.
- Per-turn streaming, repeatable terminal waits, native cancellation and interruption.
- Independent bounded subscriptions, explicit delivery-gap errors, durable history paging and replay.
- Approval/input request decoding, explicit presentation receipts, decisions and answers.
- Context-aware operations with preserved cancellation categories and custom causes.
- Seven runnable echo demos, a reproducible VHS recording, modular Just recipes, and protocol regression tests.

### Compatibility and validation

The package compiles for Linux, macOS, and Windows on amd64 and arm64 without cgo or an installed Muse binary. Native echo execution is verified on macOS arm64 and Linux arm64. Real Linux demos cover multiple turns, persisted restart/resume and history readback, cancellation/interruption, permission-mode selection, and process errors. Go 1.24 and 1.26 container race/build/vet checks pass.

Native model-generated approvals/questions, paid providers, non-text inputs, and induced live delivery gaps remain unverified. Application state reconciliation after replay and Obey integration are outside this release. Windows and amd64 runtime behavior is not certified by cross-compilation. The API may change before v1.
