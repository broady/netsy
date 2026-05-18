# preflightCancel Accessed Under Inconsistent Mutexes

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Concurrency reference (sync.Mutex -- the default for shared state) |

## Location

- `internal/primary/preflight.go:37-44` (startPreflightLocked, stopPreflight)
- `internal/primary/preflight.go:20-33,48-55` (runPreflight)

## Description

The `preflightCancel` field is protected by `preflightMu` in `runPreflight`'s
deferred cleanup (lines 49-55), but by `svcMu` in `startPreflightLocked` and
`stopPreflight`. These are different mutexes.

`StopServices()` (which holds `svcMu`) can run concurrently with
`runPreflight`'s deferred cleanup (which holds `preflightMu`). Both write
`preflightCancel = nil` concurrently -- under different locks.

## Impact

Data race detectable by `go test -race`. Practically benign (both write nil)
but violates the single-mutex-per-field invariant.

## Suggested fix

Consolidate on a single mutex for `preflightCancel`, or use
`svcMu` consistently for all accesses.
