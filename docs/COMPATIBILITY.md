# Compatibility

## Supported baseline

| Component | Baseline / evidence |
| --- | --- |
| Go | Minimum 1.24; container checks also run on 1.26 |
| Muse | Code 1.4.14, build 1.4.14-R7309.1 |
| MSP | Schema version 1; unknown fingerprint warns, unsupported version fails |
| Native runtime | macOS arm64, Linux arm64, and hosted Linux amd64 echo execution |
| Compilation | Linux, macOS, Windows; amd64 and arm64 |
| API stability | v0.1.0 initial release; API may change before v1 |

Tested stable fingerprint:

```text
sha256:c94c3d1f2f51e6424074b9ec43eac3eedcf532d3dcd77351ec1383a87d3049c6
```

Protocol shapes were checked against `muse schema generate-ts`, `muse exec --help`, and `muse serve --help` from that installed version. Raw result/event fields remain available for schema additions. New fingerprints require a fresh real-binary check; warnings alone are not a compatibility guarantee.

## Surface

Typed APIs cover exec prompts/streaming, MSP initialization, session start/resume, turn admission/events/terminal/cancel/interrupt, approval mode and decisions, user-input answers/cancel, session leave, and view paging/replay/subscription. `InputPart` includes text, image, skill, and video-reference wire fields; only text has native echo proof.

Raw `Call`, `NotifyContext`, and session `Command` allow other native methods. They are advanced escape hatches, not a promise of typed or tested parity with every Muse management command. Options intentionally cover the SDK's execution surface; consult Go documentation and native help for the available flags.

## Evidence boundaries

Real-binary demos prove one-shot and streamed echo, multiple turns, durable restart/resume, history paging, cancel/interrupt, and restrictive approval mode. Protocol fixtures exercise malformed and oversized frames, early completion, out-of-order responses, per-session routing, queued reclamation, slow consumers, gap signaling, pagination cycles, stale-approval errors, explicit receipts, and decision/question IDs.

Native model-generated approvals, questions, live gap induction, non-text inputs, and paid model providers have not been exercised. Replayed history is real; induced gap/overflow recovery semantics use fixtures. Windows and macOS amd64 have compile checks, not native execution proof. The SDK must remain importable and compilable on every Obey target even when Muse is unavailable at runtime. Ryzen Strix uses the same `amd64` Go target; the SDK does not require GPU/NPU-specific code. The SDK manages the owned stdio host, not an external socket daemon or an entire arbitrary descendant process tree.

`ReplayView` exposes durable recovery; the application owns state reconciliation and cannot recover lost ephemeral deltas. There is no automatic retry of model turns or permission decisions. No socket transport, login/installation manager, plugin manager, model budget accounting, or Obey provider is included in this SDK change.

## Release scope

`v0.1.0` is an initial public SDK release, not a claim of native feature parity on every platform. Platform availability of the Muse binary is a runtime concern: constructing/importing the SDK does not spawn Muse; explicit discovery or execution returns an error when it is absent. There are no native-library or cgo requirements in this module. The verified execution features and remaining integration boundaries above are part of the release contract.

## CI availability

Early private-repository runs were blocked by account billing/spending limits. Since the repository became public, hosted CI runs the Go checks, cross-builds, and network-disabled native Linux amd64 echo demos. A passing cross-build does not establish native runtime coverage for that target.
