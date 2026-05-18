# ApplyClusterState Not Atomic -- Interleaving Causes Lost Transitions

| Field       | Value |
|-------------|-------|
| Severity    | High |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Concurrency reference (compound mutations -- the Get/Set gap) |

## Location

- `internal/peerclient/manager.go:86-129` (ApplyClusterState)
- `internal/nodestate/state.go` (ClusterState, SetClusterState, Primary, SetPrimary)
- `internal/node/server.go:93-114` (PushClusterState gRPC handler)
- `internal/elector/cluster_push.go:26-93` (pushClusterState)

## Description

`ApplyClusterState` performs a read-modify-write sequence across multiple
independently-locked operations: `ClusterState()`, `SetClusterState()`,
`Primary()`, `SetPrimary()`. Each acquires and releases `nodestate.State.mu`
independently.

Three unserialized call sites can trigger concurrent execution:

1. `runPrimaryElectionLoop` (election goroutine)
2. `runHealthCheckLoop` -> `clearPrimary` (health check goroutine)
3. `DeregisterNode` -> `clearPrimary` (gRPC handler goroutine)

Concurrent calls can interleave, causing:

- **Lost primary transitions:** Both calls read `Primary()==Replica`, both try
  `SetPrimary(PrimaryStarting)`. One succeeds, the other reads
  `PrimaryStarting` and reverts to `PrimaryReplica`.
- **Duplicate `onPrimaryChange` callbacks:** Both calls compute
  `wasPrimary`/`isPrimary` from stale `old` snapshots, both fire the callback.
- **Stale connection decisions:** `updatePrimary` is called based on a stale
  `old` snapshot that no longer matches the stored state.

## Trigger

Health-check clear and election push arriving within microseconds of each other
-- a normal scenario during elector operation.

## Impact

A node could incorrectly believe it is Primary when it is not (or vice versa),
leading to split-brain writes or failed replication.

## Suggested fix

Hold a single lock across the entire `ApplyClusterState` operation. Add an
`applyMu sync.Mutex` to `Manager` and lock it for the duration of the method.
