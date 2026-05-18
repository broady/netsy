# Fire-and-Forget Flush Goroutine on Quorum Rollback

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 3 (every goroutine owned, cancelable, waited) |

## Location

- `internal/primary/leader_txn.go:259-265`

## Description

When quorum fails, a goroutine is spawned to flush the chunk buffer using
`context.Background()`. This goroutine is fire-and-forget: no owner, no
cancellation, not waited on during shutdown.

If shutdown occurs immediately after a quorum rollback, this goroutine may
perform I/O against object storage after the server has stopped services.

## Impact

During shutdown, this goroutine may interfere with clean teardown or produce
errors. Less severe than the recovery loop (one-shot, not infinite), but still
a leaked goroutine.

## Suggested fix

Use the server's lifecycle context. Track the goroutine so `StopServices()`
can wait for it.
