# SQLite Connection Pool Unbounded

| Field       | Value |
|-------------|-------|
| Severity    | High |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 6 (bound every resource -- database pools: MaxConns, MaxConnLifetime, MaxConnIdleTime) |

## Location

- `internal/localdb/connect.go:29` (sql.Open with no pool config)
- `internal/localdb/connect.go:36` (PRAGMA journal_mode=WAL)

## Description

`sql.Open("sqlite3", db.file)` returns a connection pool with unlimited
connections. No `SetMaxOpenConns`, `SetMaxIdleConns`, or `_busy_timeout` is
configured. WAL mode is enabled at line 36.

The database is accessed concurrently from multiple goroutines:

- Primary server: `InsertRecord`, `ExecuteCompaction`, `LatestRevision`
- Replication follower: `ReplicateTentativeRecord`, `ReplicateRecord`
- Snapshot worker: `FindAllRecordsForSnapshot`
- Client API (gRPC, per-request): `FindRecordsBy`, `FindRecordByRev`
- Heartbeat sender: indirect access

SQLite WAL mode allows concurrent readers but only one writer at a time.
Without `_busy_timeout`, SQLite returns `SQLITE_BUSY` immediately when a
second writer attempts to acquire the write lock. The `mattn/go-sqlite3` driver
translates this to a `database is locked` error.

## Trigger

Any concurrent write operations: primary writes + compaction, replication +
compaction, or any write during a client read transaction.

## Impact

`SQLITE_BUSY` errors cause write failures under normal concurrent load.
Replication records may be dropped, compaction may fail, or client transactions
may error.

## Suggested fix

Either:
1. `db.SetMaxOpenConns(1)` -- serializes all access through one connection
   (simplest).
2. Set `_busy_timeout` in the DSN: `db.file + "?_busy_timeout=5000"` --
   writers retry for up to 5s before failing, preserving read concurrency.
