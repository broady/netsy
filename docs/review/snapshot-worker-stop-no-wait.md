# Snapshot Worker Stop Does Not Wait for In-Flight Work

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 3 (every goroutine owned, cancelable, waited), Rule 5 (graceful shutdown is mandatory) |

## Location

- `internal/snapshot/worker.go:62` (NewWorker uses context.Background)
- `internal/snapshot/worker.go:78-85` (Stop)

## Description

`NewWorker` creates `context.WithCancel(context.Background())` -- entirely
disconnected from the server's lifecycle context. `Start()` launches `go
w.run()` but `Stop()` only cancels the context with no WaitGroup to ensure
in-flight work completes.

`createSnapshot` is a multi-step I/O operation: DB read, temp file write,
upload to object storage, chunk cleanup. If `Stop()` is called mid-snapshot,
the caller (shutdown sequence) may close the database or storage client while
the goroutine is still using them.

The `requestCh` channel is never closed.

## Impact

Panics or data corruption if the database or storage client is closed while a
snapshot upload is in progress.

## Suggested fix

Add a `sync.WaitGroup` or `errgroup`. Increment before launching the goroutine,
wait in `Stop()`. Accept a parent context instead of creating one from
`context.Background()`.
