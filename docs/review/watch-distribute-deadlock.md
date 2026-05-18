# Watch Distribute Deadlock

| Field       | Value |
|-------------|-------|
| Severity    | Critical |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 3 (no naked goroutines), Rule 6 (bound every resource) |

## Location

- `internal/watch/manager.go:126-142` (Distribute)
- `internal/watch/watcher.go:97-118` (Cleanup)
- `internal/clientapi/etcdapi_watch_watch.go:46-66` (inbox goroutine)

## Description

`Distribute()` acquires `m.mu.RLock()`, then iterates all watchers. Inside the
loop it calls `w.RLock()` with `defer w.RUnlock()`. Because `defer` is
function-scoped (not loop-scoped), **all watchers' RLocks accumulate** and are
held simultaneously until `Distribute` returns.

The loop body performs a **blocking send** on `w.inboxCh` (buffered at 64) with
no select/timeout/default. If the channel is full, `Distribute` blocks while
holding every lock.

When a client disconnects, the inbox dispatch goroutine exits (it was the
channel consumer). Subsequent events fill the channel. Once full:

1. `Distribute` blocks on `w.inboxCh <- msg`, holding `m.mu.RLock()` and
   `w.RLock()` on every watcher iterated so far.
2. `Cleanup` (called when the watch handler detects the disconnect) needs
   `w.Lock()` -- blocked by Distribute's `w.RLock()`.
3. `Cleanup` also calls `m.Unregister(w.id)` which needs `m.mu.Lock()` --
   blocked by Distribute's `m.mu.RLock()`.
4. Nothing can drain the channel. Nothing can release the locks.

**Permanent deadlock.** The entire write path hangs because `Distribute` is
called synchronously from the write commit path.

## Trigger

A single watch client that disconnects or becomes slow enough to let 64 events
queue up.

## Impact

All writes to the cluster block permanently until the process is killed.

## Suggested fix

1. Move `w.RLock()`/`w.RUnlock()` into the loop body (not deferred) so each
   watcher's lock is released before the next.
2. Replace the blocking send with a non-blocking `select` with a `default` case
   that drops the event and marks the watcher for cleanup, or use a
   context-aware send with a timeout.
