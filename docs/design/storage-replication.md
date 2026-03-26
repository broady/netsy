---
title: "Storage & Replication"
weight: 40
description: "Netsy data storage and replication system design"
---

# Object Storage & Multi-Node Replication

Netsy is designed to be straightforward to operate. A good litmus test of this is: if any or all Netsy servers in a Cluster are accidentially deleted or restarted, the goal of Netsy is that it won't lose data, auto-scaling can quickly recover the Cluster by starting fresh VMs which auto-recover, and downtime is minimised throughout this process.

To achieve this, Netsy aims to use object storage as the system-of-record. However, doing this comes with a trade-off: latency vs data loss. The goal is to not have data loss, so what can you do to minimise latency? Run at least 3 Nodes. If Netsy has at least 2 Healthy Replicas, the Primary will commit a transaction once at least 2 Replicas have acknlowedged receipt of the data, otherwise it will fallback to synchronous writes to object storage.

To make this straightforward for the operator and to minimise the chance of data loss in a scenario where you are running exactly 3 Nodes (and some may become unhealthy temporarily), there is zero configuration for this: Netsy automatically tracks and adjusts the approach based on the Health State of all Nodes in a Cluster.

## The Lifecycle of Transactions

To keep the implementation logic simple, during a transaction, there are two code paths which can be followed.

1. Object Storage Transactions - where the Primary commits a transaction once the Record has been flushed/written to object storage. Also known as __synchronous__ object storage writes.

2. Quorum Transactions - where the Primary commits a transaction once at least 2 Replicas have ACK'd committing the transaction themselves. Also known as __asynchronous__ object storage writes.

To determine which to follow for any given transaction, the Netsy Primary keeps track of all Nodes, and only follows path 2 for Quorum Transactions when there are at least 2 healthy Replicas.

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
6. Wait for >=2 durable ACKs (with timeout, e.g. 1s).
    - note: "durable ACK" means the Replica has committed the record to its own SQLite database before sending the ACK
7. At least 2 acks received:
    - Commit SQLite transaction
    - Increment revision counter
    - Buffer record for async S3 write
    - Respond to client
8. Timeout / insufficient acks:
    - Mark timed-out Replicas as unhealthy
    - Rollback SQLite transaction
    - Immediately trigger buffer flush to S3 (separate goroutine)
    - Return error to client
    - Client retries -> Primary now sees < 2 healthy -> follows Path 1

### Switching Between Paths

Path 1 -> Path 2:

- when a Replica's ACK count reaches 1 (first successful ACK = healthy), and total healthy Replicas reaches >=2

Path 2 -> Path 1:

- immediately on ACK timeout, stream drop, or heartbeat timeout — any event that drops healthy count below 2

### Tracking Replicas for Quorum

The Primary holds a map of the Health State of each Replica and when it last successfully ACK'd a transaction, and when it last successfully sent a Heartbeat.

Each entry for a Replica in this map uses atomic fields, so their data can be updated independently without locks. And when a new transaction needs to be written, the Primary can iterate through the list to determine whether or not there are enough healthy replicas.

For additional safety, a Primary will not count a Replica as healthy for quorum transactions until it has successfully ACK'd at least once. This protects against cases where a new Replica could have a bug or issue in its code preventing successful transactions from being committed to disk.
