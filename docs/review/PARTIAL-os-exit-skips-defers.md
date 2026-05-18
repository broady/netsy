# PARTIAL: os.Exit Throughout root.go Bypasses Deferred Cleanups

| Field       | Value |
|-------------|-------|
| Severity    | Low |
| Type        | Correctness |
| Confidence  | Partial |
| Guide rule  | Rule 9 (no log.Fatal, os.Exit outside main), Rule 5 (graceful shutdown) |

## Location

- `internal/cmd/root.go` lines 113, 123, 218, 227, 231, 236, 240, 244, 257,
  267, 313, 327, 332, 359, 377, 388, 448, 478, 491

## Original claim

`jitterWaitThenExit` calls `os.Exit(1)` from 8+ locations, skipping every
deferred cleanup (health server, storage, peer manager, snapshot worker,
context cancel).

## Investigation result

**Partially correct but overstated.** All `os.Exit`/`jitterWaitThenExit` calls
occur during **startup initialization** (before the server is serving traffic
or has established meaningful state). This is a deliberate crash-restart
pattern for a distributed system node: log the error, wait a random 0-9
seconds (to avoid thundering herd), then exit.

At most early exit points, only 1-3 deferred cleanups exist. The **graceful
shutdown path** (signal handling, lines 522-571) correctly runs all defers.

**Minor gap:** `db.Close()` is never deferred anywhere in root.go.

Severity is Low because the startup-crash pattern is intentional and the
graceful shutdown path works correctly.
