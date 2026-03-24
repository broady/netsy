---
title: "Overview"
weight: 10
description: "Overview of Netsy terminology, requirements, leader election, and startup process"
---

# Netsy System Design Overview

## Terminology

- __KV Data__: key-value data, also known as "etcd records", where each record has a revision integer, key string, value blob, and other metadata.
- __Node__: a single Netsy process.
- __Cluster__: a collection of Nodes for a given KV data store.
- __Primary__: the Cluster Node which handles all etcd transaction requests (write operations).
- __Replica__: all Cluster Nodes except for the Primary. Must proxy any transaction request (write operation) to the Primary.
- __Client__: a consumer of the etcd API subset in the Netsy API, also known as an "etcd client" e.g. `kube-apiserver` or `etcdctl`.
- __Peer__: a consumer of the Netsy API which is a Node.
- __Elector__: the Cluster Node responsible for leader election of the Primary. Can be the same Node as the Primary or a Replica.
- __Watch__: a long-lived subscription to key/key-range changes that streams ordered updates (puts/deletes) in real time as they occur.
- __Bind Address__: a host+port string used for binding a gRPC server to a given IP or hostname and port e.g. `0.0.0.0:2378`
- __Advertise Address__: a host+port string used for a Client or Peer to connect to the Node or Cluster e.g. `172.16.0.1:2378` or `etcd.example.com:2378`
- __Service Discovery__: how each Node learns about all other Nodes in a Cluster, including their Advertise addresses for Peer connections.

## Requirements

- Every __Node__ stores a copy of all __KV Data__ in a local SQLite database.
- __Replicas__ can answer range (read) requests directly, but proxy any transaction (write) requests to the __Primary__.
- The __Primary__ writes data to its SQLite database, object storage, and all __Replicas__.
    - In a __Cluster__ with less than 3 active __Nodes__, writes are synchronous to the object storage bucket, and therefore it is the canonical system-of-record.
    - Where there are 3 or more active __Nodes__, a transaction can be committed when at least 2 __Replicas__ acknowledge recording the data, and writes to object storage are asynchronous/buffered.
    - Data is sent to __Replicas__ via gRPC streams, and is stored in object storage using a custom [Netsy Data File](./data-files.md) format.
- The __Elector__ is the only __Node__ which can perform leader election for the __Cluster__ to determine which __Node__ is the __Primary__.
    - Determining which __Node__ is the __Elector__ uses a separate leader election process to the one which determines which __Node__ is the __Primary__. This may be referred to as a two-tier/dual-layer leader election system: one for the etcd writer/Primary role, one for the Elector role.
    - This two-tier approach is used because an audit must be conducted during leader election of the __Primary__ to ensure whichever __Replica__ is elected has the current latest-known etcd revision number, to prevent data loss.
    - The __Elector__ leader election process uses [s3lect](https://s3lect.dev) and uses object storage for coordination, where as the __Primary__ leader election process is handled by the __Elector__ itself.
- Mutual TLS (mTLS) is used for authentication of any __Client__ or __Peer__ connecting to a __Node__ gRPC server.
- Each __Node__ has:
   - A server TLS certificate used on the __Client__ and __Peer__ gRPC servers.
   - A client TLS certificate used for connecting to other __Peer__ gRPC servers.
   - A single CA used for all TLS certificates in the __Cluster__.
   - A __Bind__ address and an __Advertise__ address for __Client__ Node/Cluster connections and __Peer__ Node connections.
- For __Service Discovery__, each __Node__ registers itself as a member of the cluster by writing to a file in object storage under a known prefix for registration. The __Elector__ can then list files under that prefix to identify all __Nodes__ in the cluster.

## Further Reading

- [Netsy Data Files](data-files.md) – Netsy (.netsy) data file format/specification
- [Multi-Node & Replication](multi-node.md) – Netsy multi-node support & data replication system design
- [Watches & Compaction](watches-compaction.md) – Watch support & Compaction system design
