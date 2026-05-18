# Compare-Failure Counted as Success in Write Metrics

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Observability reference (metrics must reflect actual outcomes) |

## Location

- `internal/primary/leader_txn.go:158-161`

## Description

When `InsertRecord` returns `ErrCompareRevisionFailed` and there is a failure
range operation, the code rolls back the transaction, executes a Range query,
and falls through to lines 158-161 which unconditionally increment
`WriteTransactions.WithLabelValues(pathLabel, "success")`.

A compare-revision failure is semantically a conflict -- the client's
optimistic concurrency check failed. While the etcd API returns this as a
non-error response (`Succeeded: false`), counting it as a "success" write
inflates the success metric.

## Impact

Under contention, metrics show success when no data was written. The true
write-vs-conflict ratio is hidden.

## Suggested fix

Use a distinct label like `"compare_failed"` or `"conflict"` for the
compare-revision failure path.
