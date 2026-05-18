# degradeSelf TOCTOU Across Parallel Goroutines

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Partial |
| Guide rule  | Concurrency reference (compound mutations -- the Get/Set gap) |

## Location

- `internal/heartbeat/sender.go:266-290`

## Description

`degradeSelf` is called from two parallel goroutines: `sendToElector` and
`sendToPrimaryIfNeeded` (lines 121-122 in `runLoop`). Both can fail and call
`degradeSelf` concurrently.

The function performs a sequence of non-atomic reads:

1. `s.state.Health()` (line 267) -- read health
2. `s.state.Primary()` (line 271) -- read primary state
3. `s.state.SetHealth(Degraded)` (line 273) -- write health

Each operation acquires and releases `nodestate.State.mu` independently.
Between the read at line 271 and the callback at line 287, the primary state
can transition, causing the callback to fire based on stale state.

The `Degraded -> Degraded` transition guard provides a partial safety net for
the second concurrent call, but the `wasPrimary` read is still racy for the
first call.

## Impact

Could trigger the drain-flush-resign sequence (`onPrimarySelfDegrade`) on a
node that has already transitioned away from Primary.

## Suggested fix

Hold a single lock across the entire degradeSelf operation, or use a
compare-and-swap pattern for the state transition.
