# previousPrimary Data Race

| Field       | Value |
|-------------|-------|
| Severity    | High |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Concurrency reference (sync.Mutex -- the default for shared state) |

## Location

- `internal/elector/server.go:65` (field declaration)
- `internal/elector/primary_election.go:105,161-171,241` (writes from election goroutine)
- `internal/elector/healthcheck.go:154` (write from health-check goroutine)
- `internal/elector/election.go:76,286` (read and write from s3lect callback)

## Description

`previousPrimary` is a `nodestate.NodeInfo` struct containing `string` and
`uint64` fields. It is written by three different goroutines:

- `clearPrimary` (health-check loop goroutine)
- `electPrimaryOnce` (election loop goroutine)
- `onLoseLeadership` (s3lect callback goroutine)

And read by `checkPreviousPrimary` (election loop goroutine).

There is no mutex, atomic, or other synchronization protecting this field. Go
struct assignments are not atomic -- a torn read could produce a `NodeInfo`
with a `NodeID` from one write and a `PeerAdvertiseAddr` from another.

## Trigger

Health-check clearing the primary at the same time the election loop reads
`previousPrimary` in `checkPreviousPrimary`. Normal operation under any
elector.

## Impact

The election loop could contact the wrong node or fail with a confusing error
due to a torn read. Would be caught by `go test -race`.

## Suggested fix

Protect `previousPrimary` with a mutex, or use `atomic.Pointer[nodestate.NodeInfo]`
with pointer swaps.
