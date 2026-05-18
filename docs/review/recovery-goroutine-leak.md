# Recovery Goroutine Loops Forever With No Cancellation

| Field       | Value |
|-------------|-------|
| Severity    | High |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 3 (every goroutine owned, cancelable, waited), Rule 6 (retry loops need max attempts or deadline) |

## Location

- `internal/primary/leader_txn.go:322-357` (startObjectStorageRecovery)

## Description

`startObjectStorageRecovery` spawns a goroutine with `context.Background()` in
an infinite `for {}` loop with `time.Sleep` backoff. The goroutine retries
uploading a record to object storage until it succeeds.

There is:
- No parent context (uses `context.Background()`)
- No cancellation mechanism
- No retry bound or deadline
- No owner that can stop it or wait for it
- No tracking in `StopServices()`

When the node loses Primary status, this goroutine keeps running. Multiple
leadership transitions accumulate multiple leaked goroutines, each hammering
object storage indefinitely.

## Trigger

Object storage upload failure during a write transaction, followed by shutdown
or leadership loss.

## Impact

- Goroutines survive shutdown, making external calls after the process should
  have stopped.
- Multiple leaked goroutines accumulate across leadership transitions.
- Process cannot exit cleanly if these goroutines hold resources.

## Suggested fix

Use the server's lifecycle context instead of `context.Background()`. Track the
goroutine in an errgroup or WaitGroup so `StopServices()` can cancel and wait.
Add a max retry count or deadline.
