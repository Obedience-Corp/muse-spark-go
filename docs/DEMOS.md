# Demos

The demos follow the feature-oriented command layout of `claude-code-go`. They exercise the installed Muse executable through the SDK, using the local echo provider with isolated XDG configuration, cache, and data directories. No model credential or paid model call is needed. Muse's launcher auto-update is disabled for the child process.

| Command | Demonstrates |
| --- | --- |
| `just demo basic` | One-shot structured text and terminal result |
| `just demo streaming` | Runtime JSONL events and final result |
| `just demo sessions` | Per-turn MSP streaming and two turns in one session |
| `just demo resume` | Host restart, identical native session ID, durable history readback, another turn |
| `just demo cancel` | Native cancel and interrupt, each checked for a cancelled terminal |
| `just demo permissions` | Select `denyUnmatched` and read current pending approvals/input |
| `just demo errors` | Missing executable and cancelled-context error classification |

`just demo all` runs the complete suite. `just demo sessions --interactive` accepts prompts until `/quit`; Ctrl-C cancels the demo. All runs have a two-minute outer deadline. A demo exits nonzero when an expected outcome is absent. Native fixtures are removed on exit, so the resume demo deliberately performs both host lifetimes in one run.

Run without Just:

```sh
go run ./examples/demo -demo sessions -prompt 'Hello from Go'
go run ./examples/demo -demo sessions -interactive
go run ./examples/demo -demo all -muse /absolute/path/to/muse
```

## Recording

![Sessions and durable resume](demos/sessions.gif)

This recording uses Muse 1.4.14 on macOS arm64. It shows streamed echo text, completed terminals, host restart, native ID verification, and persisted history replay. It contains real command output; no provider response was inserted into the recording.

```sh
# Requires VHS, ffmpeg, a browser supported by VHS, Go, and Muse.
just demo record
```

The source is [demos/muse.tape](demos/muse.tape). The recording is checked into the repository, so the README does not depend on an external media host.

## Container verification

Use a Linux Muse binary matching your Docker architecture. CI downloads the checksum-pinned public release via `scripts/fetch-muse.sh`. To run an already available binary:

```sh
go mod download
just test native /absolute/path/to/linux/muse
```

Source, binary, and module cache are mounted read-only. Docker networking is disabled. Configuration and durable session writes stay inside the disposable container. The Linux arm64 run passed all seven demos; its [transcript](demos/linux-echo.txt) records the results.

The permissions demo proves mode selection and pending-request readback. It does not generate a native tool approval or question: echo does not call tools. Receipt, stale-requirement errors, choice/requirement identifiers, and question-answer identifiers are covered by the subprocess protocol fixture. Model-generated approval/input UI flows require separate controlled integration proof.
