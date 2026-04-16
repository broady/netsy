# Netsy

Netsy is an [etcd](https://etcd.io/) alternative for Kubernetes which stores data in S3 (or S3-compatible) object storage.

Unlike etcd which uses the Raft consensus algorithm, the design of Netsy multi-node replication is inspired by PostgreSQL synchronous streaming replication, and by modern architectures of systems like Loki/Mimir and OpenObserve which use S3 (or S3-compatible) object storage for data persistence.

__Read the [announcement blog post](https://nadrama.com/blog/introducing-netsy) to learn more about the history and evolution of Netsy.__

Netsy was created by [Nadrama](https://nadrama.com). Nadrama helps you deploy containers, in your cloud account, in minutes. Nadrama uses Netsy in production for its Kubernetes clusters!

## Goals

Netsy was created to reduce the operational complexity and compute requirements traditionally associated with running etcd for Kubernetes clusters.

* S3 MUST be the permanent data store. 

* If S3 writes are async, data MUST be able to be replicated to at least one node.

* Netsy MUST maintain compatibility with the subset of the etcd API used by Kubernetes.

* It is a non-goal to fully support the entire etcd API.

## Project Status

The aim of the current branch is to add multi-node support to Netsy. The first version of Netsy was a single-node developer preview release.

Nadrama is committed to Open Source - read more [here](https://nadrama.com/opensource).

Current Features:

* KV ranges, KV transactions, Watches - supporting all options used by Kubernetes  
* S3 snapshots - full copies of records from leader SQLite database stored in S3  
* S3 chunking - delta of records from last snapshot stored in S3 chunk files  
* Backfill from S3 - starting a a fresh server restores from snapshots+chunks in S3  
* Compaction - cluster-wide protocol to remove values from historical revisions

Roadmap:

* Conformance Tests - VCR-style acceptance test suite (in development)  
* Encryption - supporting value field encryption, with rolling key rotation  
* Leases - used by Kubernetes.

## Usage

Build:

```
make build
```

Run:

```
./bin/netsy
```

You can look at the [.env](./.env) file for configuration examples.

## Documentation

Check out the comprehensive documentation in [./docs](./docs) or read it rendered on the official Netsy website at <https://netsy.dev>

## Development

See [docs/development.md](docs/development.md) for development environment setup and usage.

## License

Netsy is licensed under the Apache License, Version 2.0.
Copyright 2026 Nadrama Pty Ltd.

See the [LICENSE](./LICENSE) file for details.
