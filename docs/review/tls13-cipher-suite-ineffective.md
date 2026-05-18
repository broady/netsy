# TLS 1.3 Cipher Suite Restriction Silently Ineffective

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 8 (system boundary contracts -- document invariants and validate) |

## Location

- `internal/mtls/auth.go:54` (NewServerTLSConfig)
- `internal/mtls/auth.go:112` (NewClientTLSConfig)

## Description

Both TLS configs set `MinVersion: tls.VersionTLS13`, `MaxVersion:
tls.VersionTLS13`, and `CipherSuites: []uint16{tls.TLS_AES_256_GCM_SHA384}`.

Go's `crypto/tls` documentation states that `CipherSuites` applies only to TLS
1.2 and below. For TLS 1.3, all three cipher suites (AES-128-GCM-SHA256,
AES-256-GCM-SHA384, CHACHA20-POLY1305-SHA256) are always available and cannot
be restricted.

The `CipherSuites` field is silently ignored.

## Impact

The code appears to enforce AES-256-GCM-only for compliance or security policy
reasons. In production, connections may negotiate AES-128-GCM or ChaCha20,
violating the intended policy without any warning.

## Suggested fix

Remove the `CipherSuites` field (it does nothing with TLS 1.3) and document
that TLS 1.3 does not allow cipher suite restriction. If AES-256 is a hard
compliance requirement, fall back to TLS 1.2 with the restricted suite (not
recommended) or accept that TLS 1.3's three suites are all considered secure.
