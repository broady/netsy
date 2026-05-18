# Panic on Empty Key in Range QueryParts

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 8 (system boundary contracts -- validate at boundaries) |

## Location

- `internal/commonapi/range.go:49-52`

## Description

`keyCopy[:len(keyCopy)-1]` panics with index-out-of-range if `r.Key` is empty
(zero length), because `len(keyCopy)-1` evaluates to `-1`.

## Trigger

A `RangeRequest` with an empty `Key` field. While uncommon in normal etcd
usage, this is reachable via the gRPC API from any client.

## Impact

Server panic on a malformed client request. Process crashes if there is no
panic recovery middleware (and the production-go guide recommends against
adding one).

## Suggested fix

Validate `len(r.Key) > 0` at the top of `QueryParts` and return an appropriate
error for empty keys.
