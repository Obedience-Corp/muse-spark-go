# MSP lifecycle, events, and recovery

## Process ownership

`StartMSP` launches `muse serve`, performs `initialize`, checks schema version 1, and sends `initialized`. Its context controls startup only. The returned host remains alive until `Close` or transport failure. Fingerprint differences appear in `Warnings`; an incompatible schema version fails initialization.

`Close` closes stdin and waits for exit. A context without a deadline gets a two-second grace period. Expiry terminates and reaps the child and returns the context error. Concurrent and repeated close calls are safe. Always close a successfully started host, including after a turn error.

Protocol stdout and diagnostic stderr are separate. Stderr is discarded unless `Client.Stderr` or `HostOptions.Stderr` supplies a writer; host configuration overrides the client writer. Diagnostics may contain prompts, paths, or provider details. Shared writers must be concurrency-safe and should not block indefinitely. Error strings do not include captured stderr or argv.

Writes are serialized complete JSON-RPC frames. Cancellation while waiting for the writer leaves the host usable. Cancellation during a pipe write closes the host because the peer may have received a partial frame. `Call` timeout after sending stops waiting without retracting the native command. Request outcomes may be unknown after a disconnect; do not blindly resubmit a side effect under a fresh command ID.

## Turn ownership

`Send` registers observation before sending `turn/start`. Notifications can precede the admission reply. A `Turn` takes its ID from that reply, including when the host queues or steers the input. `TurnOptions.CommandID` accepts an explicit UUIDv7 for deliberate idempotent retry; omission generates one. The SDK performs no retries.

`Send` tracks terminal state without collecting a user event stream. For streamed events use `SendWithOptions` with `Stream: true` and drain `Events()` while the turn runs. `Wait` is repeatable, concurrent-safe, and returns copied raw result bytes. A cancelled wait leaves tracking and the native turn active. Use `Cancel` or `Interrupt` to request a native action, then wait for its outcome. `CloseTracking` stops only local observation.

`turn/completed` is terminal. `turn/foregroundCompleted` is not. `turn/unqueued` settles tracking with terminal `unqueued`: no completion event will follow for that reclaimed queued turn. Unknown terminal strings remain visible and are never converted to success.

Each turn correlates nested item IDs with its native turn ID. An `item/delta` has no turn ID; the preceding item record supplies that association. `item/completed` contains the authoritative item revision and final text. Replace that item's content; do not append final text to already applied deltas. Unknown notifications retain raw JSON. Use a session subscription for session-scoped events or future events that cannot be associated with a turn.

## Independent subscriptions

`Subscribe(sessionID, capacity)` registers a local event subscription. An empty ID selects all sessions, and nonpositive capacity selects 256 events. Register before commands to observe early events. Close subscriptions when done. A subscription does not issue a remote `view/subscribe` command.

The reader never waits on application consumption. An overflowing subscription closes with an error wrapping `ErrOverflow`; healthy subscriptions and the host continue. Buffered events remain readable. Check `Subscription.Err()` after draining. A streamed turn has a separate bounded event buffer; overflow ends local tracking with `ErrOverflow`, not the native turn. Without `Stream: true`, no user event drain is required.

`Notifications()` remains a single-consumer compatibility API and activates on first call. Prefer `Subscribe`, whose independent error surface makes loss explicit.

Server requests use a separate 32-request queue. Overflow fails the host rather than dropping requests that require acknowledgement. Inbound and outbound frames are bounded at 8 MiB. Malformed envelopes and oversized input terminate the transport and release waiters.

## Approval and user-input requests

1. Receive a request from `host.Requests()` and decode `Approval()` or `Input()`.
2. Accept responsibility for presenting it, then call `RespondContext(ctx, nil)`. The receipt is `{}` and is allowed once per received request. Unsupported request methods should receive a JSON-RPC error, such as code `-32601`.
3. Present the offered choices or questions. Do not invent choice IDs, interpret human labels as policy, or select a default permission automatically.
4. Submit `DecideApproval(ctx, approval.ApprovalID, selectedChoiceID, approval.CurrentRequirementID)`, or `AnswerInput(ctx, input.UserInputID, answers)` using the exact question IDs and option labels.
5. If the requirement is stale, inspect structured remote error data, refresh pending requests, and present the current requirement. Do not reuse the old choice blindly.

`Request.ID` identifies the presentation RPC on this connection. `approvalId`, `currentRequirementId`, `userInputId`, and question IDs identify native decisions; they are different namespaces. Later stages and reconnects can reissue presentation requests. The SDK deduplicates a receipt for the same received request object, not separate native deliveries.

`Pending` returns the host's current approval and input payloads. `CancelInput` explicitly cancels a prompt. User-input dialogs default to false; only enable them when an application can service the request queue. The echo provider cannot prove actual tool approvals or question settlements. Those wire paths have protocol-fixture coverage; native UI approval/input proof remains an integration gate.

## Durable view recovery

Cursors are opaque. `ReadViewPage` returns typed paging metadata, raw events, and `NextCursor == nil` at the end. `ReplayView(ctx, after, visitor)` reads forward from an observed cursor and rejects cyclic/no-progress pagination. Visitor errors and cancellation stop replay. Use a deadline on an actively growing view.

`view/gap` includes exclusive `after` and `next` bounds. Affected turn tracking fails with `ErrGap` and retains those bounds in `Error.Data`. A normal subscription receives the raw gap event. Durable pages contain completed item revisions and terminals, not every ephemeral delta.

Recovery belongs to the application's view reducer:

1. Keep the last fully applied durable cursor and item revisions.
2. Start buffering the session's live subscription before fetching history.
3. Replay durable history from the last applied cursor, applying item revisions authoritatively.
4. Reconcile buffered live records by event identity and item revision; multiple different notifications can share one cursor, so cursor-only deduplication is incorrect.
5. If an earlier terminal was lost, observe it in replay rather than resubmitting the model turn.

`SubscribeView` can request native replay after an observed cursor. The SDK does not automatically merge that replay into a caller's UI state or claim ephemeral deltas are recoverable. Raw events and cursor-based paging support applications with different projection models.
