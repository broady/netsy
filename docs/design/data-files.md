---
title: "Netsy Data Files"
weight: 30
description: "Netsy (.netsy) data file format/specification"
---

# Netsy Data Files

A `.netsy` data file is a varint size-delimited Protocol Buffer messages file with optional body+footer compression, consisting of:

* Header: 1x Header message (always uncompressed)
* Body: 1+ Record message(s) (compressed or uncompressed based on header)
* Footer: 1x Footer message (compressed or uncompressed based on header)

There are two kinds of Netsy data files:
* Snapshot files - containing a complete snapshot of KV records (always compressed when created by Netsy).
* Chunk files - containing a set of new records not yet captured in a Snapshot (compressed when created by Netsy only if >4KB of key+value data for all records combined).

The compression type is specified in the header's `compression` field (`COMPRESSION_NONE` or `COMPRESSION_ZSTD`). External systems can create uncompressed snapshot files for easier implementation.

The file is written using the [google.golang.org/protobuf/encoding/protodelim](https://google.golang.org/protobuf/encoding/protodelim) package.

## CRCs

We use CRC64 to protect against accidental corruption like bit rot, network errors, S3 silent failures.
It does not defend against malicious tampering of S3 objects

CRC64 advantages:

   * Detects all single-bit errors
   * Detects burst errors up to 64 bits
   * Collision probability: 1 in 2^64 for random data
   * Much faster chunk upload/validation
   * 8 bytes vs 32 bytes (smaller files)

We use CRC's in 4 places:
  * Header struct contents CRC
  * Record struct contents CRC
  * Footer struct contents CRC
  * All-Records field in the Footer struct (a CRC of all Record structs in the File)

Per-Record CRC protects against:
   * Corrupted Record content
   * Individual Record bit rot

All-Records CRC protects against:
   * Missing records: All individual CRCs valid, but records 500-600 disappeared
   * Duplicated records: Record 123 appears twice (both have valid CRCs)
   * Reordered records: All CRCs valid but revision sequence scrambled
   * Truncated files: Partial S3 download, missing last 1000 records
   * Header/footer corruption: Per-record CRCs don't protect metadata
