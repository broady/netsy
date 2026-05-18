# DISMISSED: s.lastWritePath Data Race

| Field       | Value |
|-------------|-------|
| Severity    | N/A |
| Type        | N/A |
| Confidence  | Not real |

## Location

- `internal/primary/leader_txn.go:98-106`

## Original claim

`s.lastWritePath` is written under `leaderTxnMutex` but it's unclear if it's
always read under the same lock.

## Investigation result

**Dismissed.** Every read and write of `lastWritePath` occurs inside
`LeaderTxn`, after `leaderTxnMutex.Lock()` is acquired at line 70. There are
zero other access sites anywhere in the codebase (confirmed by project-wide
grep). The field is fully protected by `leaderTxnMutex`.
