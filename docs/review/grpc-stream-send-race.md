# gRPC Stream Send Data Race

| Field       | Value |
|-------------|-------|
| Severity    | Critical |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 3 (no naked goroutines), Concurrency reference (closure capture, shared state) |

## Location

- `internal/clientapi/etcdapi_watch_watch.go:61` (inbox dispatch goroutine)
- `internal/watch/watcher.go` lines 147, 152, 165, 170, 211, 216, 257, 262,
  278, 312 (CreateWatch and CancelWatch Send calls)

## Description

Two unsynchronized goroutines call `w.client.Send()` on the same gRPC
`ServerStream`:

1. **Inbox dispatch goroutine** -- reads from `w.inboxCh` and calls
   `w.Client().Send(&msg)` to deliver watch events.
2. **Main request goroutine** -- processes incoming `WatchRequest` messages and
   calls `w.client.Send()` directly inside `CreateWatch` (ack/rejection
   responses) and `CancelWatch` (cancel acknowledgment).

gRPC's `ServerStream.Send()` is explicitly documented as **not safe for
concurrent use**. The `Watcher.RWMutex` protects the `watches` map and
`inboxOk` flag but is never held around any `Send` call.

The comment at line 59-60 ("this should be the only goroutine sending messages
to the client") is incorrect.

## Trigger

Any `CreateWatch` or `CancelWatch` request arriving while events are being
dispatched from the inbox channel.

## Impact

Corrupted wire messages, panics inside the gRPC transport, or silent data
corruption on the watch stream.

## Suggested fix

Route ALL responses through `inboxCh` so the inbox dispatch goroutine is truly
the only sender. Alternatively, add a dedicated send mutex wrapping every
`w.client.Send()` call.
