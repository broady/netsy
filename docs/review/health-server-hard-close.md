# Health Server Uses Hard Close Instead of Graceful Shutdown

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 5 (graceful shutdown is mandatory and phased) |

## Location

- `internal/healthserver/server.go:60`

## Description

`Close()` calls `s.server.Close()` which immediately closes the listener and
terminates in-flight connections. The standard pattern is
`s.server.Shutdown(ctx)` which drains in-flight requests before closing.

## Impact

During rolling deployments, in-flight health check probes from the load
balancer or Kubernetes kubelet receive connection-reset errors. The
orchestrator may interpret this as a crash rather than a clean shutdown.

Practical risk is low -- the health server is cluster-internal and the handler
completes nearly instantly -- but it violates the graceful shutdown contract.

## Suggested fix

Replace `s.server.Close()` with `s.server.Shutdown(ctx)` using a short
deadline (e.g., 5 seconds).
