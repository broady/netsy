# SetWatchAdmissionFloor TOCTOU Between Store and Validation

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Design hazard |
| Guide rule  | Concurrency reference (compound mutations -- the Get/Set gap) |

## Location

- `internal/watch/manager.go:240-260`

## Description

`SetWatchAdmissionFloor` stores the new floor value atomically (line 245), then
iterates watches to validate. During the validation window, a concurrent
`CreateWatch` call could check the floor (already set to the new value) and
reject a legitimate watch.

If validation subsequently fails and the floor is rolled back (line 253), the
rejected watch was erroneously denied. `mu.RLock` guards the watcher map
iteration but does not prevent concurrent `CreateWatch` calls.

## Impact

During compaction, a legitimate watch creation could be spuriously rejected.
The client would need to retry.

## Suggested fix

Perform validation before committing the new floor value, or use a write lock
that also blocks `CreateWatch` during validation.
