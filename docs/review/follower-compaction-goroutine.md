# Follower Compaction Goroutine Has No Context or Shutdown Coordination

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Partial |
| Guide rule  | Rule 3 (every goroutine owned, cancelable, waited) |

## Location

- `internal/replication/follower.go:412-433` (handleCompact)
- `internal/primary/compaction.go:207` (same pattern on primary side)

## Description

`handleCompact` launches `go func()` that calls `ExecuteCompaction`. The
goroutine has no context, no concurrency bound, and no shutdown coordination.
`Stop()` cancels the follower's context but has no WaitGroup to wait for
in-flight compaction goroutines.

The same pattern exists on the primary side at `compaction.go:207`.

## Trigger / frequency

Compaction messages arrive ~once per 5 minutes (default `CompactionInterval`).
Unbounded accumulation is unlikely in practice -- each `ExecuteCompaction` is a
single SQLite UPDATE that typically completes in milliseconds.

## Impact

The primary risk is unclean shutdown: if a compaction goroutine is running when
`Stop()` is called, it may access the database after other shutdown steps have
closed it. The "unbounded goroutine" claim is overstated given the call
frequency, but the lack of shutdown coordination is real.

## Suggested fix

Accept a context and use the follower/primary lifecycle context. Track the
goroutine in a WaitGroup so `Stop()` can wait for it.
