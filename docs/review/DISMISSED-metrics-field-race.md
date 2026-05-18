# DISMISSED: s.server.metrics Data Race

| Field       | Value |
|-------------|-------|
| Severity    | N/A |
| Type        | N/A |
| Confidence  | Not real |

## Location

- `internal/elector/election.go:126-137` (SetMetrics)

## Original claim

`s.server.metrics` is written by `SetMetrics` (no lock) and read from
concurrent goroutines. This is a data race.

## Investigation result

**Dismissed.** `SetMetrics` is called exactly once, sequentially before
`Start()` (root.go lines 328-329). The `go` statement inside `Start()`
creates a happens-before edge per the Go memory model: the write is guaranteed
visible to all goroutines spawned transitively by `Start()`.

This is a safe write-once-before-go pattern. No lock is needed.
