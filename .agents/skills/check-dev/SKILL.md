---
name: check-dev
description: "Checks the status and diagnoses the health of the Netsy dev environment (make dev). Checks overmind processes, dev-s3, health endpoints, log files for errors, heartbeat and election state, and primary election status. Use when asked to check, verify, or debug the dev environment."
---

# Checking Dev Health

Runs diagnostics on a running Netsy dev environment (`make dev`).

## What It Checks

1. **Processes** — overmind socket, dev-s3, and netsy instance(s) must be running
2. **Health endpoints** — HTTP health check on `:8080/health`, TCP checks on client/peer gRPC ports, dev-s3 port
3. **Log files** — scans `temp/logs/` for panics, fatals, and error-level logs
4. **Election status** — confirms elector leadership was acquired and a primary was elected
5. **Heartbeat health** — checks for degradation, self-degradation, and heartbeat failures

## Usage

Run the diagnostic script:

```bash
./scripts/check-dev.sh
```

The script exits 0 if healthy (including warnings only), exits 1 if any failures are found.

### Investigating Failures

If the script reports failures or warnings, follow up:

1. **Check dev logs** in `temp/logs/` for context:
   - `temp/logs/dev-s3.log` — fake S3 storage issues
   - `temp/logs/netsy-1.log` (and `netsy-2.log`, etc.) — node startup, election, heartbeat, replication
   - Filter with `grep -i -E "error|warn|panic" temp/logs/netsy-1.log | tail -20`

2. **Correlate with design docs** under `docs/design/` for expected behavior:
   - `docs/design/leader-election.md` — two-tier election, heartbeat mechanism, degradation rules
   - `docs/design/loading-startup.md` — bootstrap sequence, registration, backfill
   - `docs/design/storage-replication.md` — quorum config, primary/replica roles
   - `docs/design/failure-scenarios.md` — expected failure modes and recovery

3. **Common failure patterns**:
   - `"no registered nodes"` — election ran before node registration completed (normal on first attempt)
   - `"no healthy candidates"` — node marked degraded due to missed heartbeats; check heartbeat sender logs
   - `"non-degraded node X has primary state "` — node map entry missing `PrimaryState` initialization
   - `"election_failed"` looping indefinitely — check that heartbeats are reaching the elector

## Prerequisites

The user should already have `make dev` running. Use `make clean-dev` then `make dev` to restart from a clean state.
