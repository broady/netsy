---
title: "Overview"
weight: 10
description: "Overview of Netsy terminology, requirements, leader election, and startup process"
---

# Netsy System Design Overview

## Terminology

- __Record__: a single key-value data entry, also known as an "etcd record", which has a revision integer, key string, value blob, and other metadata. Unique on revision+key.
- __KV Data__: the collection of Records, also known as "etcd records".
- __Node__: a single Netsy process. Each node has an identifier or "Node ID".
- __Cluster__: a collection of Nodes for a given KV Data store.
- __Primary__: the Cluster Node which handles all etcd transaction requests (write operations).
- __Replica__: all Cluster Nodes except for the Primary. Must proxy any transaction request (write operation) to the Primary.
- __Client__: a consumer of the etcd API subset in the Netsy API, also known as an "etcd client" e.g. `kube-apiserver` or `etcdctl`.
- __Peer__: a consumer of the Netsy API which is a Node.
- __Elector__: the Cluster Node responsible for leader election of the Primary. Can be the same Node as the Primary or a Replica.
- __Heartbeat__: a message sent by a Node to the Elector and/or Primary containing its current Node State (Health State, Primary State, and latest Revision).
- __ACK__: an acknowledgement message sent by a Replica to the Primary confirming that a Record has been durably committed to the Replica's local SQLite database. Every ACK embeds a Heartbeat.
- __Watch__: a long-lived subscription to key/key-range changes that streams ordered updates (puts/deletes) in real time as they occur.
- __Bind Address__: a host+port string used for binding a gRPC server to a given IP or hostname and port e.g. `0.0.0.0:2378`
- __Advertise Address__: a host+port string used for a Client or Peer to connect to the Node or Cluster e.g. `172.16.0.1:2378` or `etcd.example.com:2378`
- __Service Discovery__: how each Node learns about all other Nodes in a Cluster, including their Advertise addresses for Peer connections.

## Cluster State

There are two fields referred to as the "Cluster State" communicated to all Nodes:

- `Elector` Node ID

- `Primary` Node ID

## Node States

Each __Node__ has three state fields which can be read via the __Peer__ API:

1. __Health State__:

    - `Loading` during its initial startup and database backfill process.

    - `Healthy` when it has the latest revision within a threshold.

    - `Degraded` when it has failed to send any ACK or Heartbeat.

    A Node should be considered "unhealthy" if it has been in the `Loading` or `Degraded` state after a timeout.

2. __Elector State__:

    - `Leader`: the Node has been elected and is currently the Elector.

    - `Follower`: the Node is not the Elector.

3. __Primary State__:

    - `Replica`: the Node is a Replica and has not been elected Primary by the Elector since it started.

    - `Starting`: immediately after being elected Primary, while performing "preflight checks" before becoming Active.

    - `Active`: able to accept writes (provided its Chunk Buffer is not full).

    - `Draining`: needing to shutdown or consistently failing to write data (or Chunk Buffer is full).

    After a Primary finishes Draining, the Node process gives up its Primary leadership with the Elector, and restarts into a `Replica` Primary State (with a Loading Health State).

## Requirements

- Every __Node__ stores a copy of all __KV Data__ in a local SQLite database. SQLite must be configured for durability to ensure that a committed transaction is actually persisted to disk before an ACK is sent (to guarantee quorum transactions). The required configuration is:
    - `PRAGMA journal_mode=WAL` — WAL (Write-Ahead Logging) for concurrent read/write performance. Must be set once when the database is opened.
    - `PRAGMA synchronous=FULL` — ensures WAL writes are fsynced to disk before reporting commit success. This is critical: without it, a crash after commit but before fsync could lose data that was already ACK'd.
- __Replicas__ can answer range (read) requests directly, but proxy any transaction (write) requests to the __Primary__.
    - If a __Replica__ Health State is `Degraded`, it must continue to serve range (read) requests, which may return stale data. If stricter read consistency guarantees are required in the future, a Replica in `Degraded` state may instead reject or proxy read requests.
    - By its nature the __Primary__ cannot be in a `Degraded` state.
- Each __Node__ has an unencrypted HTTP endpoint `/health` for health-checking the Netsy process, with health determined by the Health State, which can be used by systems like Kubernetes or ASGs for health-checking the process.
- The __Primary__ writes data to its SQLite database, object storage, and all __Replicas__.
    - In a __Cluster__ without enough `Healthy` __Replicas__ to meet the configured quorum threshold, writes are synchronous to the object storage bucket, and therefore it is the canonical system-of-record.
    - Where there are enough `Healthy` __Replicas__ to meet the quorum threshold, a transaction can be committed when those __Replicas__ acknowledge recording the data, and writes to object storage are asynchronous/buffered. See [Quorum Configuration](./storage-replication.md#quorum-configuration) for details.
    - Data is sent to __Replicas__ via gRPC streams, and is stored in object storage using a custom [Netsy Data File](./data-files.md) format.
    - __Replicas__ must not accept data from the __Primary__ unless its Primary State is not `Replica` (must be `Starting`, `Active`, or `Draining`).
    - The __Primary__ must accept connections from __Replicas__ when its Primary State is `Starting`, `Active`, or `Draining`.
- The __Elector__ is the only __Node__ which can perform leader election for the __Cluster__ to determine which __Node__ is the __Primary__.
    - Determining which __Node__ is the __Elector__ uses a separate leader election process to the one which determines which __Node__ is the __Primary__. This may be referred to as a two-tier/dual-layer leader election system: one for the etcd writer/Primary role, one for the Elector role.
    - This two-tier approach is used because an audit must be conducted during leader election of the __Primary__ to ensure whichever __Replica__ is elected has the current latest-known etcd revision number, to prevent data loss.
    - The __Elector__ leader election process uses [s3lect](https://s3lect.dev) and uses object storage for coordination, whereas the __Primary__ leader election process is handled by the __Elector__ itself.
- Mutual TLS (mTLS) is used for authentication of any __Client__ or __Peer__ connecting to a __Node__ gRPC server.
- Each __Node__ has:
   - A server TLS certificate used on the __Client__ and __Peer__ gRPC servers.
   - A client TLS certificate used for connecting to other __Peer__ gRPC servers.
   - Its "Node ID" embedded in its server and client TLS certificate to prevent impersonation.
   - A single CA used for all TLS certificates in the __Cluster__.
   - A __Bind__ address and an __Advertise__ address for __Client__ Node/Cluster connections and __Peer__ Node connections.
- For __Service Discovery__, each __Node__ registers itself as a member of the cluster by writing to a file in object storage under a known prefix for registration. The __Elector__ can then list files under that prefix to identify all __Nodes__ in the cluster.

## Further Reading

- [Leader Election](leader-election.md) - Netsy two-tier leader election system design.
- [Netsy Data Files](data-files.md) – Netsy (.netsy) data file format/specification.
- [Storage & Replication](storage-replication.md) – Netsy data storage and replication system design.
- [Loading & Startup](loading-startup.md) - Outline of how Node Loading and Primary Startup states work.
- [Watches & Compaction](watches-compaction.md) – Watch support & Compaction system design.
