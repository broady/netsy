# "List All Keys" Range Request Produces Invalid SQL

| Field       | Value |
|-------------|-------|
| Severity    | High |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 8 (system boundary contracts -- validate at boundaries) |

## Location

- `internal/commonapi/range.go:60-62` (QueryParts)
- `internal/localdb/find.go:114` (FindRecordsBy)

## Description

When a client sends a Range request with `Key=\x00` and `RangeEnd=\x00` (the
standard etcd convention for "list all keys"), the branch at `range.go:60-62`
correctly identifies this case and comments `// no WHERE`, but leaves
`queryWhere` as the empty string `""`.

This empty string is passed to `FindRecordsBy`, which unconditionally formats
it as:

```go
whereClause := fmt.Sprintf("WHERE (%s)", whereQuery)
// produces: "WHERE ()"
```

`WHERE ()` is syntactically invalid SQL. The query fails.

## Trigger

Any "list all keys" request: `etcdctl get --prefix ""`, Kubernetes list
operations, controller reconciliation loops.

## Impact

The etcd-compatible "list all" API is broken. Any client (including
Kubernetes) that lists all keys receives an error.

## Suggested fix

Either:
1. In `FindRecordsBy`: check `whereQuery == ""` and omit the WHERE clause.
2. In `range.go`: set `queryWhere = "1=1"` in the all-keys branch so the SQL
   becomes `WHERE (1=1)`.
