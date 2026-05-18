# Watch Key Append Data Race Under Read Lock

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Concurrency reference (closure capture, shared state under locks) |

## Location

- `internal/watch/watcher.go:422` (`append(rangeKey, byte(0))` in isInRange)

## Description

`isInRange` is called from `Distribute()`, which holds only `w.RLock()` (a
read lock). However, `append(rangeKey, byte(0))` performs a **write** to the
backing array of `rangeKey` when spare capacity exists. `rangeKey` originates
from `watchEntry.key`, which is stored directly from the `WatchCreateRequest`
protobuf field without copying.

Multiple concurrent `Distribute()` calls are allowed (both acquire
`m.mu.RLock()`), so two goroutines can write to the same backing array
location simultaneously. Under the Go memory model, concurrent unsynchronized
writes -- even of the same value -- are undefined behavior.

## Impact

Would be caught by `go test -race`. No functional corruption in current usage
(all writes are idempotent `0x00`), but it is formally a data race.

## Suggested fix

Make a defensive copy before appending:

```go
rangeKeyAndZeroByte := make([]byte, len(rangeKey)+1)
copy(rangeKeyAndZeroByte, rangeKey)
```

Also copy the key at watch creation time (`watcher.go:230`) so the watch entry
owns its own slice.
