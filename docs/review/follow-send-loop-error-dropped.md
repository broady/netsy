# Follow Stream Send-Loop Errors Silently Dropped

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 2 (errors: handle once -- never swallowed silently) |

## Location

- `internal/primary/follow.go:143-157`

## Description

After `followRecvLoop` returns, the code does a non-blocking `select` on
`sendErr` (lines 148-154 with a `default` case). If the send goroutine has not
yet finished, the `default` case fires and `Follow` returns `recvErr`. The
send-loop error is silently discarded.

The still-running `followSendLoop` goroutine will eventually exit when
`removeFollowStream` closes `session.sendCh`, but any error from the send loop
is lost.

## Impact

Errors on the replication send path are silently lost, making debugging
replication failures harder. Not data-corrupting, but reduces observability.

## Suggested fix

Wait for the send-loop goroutine before returning, or at minimum log the
send error if it occurs.
