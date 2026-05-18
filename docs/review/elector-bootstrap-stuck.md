# Elector Bootstrap Failure Permanently Prevents Primary Election

| Field       | Value |
|-------------|-------|
| Severity    | High |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 6 (bound every resource -- retry loops need max attempts or deadline) |

## Location

- `internal/elector/election.go:265-271` (onAcquireLeadership)
- `internal/elector/bootstrap.go:22-60` (Bootstrap)

## Description

`onAcquireLeadership` launches a goroutine that calls `Bootstrap(ctx)`. If
Bootstrap fails (any object storage error), the goroutine logs and returns.
`runPrimaryElectionLoop` is never started.

The callback already returned `nil` to s3lect, so the lease is held
indefinitely. s3lect's election loop continues renewing the leader record in
S3. Other nodes see an active leader and do not attempt to take over.

The health-check loop starts unconditionally but is inert because
`nodeMap.Ready()` is never set (Bootstrap is what calls `nodeMap.SetReady()`).

The cluster is permanently stuck with no Primary until manual intervention
(killing the elector node).

## Trigger

Transient object storage failure during elector bootstrap: S3 throttling,
network blip, GCS token refresh failure. `Bootstrap` can fail from
`ReadMembersFile`, `ListNodeRegistrations`, `bootstrapFirstElector`, or
`bootstrapExisting` -- all object storage operations.

## Impact

Cluster permanently unable to accept writes. No automatic recovery.

## Suggested fix

Either:
1. Return the Bootstrap error synchronously from `onAcquireLeadership` so
   s3lect resigns leadership and another node can take over.
2. Retry Bootstrap with backoff until it succeeds or the context is cancelled.
3. Actively resign leadership (`leaderCancel()` + trigger `onLoseLeadership`)
   when Bootstrap fails, allowing another node to take over.
