# Temp File Leak in DownloadAndImportFile

| Field       | Value |
|-------------|-------|
| Severity    | High |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 2 (errors: handle once -- the cleanup error path is silently broken) |

## Location

- `internal/datastore/download.go:82-83`

## Description

```go
var tempFiles []string
defer cleanupTempFiles(logger, tempFiles) // captures nil slice
```

Go's `defer` evaluates function arguments eagerly. At registration time,
`tempFiles` is nil. The `Download` function later reassigns `tempFiles` via
`*tempFiles = append(*tempFiles, tempPath)`, which updates the caller's slice
variable -- but the deferred call already captured a copy of the nil slice
header. `cleanupTempFiles` iterates zero elements and never removes any temp
files.

## Trigger

Any data file download from object storage larger than 2MB (the threshold for
writing to a temp file).

## Impact

Temp files accumulate on disk indefinitely, eventually filling the volume.

## Suggested fix

Wrap the defer in a closure so `tempFiles` is read at execution time:

```go
defer func() { cleanupTempFiles(logger, tempFiles) }()
```
