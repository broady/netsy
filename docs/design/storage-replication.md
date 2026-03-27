---
title: "Storage & Replication"
weight: 40
description: "Netsy data storage and replication system design"
---

# Object Storage & Multi-Node Replication

Netsy is designed to be straightforward to operate. A good litmus test of this is: if any or all Netsy servers in a Cluster are accidentially deleted or restarted, the goal of Netsy is that it won't lose data, auto-scaling can quickly recover the Cluster by starting fresh VMs which auto-recover, and downtime is minimised throughout this process.

To achieve this, Netsy aims to use object storage as the system-of-record. However, doing this comes with a trade-off: latency vs data loss. The goal is to not have data loss, so what can you do to minimise latency? Run multiple Nodes (ideally 3+).

If Netsy has enough Healthy Replicas to meet the configured quorum threshold, the Primary will commit a transaction once those Replicas have acknowledged receipt of the data, otherwise it will fallback to synchronous writes to object storage - and Netsy automatically tracks and adjusts the approach based on the Health State of all Nodes in a Cluster to make it straightforward for the operator and to minimise the chance of data loss.

## The Lifecycle of Transactions

To keep the implementation logic simple, during a transaction, there are two code paths which can be followed.

1. Object Storage Transactions - where the Primary commits a transaction once the Record has been flushed/written to object storage. Also known as __synchronous__ object storage writes.

2. Quorum Transactions - where the Primary commits a transaction once the configured quorum threshold of Replicas have ACK'd committing the transaction themselves. Also known as __asynchronous__ object storage writes.

To determine which to follow for any given transaction, the Netsy Primary keeps track of all Nodes, and only follows path 2 for Quorum Transactions when there are enough healthy Replicas to meet the configured quorum threshold (see [Quorum Configuration](#quorum-configuration)).

### 1. Object Storage Transaction Logic

1. Lock leaderTxnMutex
2. Parse request, assign revision
3. Begin SQLite transaction
4. Insert record into SQLite (not committed)
5. Write chunk and flush to S3 (synchronous)
6. S3 fails -> rollback, return error to client
7. S3 succeeds -> commit SQLite transaction
8. Increment revision counter
9. Send record to any connected Replicas asynchronously
    - note: asynchronously means do not wait for ACK, though it is still tracked for health
10. Respond to client

### 2. Quorum Transaction Logic

1. Lock leaderTxnMutex
2. Parse request, assign revision
3. Begin SQLite transaction
4. Insert record into SQLite (not committed)
5. Send record to all connected healthy Replicas
6. Wait for >= quorum threshold durable ACKs (with timeout, e.g. 1s).
    - note: "durable ACK" means the Replica has committed the record to its own SQLite database (with `synchronous=FULL`, ensuring fsync to disk) before sending the ACK. See [Requirements](overview.md#requirements) for SQLite durability configuration.
7. Quorum threshold acks received:
    - Commit SQLite transaction
    - Increment revision counter
    - Buffer record for async S3 write
    - Respond to client
8. Timeout / insufficient acks:
    - Mark timed-out Replicas as unhealthy
    - Rollback SQLite transaction (the failed record is discarded and will not be included in any subsequent S3 flush)
    - Immediately trigger buffer flush to S3 of any previously buffered records (separate goroutine)
    - Return error to client
    - Client retries -> Primary now sees insufficient healthy Replicas for quorum -> follows Path 1

### Switching Between Paths

Path 1 -> Path 2:

- when a Replica's ACK count reaches 1 (first successful ACK = healthy), and the total healthy Replicas meets or exceeds the configured quorum threshold

Path 2 -> Path 1:

- immediately on ACK timeout, stream drop, or heartbeat timeout — any event that drops healthy count below the quorum threshold

## Quorum Configuration

The quorum threshold is configurable and determines the number of Replica ACKs required before a transaction can be committed without a synchronous object storage write.

The configuration value represents the number of Replica ACKs needed (the Primary's own copy does not count towards satisfying this number):

- __`-1`__ (default): dynamically calculate the quorum as a raft-style majority based on the number of registered Nodes: `floor(N/2) + 1` where N is the total number of registered Nodes. For example:
  - 7 registered Nodes -> 4 Replica ACKs needed
  - 5 registered Nodes -> 3 Replica ACKs needed
  - 4 registered Nodes -> 3 Replica ACKs needed
  - 3 registered Nodes -> 2 Replica ACKs needed
  - If the total registered Nodes drops below 3, the calculated threshold equals or exceeds the number of available Replicas (e.g. 2 Nodes -> 2 ACKs needed but only 1 Replica exists), so the Primary will always fall back to synchronous object storage writes. This effectively behaves the same as disabled quorum (`0`).
- __Positive integer__ (e.g. `2`): a static number of Replica ACKs required. This is similar to PostgreSQL's synchronous replication, optimised for performance in larger clusters where a full majority is not required. When using a static value less than a majority, the Elector must contact all registered Nodes during leader election (not just a majority) to ensure the Node with the highest revision can be found and elected. Leader election will block until all Nodes are contactable, or until unavailable Nodes are deregistered.
- __`0`__: disable quorum transactions entirely. All writes use synchronous Object Storage Transactions (Path 1). Useful for single-node deployments or when latency is not a concern.

If the number of healthy Replicas is below the value (static or dynamically calculated threshold), the Primary automatically falls back to synchronous Object Storage Transactions (Path 1). This means writes continue to succeed with higher latency rather than failing, and the system self-heals once enough Replicas become healthy again.

### Tracking Replicas for Quorum

The Primary holds a map of the Health State of each Replica and when it last successfully ACK'd a transaction, and when it last received a Heartbeat (either standalone or embedded in an ACK). Since every ACK embeds a Heartbeat, the Primary processes both using the same code path to update this map.

Each entry for a Replica in this map uses atomic fields, so their data can be updated independently without locks. And when a new transaction needs to be written, the Primary can iterate through the list to determine whether or not there are enough healthy replicas.

A Replica that reports a `Degraded` Health State (e.g. because it can no longer reach the Elector) will not be counted as healthy for quorum. This is critical for partition safety: a Replica that self-degrades due to Elector connectivity loss causes the Primary to fall back to synchronous S3 writes, ensuring all committed data is durably in object storage before a response is sent to the client.

For additional safety, a Primary will not count a Replica as healthy for quorum transactions until it has successfully ACK'd at least once. This protects against cases where a new Replica could have a bug or issue in its code preventing successful transactions from being committed to disk.
