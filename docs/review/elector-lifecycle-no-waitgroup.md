# Elector Lifecycle Race -- No WaitGroup Between Epochs

| Field       | Value |
|-------------|-------|
| Severity    | High |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 3 (every goroutine is owned, cancelable, bounded, and waited) |

## Location

- `internal/elector/election.go:248-276` (onAcquireLeadership)
- `internal/elector/election.go:278-291` (onLoseLeadership)
- `internal/elector/nodemap.go:122-130` (Reset)
- `internal/elector/healthcheck.go:24-157` (runHealthCheckLoop, checkNodeHealth)
- `internal/elector/primary_election.go:24-144` (runPrimaryElectionLoop)
- `internal/elector/cluster_push.go:26-93` (pushClusterState)

## Description

`onAcquireLeadership` spawns three goroutines (Bootstrap, election loop, health
check loop) sharing a cancellable context. `onLoseLeadership` calls
`leaderCancel()` then **immediately** proceeds to `nodeMap.Reset()` and state
mutations with no WaitGroup or barrier.

The goroutines receive cancellation asynchronously but may be mid-operation:

- `checkNodeHealth` may be iterating the node map, making RPCs, or calling
  `pushClusterState` -- all of which involve multiple lock acquisitions.
- `electPrimaryOnce` may be in `checkPreviousPrimary` (blocks up to 5s in a
  retry loop) or `collectNodeStates` (parallel RPCs with 5s timeout).
- `Bootstrap` may be writing to object storage.

After `leaderCancel()`, `onLoseLeadership` immediately calls
`nodeMap.Reset()`, which clears all nodes and sets `ready=false`. Goroutines
still running from the prior epoch now operate on a cleared map, potentially:

- Pushing stale cluster state to peers after the node is a follower
- Deregistering nodes from object storage via cancelled context
- Adding entries to the node map after it was cleared (if Bootstrap is still
  running)

Individual `NodeMap` operations are mutex-protected (no Go-level data race),
but the **logical race** is that multi-step operations span multiple lock
acquisitions and `Reset()` can fire between any two steps.

## Trigger

s3lect leadership loss while health-check or election loop is mid-operation.

## Impact

Stale cluster state pushes from a former elector could interfere with the new
elector's decisions. Nodes may receive conflicting role assignments.

## Suggested fix

Add a `sync.WaitGroup` to `Runner`. Increment for each goroutine in
`onAcquireLeadership`. In `onLoseLeadership`, call `leaderCancel()` then
`wg.Wait()` before proceeding to `Reset()` and state mutations.
