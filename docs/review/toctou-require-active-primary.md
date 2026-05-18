# TOCTOU Between requireActivePrimary and leaderTxnMutex

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Partial |
| Guide rule  | Concurrency reference (compound mutations -- the Get/Set gap) |

## Location

- `internal/primary/leader_txn.go:66-71`

## Description

`requireActivePrimary()` is checked before acquiring `leaderTxnMutex`. Between
the check (line 66) and the lock acquisition (line 70), the Primary state can
transition to Draining (e.g., from chunk buffer full or `GracefulDrain`).

A transaction could be processed and committed while the node is already in
Draining state and should not accept new writes.

## Impact

During graceful drain, one or more extra writes could slip through. The window
is narrow but real.

## Suggested fix

Move the `requireActivePrimary()` check inside the `leaderTxnMutex` critical
section, or check the state again after acquiring the lock.
