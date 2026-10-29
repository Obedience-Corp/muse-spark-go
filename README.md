# muse-spark-go

Go SDK for driving the installed Muse CLI and its Muse Session Protocol (MSP) host. The API is in early development and may change before v1. The repository and module are private; configure `GOPRIVATE=github.com/Obedience-Corp/*` and Git credentials in every build environment that consumes it.

Requires Go 1.24+ and an installed Muse binary. Tested locally with Muse Code 1.4.14 (1.4.14-R7309.1) on macOS arm64. The SDK does not install Muse, manage credentials, or disable its updater; set `MUSE_NO_AUTO_UPDATE=1` when a pinned binary is required. Muse owns authentication, config, session persistence and permission policy.

## One-shot

```go
client, err := muse.NewClientFromPath()
if err != nil { return err }
result, err := client.RunPromptCtx(ctx, "Summarize this project", &muse.RunOptions{Workspace: workspace})
if err != nil { return err }
fmt.Println(result.Text)
```

`StreamPrompt` exposes raw JSONL events. Drain both channels, or cancel the context; task errors and the final turn result are distinct. `RunPromptCtx` requires a completed terminal record. Environment, working directory and explicit executable path are available on `Client`; don't mutate a client concurrently with a call.

## Long-lived sessions

```go
host, err := client.StartMSP(ctx, muse.HostOptions{Provider: "echo", NoSessionLog: true})
if err != nil { return err }
defer host.Close(context.Background())
go func() {
    for event := range host.Notifications() { fmt.Println(event.Method, string(event.Params)) }
}()
session, err := host.StartSession(ctx, muse.SessionOptions{WorkspaceRoot: workspace})
if err != nil { return err }
turn, err := session.Send(ctx, []muse.InputPart{muse.Text("hello")})
if err != nil { return err }
result, err := turn.Wait(ctx)
if err != nil { return err }
if result.Terminal != "completed" { return fmt.Errorf("turn ended: %s", result.Terminal) }
```

`Send` returns after admission; `Wait` returns the eventual terminal. `Notifications()` is a single host-wide stream: start draining it before sending turns. `Requests()` carries server-initiated approval and input requests and also must be drained when enabled. Queues are bounded; overflow closes the host with an error instead of silently losing events. `view/gap` is exposed to the caller; use `ViewPage` and protocol recovery or report incomplete history. `ResumeSession` returns the native session and raw history; ephemeral (`NoSessionLog`) sessions cannot be resumed after shutdown.

Requests require an explicit `Request.Respond` receipt once the application accepts responsibility for presentation. That receipt does not approve a tool or answer a question: use `Session.DecideApproval` or `Session.AnswerInput` separately. User input dialogs default to disabled. Never auto-approve unknown choices. See [MSP details](docs/MSP.md).

Use `just --list`, `just check`, and `just test race`. `go run ./examples/echo` exercises `exec` and MSP with the echo provider; `MUSE_TEST_DURABLE=1` additionally tests restart and resume. Use isolated configuration for smoke runs; the examples do not contact a model but Muse may still read user configuration unless the environment is isolated.

## Current limits

The SDK exposes generic MSP `Call`/`Notify`, session start/resume, turn start/wait/cancel/interrupt, approval decision, input answer, leave, mode and view paging. It does not yet automatically recover `view/gap`, multiplex a per-turn event stream, or implement a socket daemon. Live approvals, questions, cancellation, gap recovery and non-macOS binaries have not been proven end-to-end. Raw protocol calls and the native policy remain authoritative. The SDK is not yet integrated into Obey.
