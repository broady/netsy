---
title: "Overview"
weight: 10
description: "Overview of Netsy terminology, requirements, leader election, and startup process"
---

# Netsy System Design Overview

## Terminology

- __Record__: a single key-value data entry, also known as an "etcd record", which has a revision integer, key string, value blob, and other metadata. Unique on revision, as each revision produces exactly one Record.
- __KV Data__: the collection of Records, also known as "etcd records".
- __Node__: a single Netsy process. Each node has an identifier or "Node ID". The Node ID must be lowercase alphanumeric characters and hyphens only, with no leading, trailing, or consecutive hyphens, and a maximum of 32 characters.
- __Cluster__: a collection of Nodes for a given KV Data store. Each Cluster has a "Cluster ID" following the same naming/validation rules as Node ID.
- __Primary__: the Cluster Node which handles all etcd transaction requests (write operations).
- __Replica__: all Cluster Nodes except for the Primary. Must proxy any transaction request (write operation) to the Primary.
- __Client__: a consumer of the etcd API subset in the Netsy API, also known as an "etcd client" e.g. `kube-apiserver` or `etcdctl`.
- __Peer__: a consumer of the Netsy API which is a Node.
- __Elector__: the Cluster Node responsible for leader election of the Primary. Can be the same Node as the Primary or a Replica.
- __Heartbeat__: a message sent by a Node to the Elector and/or Primary containing its current Node State (Health State, Primary State, and latest Revision).
- __Receipt__: a message sent by a Replica to the Primary confirming that a Record has been durably committed to the Replica's local SQLite database. Every Receipt embeds a Heartbeat.
- __Revision__: a monotonically increasing integer assigned by the Primary to each Record. Every write operation (create, update, delete) produces a new Record at the next revision number.
- __Committed Revision__: the highest Revision that has been confirmed committed by the Primary (quorum met or written to object storage).
- __Initial__: a logical message sent by the Primary to a Replica when a `Follow` stream is first established, carrying the current Committed and Compaction Revision.
- __Commit__: a logical message sent by the Primary to Replicas on the replication stream, advancing the Committed Revision and allowing Replicas to serve Records up to that Revision to Clients and Watches.
- __Compact__: a logical message sent by the Primary to Replicas on the replication stream, confirming the current Compaction Revision for watch-admission gating and per-Node compaction execution.
- __Watch__: a long-lived subscription to key/key-range changes that streams ordered updates (puts/deletes) in real time as they are committed (at or below the `committed_revision`).
- __Bind Address__: a host+port string used for binding a gRPC server to a given IP or hostname and port e.g. `0.0.0.0:2378`
- __Advertise Address__: a host+port string used for a Client or Peer to connect to the Node or Cluster e.g. `172.16.0.1:2378` or `etcd.example.com:2378`
- __Service Discovery__: how each Node learns about all other Nodes in a Cluster, including their Advertise addresses for Peer connections.
- __Member ID__: a stable numeric etcd-compatible member identifier assigned by the Elector and stored in object storage separately from active Node registrations.

## Cluster State

There are two components referred to as the "Cluster State" communicated to all Nodes:

- Current `Elector` Node

- Current `Primary` Node

## Node States

Each __Node__ has three state fields which can be read via the __Peer__ API:

1. __Health State__:

    - `Loading` during its initial startup and database backfill process.

    - `Healthy` when it has completed `Loading` and is not `Degraded`.

    - `Degraded` when it has failed to send any Receipt or Heartbeat after 1 immediate retry (self-degraded), when the Elector or Primary has detected 2 consecutive missed Heartbeats from the Node (Elector-degraded), or when a Replica receives a `committed_revision` from the Primary that is higher than its own latest revision and has not caught up within 2 seconds.

    A Node should be considered "unhealthy" if it has been in the `Loading` or `Degraded` state after a timeout.

2. __Elector State__:

    - `Leader`: the Node has been elected and is currently the Elector.

    - `Follower`: the Node is not the Elector.

3. __Primary State__:

    - `Replica`: the Node is a Replica and has not been elected Primary by the Elector since it started.

    - `Starting`: immediately after being elected Primary, while performing "preflight checks" before becoming Active. Must not accept new writes while in this state.

    - `Active`: able to accept writes (provided its Chunk Buffer is not full).

    - `Draining`: needing to shutdown or consistently failing to write data (or Chunk Buffer is full). Must not accept new writes while in this state.

    After a Primary finishes Draining, the Node process gives up its Primary leadership with the Elector, and restarts into a `Replica` Primary State (with a Loading Health State).

## Requirements

- Every __Node__ stores a copy of all __KV Data__ in a local SQLite database. SQLite must be configured for durability to ensure that a committed transaction is actually persisted to disk before a Receipt is sent (to guarantee quorum transactions). The required configuration is:
    - `PRAGMA journal_mode=WAL` — WAL (Write-Ahead Logging) for concurrent read/write performance. Must be set once when the database is opened.
    - `PRAGMA synchronous=FULL` — ensures WAL writes are fsynced to disk before reporting commit success. This is critical: without it, a crash after commit but before fsync could lose data that was already receipted.
- __Replicas__ can answer range (read) requests directly, but proxy any transaction (write) requests to the __Primary__.
    - Replicas must only serve records up to the `committed_revision` received from the Primary. Records above this revision are tentative (from in-progress or rolled-back transactions) and must not be visible to clients.
    - If a __Replica__ Health State is `Degraded`, it must continue to serve range (read) requests, which may return stale data. If stricter read consistency guarantees are required in the future, a Replica in `Degraded` state may instead reject or proxy read requests.
    - An active __Primary__ cannot be in a `Degraded` state — if a Primary degrades, it transitions to `Draining`, gives up leadership, and restarts as a `Replica` (see [Graceful Shutdown](./loading-startup.md#graceful-shutdown-signal-handling)).
- Each __Node__ has an unencrypted HTTP endpoint `/health` for health-checking the Netsy process, with health determined by the Health State, which can be used by systems like Kubernetes or ASGs for health-checking the process.
- The __Primary__ writes data to its SQLite database, object storage, and all __Replicas__.
    - In a __Cluster__ without enough `Healthy` __Replicas__ to meet the configured quorum threshold, writes are synchronous to the object storage bucket, and therefore it is the canonical system-of-record.
    - Where there are enough `Healthy` __Replicas__ to meet the quorum threshold, a transaction can be committed when those __Replicas__ confirm Receipt of the Record, and writes to object storage are asynchronous/buffered. See [Quorum Configuration](./storage-replication.md#quorum-configuration) for details.
    - The __Primary__ sends `PrimaryMessage` values to __Replicas__ on the replication stream, where `initial` carries the current Committed Revision and current Compaction Revision for a newly established stream, `record` is treated as a logical Record message, `commit` advances the current __Committed Revision__, and `compact` advances the current Compaction Revision. Records above the __Committed Revision__ are tentative and, if a rollback occurs, will be overwritten by a new transaction from the same __Primary__.
    - Data is sent to __Replicas__ via gRPC streams, and is stored in object storage using a custom [Netsy Data File](./data-files.md) format.
    - __Replicas__ must not accept data from the __Primary__ unless its Primary State is not `Replica` (must be `Starting`, `Active`, or `Draining`).
    - The __Primary__ must accept connections from __Replicas__ when its Primary State is `Starting`, `Active`, or `Draining`.
- The __Elector__ is the only __Node__ which can perform leader election for the __Cluster__ to determine which __Node__ is the __Primary__.
    - Determining which __Node__ is the __Elector__ uses a separate leader election process to the one which determines which __Node__ is the __Primary__. This may be referred to as a two-tier/dual-layer leader election system: one for the etcd writer/Primary role, one for the Elector role.
    - This two-tier approach is used because an audit must be conducted during leader election of the __Primary__ to ensure whichever __Replica__ is elected has the current latest-known etcd revision number, to prevent data loss.
    - The __Elector__ leader election process uses [s3lect](https://s3lect.dev) and uses object storage for coordination, whereas the __Primary__ leader election process is handled by the __Elector__ itself.
- Mutual TLS (mTLS) is used for authenticating/authorising connections to __Node__ gRPC servers.
    - All TLS certificates in a __Cluster__ share a single CA.
    - __Peer__ certificates must contain `OU=peer`, `CN=node_id`, and `O=cluster_id`. Node ID's are embedded into client ceritifcates to prevent impersonation.
    - __Client__ certificates must contain `OU=client`, `CN=client_id`, and `O=cluster_id`.
    - During the authentication flow, the server verifies `O` matches its own Cluster ID, preventing cross-cluster connections if a CA is re-used across clusters.
- Each __Node__ has:
    - A server certificate (used on __Client__ and __Peer__ gRPC servers) and a client certificate (used for outbound __Peer__ connections).
    - The `CN` (Node ID) is validated during loading/startup to ensure it matches the Node configuration.
    - The server certificate SANs are validated during loading/startup to ensure they cover the configured Client, Peer, and election advertise addresses.
    - A __Bind__ address and an __Advertise__ address for __Client__ Node/Cluster connections and __Peer__ Node connections.
- For __Service Discovery__, each __Node__ registers itself by writing a file in object storage under the `nodes/` prefix containing its Advertise addresses.
    - The __Elector__ also maintains a durable `members.json` file containing the Cluster ID and stable etcd `member_id -> node_id` mappings used for cluster topology APIs such as `MemberList`.

## Further Reading

- [Leader Election](leader-election.md) - Netsy two-tier leader election system design.
- [Netsy Data Files](data-files.md) – Netsy (.netsy) data file format/specification.
- [Storage & Replication](storage-replication.md) – Netsy data storage and replication system design.
- [Loading, Startup, & Shutdown](loading-startup.md) - Outline of how Node Loading, Primary Startup, and graceful Node Shutdown works.
- [Failure Scenarios](failure-scenarios.md) – Data integrity and safety analysis across quorum configurations and cluster sizes.
- [Watches & Compaction](watches-compaction.md) – Watch support & Compaction system design.
- [Compatibility With etcd](etcd-compatibility.md) – Supported etcd RPCs, unsupported RPCs, and notes on compatibility differences.
- [Observability](observability.md) – Metrics, structured logging, and debugging for Netsy clusters.
