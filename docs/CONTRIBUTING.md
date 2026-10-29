# Contributing

Use Go 1.24 or newer and Just. Recipes are grouped under `.justfiles/{build,test,lint,demos}.just`.

```sh
go mod download
just check
just test race
just test container 1.24
just test container 1.26
just build cross
```

The default test suite launches its own test executable as a protocol peer. It does not read user credentials, run a paid model, or create workspace fixtures. Tests should assert externally visible protocol and lifecycle behavior: terminal ownership, cancellation, response correlation, error propagation, and child-process cleanup.

Filesystem-mutating tests and durable native scenarios belong in disposable containers. Use `just test native /absolute/path/to/linux/muse`. Keep source and module caches read-only and the scenario network-disabled. Do not add host temp-directory tests for filesystem workflows.

`just test coverage` writes a local report under ignored `coverage/`. It is diagnostic, not a quota. Demo executables are exercised through the native lane rather than counted as unit-test coverage.

When changing the protocol surface, export the tested Muse binary's schema and compare the affected methods. Preserve raw payloads and open enum values. Keep approval presentation separate from permission decisions. Never add implicit retries to admitted commands. New queues need bounded behavior, an explicit loss signal, and cancellation coverage.

When changing examples, run them against the real binary and update their documented proof. Regenerate `docs/demos/sessions.gif` with `just demo record` when its behavior changes; inspect the rendered terminal. Keep recordings free of credentials and personal session data.

This project is registered in an Obedience Corp camp. Its design and handoff are work item **WI-ebeb03**, `workflow/design/muse-spark-go-sdk`, linked to `muse-spark-go`. Inside the camp use `camp p commit`; keep planning updates in the camp-root work item, separate from product commits.
