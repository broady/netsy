# datafile Reader Never Closes zstd Decoder

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 6 (bound every resource) |

## Location

- `internal/datafile/reader.go:153` (Close method)

## Description

`Reader.Close()` validates the footer and CRCs but never calls
`r.decompressor.Close()`. The `zstd.Decoder` documentation states that
`Close()` releases all resources and must be called. Each `NewReader` on a
zstd-compressed file allocates a decoder with internal buffers that are never
freed.

## Trigger

Reading any compressed data file. Accumulates during bootstrap backfill or
snapshot restores that read many files.

## Impact

Memory leak proportional to the number of data files read. Under sustained load
(e.g., bootstrap from a large cluster), this can cause OOM.

## Suggested fix

Call `r.decompressor.Close()` in `Reader.Close()`.
