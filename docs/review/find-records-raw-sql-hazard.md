# FindRecordsBy Accepts Raw SQL WHERE Clause

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Design hazard |
| Guide rule  | Rule 8 (system boundary contracts), Database reference (Querier interface) |

## Location

- `internal/localdb/find.go:108` (FindRecordsBy implementation)
- `internal/localdb/db.go:33` (Database interface)

## Description

`FindRecordsBy` accepts a `whereQuery string` parameter that is interpolated
into SQL via `fmt.Sprintf("WHERE (%s)", whereQuery)`. The `Database` interface
exposes this as a public method.

Currently safe: the single real caller (`internal/commonapi/range.go:95`)
builds `whereQuery` entirely from constant string literals with `?`
placeholders. All user data flows through `queryArgs`.

However, the interface provides no compile-time or runtime protection against
a future caller constructing `whereQuery` from user input.

## Impact

No current SQL injection risk. The API shape is fragile -- any future caller
that constructs `whereQuery` from user input would introduce a real
vulnerability.

## Suggested fix

Consider replacing the raw string interface with a structured query builder
or an enum of supported query patterns. At minimum, document the contract
explicitly: "whereQuery must contain only constant strings and ? placeholders."
