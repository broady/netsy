# GCS/S3 Client Initialization Blocks Indefinitely

| Field       | Value |
|-------------|-------|
| Severity    | Medium |
| Type        | Correctness |
| Confidence  | Confirmed |
| Guide rule  | Rule 6 (bound every resource -- shutdown: deadline on drain) |

## Location

- `internal/storage/gcs.go:32` (`gcsstorage.NewClient(context.Background())`)
- `internal/storage/s3.go:39` (`awsconfig.LoadDefaultConfig(context.Background())`)

## Description

Both cloud storage clients are initialized with `context.Background()` and no
timeout. If the cloud metadata service hangs (IMDS on EC2/EKS, metadata server
on GCE/GKE), the process blocks indefinitely during startup.

The `New` function signature does not accept a `context.Context`, so callers
cannot impose a deadline.

## Practical risk

Low to moderate. Startup-only (runs once). In Kubernetes, the liveness probe
would eventually kill a stuck pod. However, until the probe fires, the pod
consumes resources while doing nothing, and the failure mode is opaque (no
error, no log, just silence).

## Impact

Silent startup hang. No error message, no timeout. Requires external
intervention (Kubernetes restart, manual kill).

## Suggested fix

Add `context.WithTimeout(context.Background(), 30*time.Second)` for client
initialization. Update the `New` function to accept a `context.Context`.
