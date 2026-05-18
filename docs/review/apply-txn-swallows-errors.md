# ApplyTxn Silently Swallows All LeaderTxn Errors

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 2 (errors: handle once -- never swallowed silently), Error reference (boundary error mapping) |

## Location

- `internal/clientapi/etcdapi_kv_txn.go:113`

## Description

`ApplyTxn` always returns `nil` error (line 113: `return resp, nil`) even when
`LeaderTxn` fails with a real error (database corruption, I/O failure, etc.).
The error is logged but the gRPC client receives a `TxnResponse` with
`Succeeded=false` (zero value) and whatever revision could be retrieved.

The client cannot distinguish between a legitimate precondition failure
(`Succeeded: false` with a correct response) and a serious internal error
(`Succeeded: false` because something broke).

## Impact

Clients checking `err != nil` believe every write succeeded. Internal errors
are invisible to the application layer.

## Suggested fix

Return a gRPC status error (e.g., `codes.Internal`) for unexpected errors from
`LeaderTxn`. Only return `nil` error for semantically-valid outcomes
(successful write, compare-revision failure).
