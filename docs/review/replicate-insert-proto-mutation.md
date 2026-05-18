# ReplicateRecord and InsertRecord Mutate Caller's Protobuf

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 11 (copy mutable data at ownership boundaries) |

## Location

- `internal/localdb/replicate.go:36` (`record.ReplicatedAt = timestamppb.Now()`)
- `internal/localdb/insert.go:63` (`record.CreatedAt = timestamppb.Now()`)

## Description

Both functions modify the caller's `*proto.Record` in place before performing
the database operation. If the database operation fails, the caller's record
has been permanently mutated with timestamps that were never persisted.

The replication pipeline may still reference this proto after the call.

## Impact

If a caller retries a failed replicate/insert using the same record pointer,
the record now has stale timestamps from the first failed attempt. If the
caller logs or inspects the record after failure, it shows misleading state.

## Suggested fix

Either copy the record before mutation, or set the timestamps only after a
successful database operation (requires restructuring the query to use
`CURRENT_TIMESTAMP` or similar in SQL).
