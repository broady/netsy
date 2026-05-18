# Range Key Append Mutates Caller's Backing Array

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 11 (copy mutable data at ownership boundaries) |

## Location

- `internal/commonapi/range.go:46` (`append(r.Key, byte(0))`)
- `internal/commonapi/range.go:77` (`append(r.Key, byte(37))`)

## Description

`append(r.Key, byte(0))` writes past `len(r.Key)` in the backing array when
spare capacity exists. Protobuf deserialization typically produces slices with
spare capacity due to memory allocator size classes (e.g., a 5-byte key gets
cap=8).

The code at line 47-48 already makes a defensive copy for `keyCopy`,
demonstrating awareness of the problem -- but lines 46 and 77 were missed.

Currently no caller is harmed because the `RangeRequest` is not reused after
`Range()` returns. However, this is a latent correctness bug that would
surface if any caller retains the request.

## Impact

No current impact. Latent -- would cause silent data corruption if a future
caller reuses the `RangeRequest` or shares the key slice.

## Suggested fix

Replace `append(r.Key, byte(0))` with an explicit copy-and-append:

```go
keyAndZeroByte := make([]byte, len(r.Key)+1)
copy(keyAndZeroByte, r.Key)
```
