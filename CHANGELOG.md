# Changelog

## v0.2.0 — 2026-11-26

Adds typed model catalog listing so applications can offer Muse's real models instead of a fixed "default".

### Added

- `MSPHost.ListModels(ctx, sessionID)` calls the stable MSP `model/list` query and returns a typed `ModelList`: rows newest first with model ID, display label, provider/profile, default and active flags, limits, verbatim cost strings and reasoning-effort tiers. Nullable catalog fields are pointers; `Variants` is a typed `ModelEffortVariants` (`Tiers`, or `Unknown` for the wire value `"unknown"`) and is the complete effort set, while `ReasoningEffortVariants` describes only labelled tiers; `Raw` preserves future fields and `Warnings` repeats the host's startup warnings. `ModelList` re-encodes with wire field names.
- `Client.ListModels(ctx, HostOptions)` starts an owned host, lists the catalog and reaps the child before returning, including on error or cancellation. Set `HostOptions.Provider` to the provider the application will run.
- `ModelSource*` constants for the open catalog-source vocabulary. An unknown source is returned as-is, not rejected.

### Compatibility and validation

Protocol fixtures cover full and null-field rows, typed and `"unknown"` effort variants, an older host without `variants`, session-flagged active rows, an empty catalog, unknown sources, malformed catalogs, startup warnings, a host answering `-32601` for `model/list`, cancellation and child reaping. The `-32601` path is fixture-only. Against Muse Code 1.4.25 on macOS arm64, `Client.ListModels` returned the live provider catalog with its default row flagged, and all seven echo demos passed. Muse 1.4.25 reports a stable schema fingerprint different from the 1.4.14 baseline, so `StartMSP` still adds a fingerprint warning; Linux runtime evidence remains at 1.4.14. The API may change before v1.

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

The package compiles for Linux, macOS, and Windows on amd64 and arm64 without cgo or an installed Muse binary. Native echo execution is verified on macOS arm64, Linux arm64, and hosted Linux amd64. Real Linux demos cover multiple turns, persisted restart/resume and history readback, cancellation/interruption, permission-mode selection, and process errors. Go 1.24 and 1.26 container race/build/vet checks pass.

Native model-generated approvals/questions, paid providers, non-text inputs, and induced live delivery gaps remain unverified. Application state reconciliation after replay and Obey integration are outside this release. Windows and macOS amd64 runtime behavior is not certified by cross-compilation. The API may change before v1.
