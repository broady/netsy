# HTTP Servers Missing All Timeouts

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 6 (bound every resource -- HTTP servers: ReadHeaderTimeout, ReadTimeout, WriteTimeout, IdleTimeout) |

## Location

- `internal/healthserver/server.go:40`
- `cmd/dev-s3/main.go:68-71`

## Description

Both HTTP servers create `http.Server{}` with zero-value timeouts. A slow or
malicious client can hold a connection open indefinitely, tying up a goroutine.

## Practical risk

Low. The health server is cluster-internal (headless Kubernetes Service,
kubelet probes only). The dev-s3 server is a local development tool. Neither
is exposed to untrusted networks.

## Impact

Theoretical slowloris-style resource exhaustion if an attacker has network
access. In practice, Kubernetes network policies limit exposure.

## Suggested fix

Add `ReadHeaderTimeout: 10 * time.Second` to the health server as defensive
hardening. The dev-s3 server is low priority.
