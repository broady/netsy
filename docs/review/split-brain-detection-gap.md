# Split-Brain Detection Gap -- Multiple Active Primaries Not Fenced

| Field       | Value |
|-------------|-------|
| Severity    | High |
| Type        | Correctness |
| Confidence  | Design hazard |
| Guide rule  | Rule 8 (system boundary contracts -- document invariants) |

## Location

- `internal/elector/primary_election.go:421-445` (findActivePrimary)

## Description

When `findActivePrimary` detects multiple nodes reporting `PrimaryActive`, it
logs the error but returns `(NodeInfo{}, false)`. This falls through to
`checkNonReplicaStates` which fails because there are non-degraded nodes with
non-Replica primary state. The election retries in 500ms.

Meanwhile, both Active nodes continue serving writes independently. The code
does not attempt to fence off the stale Primary (no epoch/generation counter,
no STONITH-style mechanism).

This situation can occur after a network partition heals -- the old Primary
never saw the drain signal and continued operating.

## Trigger

Network partition heals after a new Primary was elected. The old Primary still
reports `PrimaryActive`.

## Impact

During the window between detecting split brain and the next successful
election (which may never succeed if both nodes keep reporting Active), clients
may write to two different Primaries simultaneously. Data divergence requires
manual intervention.

## Suggested fix

Implement an epoch or fencing token in the replication protocol. When a new
Primary is elected, it increments the epoch. Writes with a stale epoch are
rejected by replicas. Alternatively, the elector should actively drain/stop the
stale Primary before electing a new one.
