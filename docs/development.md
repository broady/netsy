---
title: "Development"
weight: 40
description: "How to work on and develop Netsy locally"
---

# Developing Netsy

## Prerequisites

Install the following tools:

- [Go](https://go.dev/dl/) (see `go.mod` for required version)
- [Air](https://github.com/air-verse/air) — live reload (`go install github.com/air-verse/air@latest`)
- [Overmind](https://github.com/DarthSim/overmind) — process manager (`brew install overmind`)

Run `make check` to verify all required tools are installed.

## Quick Start

Start the full development environment (fake S3 server + Netsy with live reload):

```
make dev
```

This will:
1. Generate development TLS certificates in `temp/certs/` if they don't exist
2. Start the fake S3 server (`cmd/dev-s3`) on `:4566`
3. Start Netsy via Air for live reload

Cluster config is loaded from `examples/config.jsonc`.

## Viewing Logs

Logs are written to `temp/logs/`:
- `temp/logs/dev-s3.log` — fake S3 server
- `temp/logs/netsy.log` — Netsy server

To view logs from a specific process in real-time:

```
overmind echo netsy
overmind echo s3
```

## Resetting Dev Data

Reset everything (certs, database, S3 data, logs):

```
make clean-dev && make dev
```

Reset only the database:

```
rm -f temp/data/db.sqlite3*
```

Reset only S3 data:

```
rm -rf temp/dev-s3/
```

Regenerate only TLS certificates:

```
rm -rf temp/certs/ && ./scripts/certs.sh
```

## Testing with kube-apiserver

Run a kube-apiserver container configured to use Netsy as its etcd backend:

```
./scripts/kube-apiserver.sh
```

Press `Ctrl+C` to stop. Requires a running Netsy instance (`make dev`).

## etcdctl

Run `etcdctl` with the correct certs and endpoint:

```
./scripts/etcdctl.sh endpoint status
./scripts/etcdctl.sh get "" --prefix
```

## Git Hooks

Install the pre-commit hook:

```
make git-hooks
```
