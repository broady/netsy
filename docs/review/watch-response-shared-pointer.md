# Watch Response Shared Pointer Corruption

| Field       | Value |
|-------------|-------|
| Severity    | Critical |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 11 (copy mutable data at ownership boundaries) |

## Location

- `internal/watch/manager.go:87-142` (Distribute)

## Description

`Distribute()` creates a single `*mvccpb.Event` and wraps it in a
`[]*mvccpb.Event` slice inside a `pb.WatchResponse` struct. For each matching
watch, the code mutates `msg.Events[0].PrevKv` and `msg.WatchId`, then sends
`msg` by value on `w.inboxCh`.

Sending a struct by value copies the slice header but not the backing array.
Every queued `WatchResponse` copy shares the **same `*mvccpb.Event` pointer**.
When the next matching watch modifies `msg.Events[0].PrevKv` (setting it to nil
for a watch without `prevKv`, or to the previous value for one with it), it
mutates the Event visible to all previously-queued, unconsumed messages.

## Trigger

Two watches on the same watcher match the same key, one with `prevKv=true` and
one with `prevKv=false`. The second iteration corrupts the first message's
`PrevKv` field.

## Impact

Watch clients silently receive incorrect `PrevKv` values -- missing when
expected, or present when not requested.

## Suggested fix

Create a shallow copy of the Event for each watch iteration:

```go
ev := *msg.Events[0]
if watch.prevKv {
    ev.PrevKv = msgPrevKv
} else {
    ev.PrevKv = nil
}
outMsg := msg
outMsg.Events = []*mvccpb.Event{&ev}
w.inboxCh <- outMsg
```
