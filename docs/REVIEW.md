# Implementation review and validation

Reviewed against the Muse 1.4.14 stable schema, captured native traffic, and the [Claude SDK demos and documentation at 462120a](https://github.com/lancekrogers/claude-code-go/tree/462120a7a80e59b5a01b8ae5c3250d8cf2d05704). This is an implementation self-review and evidence record, not an independent maintainer approval.

## Corrections from the initial scaffold

| Reachable problem | Correction and proof |
| --- | --- |
| A second `Wait` consumed no result and could block forever | Cached terminal observation with broadcast completion; concurrent/repeated-wait test |
| A queued turn reclaimed by Muse never emits `turn/completed` | Recognize `turn/unqueued`; terminal fixture test |
| An early item/terminal arrives before `turn/start` responds | Subscribe before admission; early-event routing test |
| An item delta has no native turn ID | Associate it through the item's start/completed record; foreign-turn filtering test |
| An application never drains global notifications | Global stream is opt-in; local subscriptions isolate overflow; repeated-burst test |
| One slow consumer blocks or terminates unrelated consumers | Independent bounded queues with explicit errors; slow/healthy subscription test |
| Writable raw event bytes are shared across consumers | Copy at delivery boundaries; payload-ownership regression test |
| A wait is cancelled while native work continues | Keep tracking; explicit cancel/interrupt; cancelled-wait then native-terminal test |
| A request/notification write can block indefinitely | Context-aware writer acquisition and pipe writes; blocked-write and cancelled-writer tests |
| A descendant retains exec stdout after parent exit | Close the owned read pipe on context cancellation; inherited-stdout test |
| A presentation request can be acknowledged twice | Shared receipt state across request copies; duplicate-receipt test |
| A requirement refers to a different approval or omits its index | Validate the native requirement reference; exact identifiers in decision fixture |
| An MSP child exits nonzero | Retain the native process error and exit status; exit-status regression test |
| A host drops view events or returns a pagination cycle | Explicit `ErrGap`, durable replay, no-progress detection; gap/replay tests |
| A successful process exits without a run terminal | Reject missing/malformed/duplicate terminal streams; exec negative-path tests |

No model-turn retries were added. Cancellation or disconnect after a write can leave a command's outcome unknown; the SDK preserves that boundary rather than inferring success or issuing another side effect.

## Validation lanes

- `just check`: build, default tests, vet, and formatting.
- `just test race`: concurrent host calls, turn waits, event subscriptions, and error paths under the race detector.
- `just test container 1.24` and `1.26`: Linux checks with source/module cache read-only and networking disabled.
- `just build cross`: Linux, Darwin, and Windows, each amd64 and arm64.
- `just test native <verified-linux-binary>`: all seven demos, with real durable restart/readback and native cancel/interrupt terminals. [Captured output](demos/linux-echo.txt).
- `just demo record`: real macOS terminal recording; the rendered final frame was inspected for both session completion and durable resume/readback.

Coverage was inspected to identify unexercised paths; no percentage threshold is used. Real demos are a separate lane and are not represented by unit coverage. `actionlint`, shell syntax validation, local Markdown link validation, and `git diff --check` also passed.

The Linux arm64 artifact was downloaded from the public Muse release manifest and checked against SHA-256 `3dc39b8fa26f9963cc2038f2966c0932ed5a39f0f9efb39cc0266a261cbbcf76`. CI pins the same release and architecture-specific checksum. Downloads occur before the network-disabled native scenario.

## Remaining boundaries

Native model-generated approval and input UI flows, induced live view gaps, paid providers, and non-text inputs require controlled integration proof. Application state reconciliation after replay is intentionally explicit. The SDK is ready for review of the tested core; this record does not authorize a v1 claim or substitute for the separate Obey integration gates. See [compatibility](COMPATIBILITY.md).

## Follow-up context audit

After PR #2 merged, audited command execution, host startup, writer/receipt serialization, RPC response waits, session wrappers, turn waits, replay, and close. Fixed cancellation-category loss in `Version`, custom-cause loss in cancellation returns, receipt mutex contention that ignored deadlines, a fresh two-second cleanup delay after cancelled startup, and success returned after a cancelling final replay callback.

Regression tests cover mid-flight version cancellation, stalled handshake cancellation, host survival after the startup context ends, receipt contention, RPC waiter detachment, writer acquisition deadlines, stream cancellation, final replay-callback cancellation, wait-only turn cancellation, and forced shutdown/reaping. Tests preserve intentional host/turn lifetime boundaries. The [context contract](MSP.md#context-contract) documents those boundaries and the cooperative requirement for caller-provided I/O and callbacks.

Context-audit validation passed: targeted race tests, `just check`, full Go 1.24 and Go 1.26 network-disabled container race/build/vet checks, and all seven real Linux Muse demos. No public method signatures or native host/turn ownership semantics changed.
