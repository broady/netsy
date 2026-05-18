# Panic on Empty rangeKey in Watch isInRange

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 8 (system boundary contracts -- validate at boundaries) |

## Location

- `internal/watch/watcher.go:427-430`

## Description

`rangeKeyCopy[:len(rangeKeyCopy)-1]` panics if `rangeKey` is empty (zero
length). The guard at line 424 checks `len(key) > 0` (the record key), not
`len(rangeKey)` (the watch key). A non-empty record key with an empty watch
range key causes a panic.

## Trigger

A watch entry created with an empty key, followed by any record distribution.

## Impact

Server panic on the write path (Distribute calls isInRange). Process crashes.

## Suggested fix

Add a length guard: `if len(rangeKey) == 0 { return false }` at the top of
`isInRange`.
