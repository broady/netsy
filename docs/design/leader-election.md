---
title: "Leader Election"
weight: 20
description: "Netsy two-tier leader election system design"
---

# Leader Election for Elector & Primary Roles

Unlike etcd, Netsy does not use Raft or consensus algorithms. Netsy is more like a traditional database such as PostgreSQL with its synchronous streaming replication.

Netsy only ever has a single Primary Node which is responsible for handling all etcd transactions (write operations) in a Cluster, and it serializes all writes.

The Primary Node is determined through a leader election process, to which its correctness is critical to data consistency and integrity.

## Prerequisites for Replication

Due to the replication model used by Netsy, there is a strict requirement when electing a new Primary: Netsy must audit all Nodes in the cluster, and ensure only Nodes with the latest known revision can become the new Primary, to prevent data loss.

While etcd handles this using raft, Netsy does not use consensus: instead, it uses a two-tier leader election system, allowing the audit process to more efficiently run on a single Node known as the Elector.

## Two-Tier Leader Election

Why are there two tiers? The “second tier” leader election logic for determining the Primary, including the latest-revision audit, is implemented in a Netsy process itself - but which Node is responsible for running this is determined through the “first tier” leader election logic, which uses s3lect to coordinate leader election using object storage, and gives the node responsible the role of the Elector.

Nodes can establish which Node is currently the Elector using s3lect, which does so by querying object storage, and also has an efficient HTTPS-based mechanism during a stable state.

Nodes can determine which Node is the Primary by querying the Elector.

## Service Discovery for Nodes

The Elector requires knowledge of all Nodes in its Cluster. The mechanism for registering and listing Nodes is referred to as Service Discovery.

During the Node `Loading` Health State, Netsy ensures a file is written to object storage for its Node ID, containing its Advertise address(es). The prefix used for this is `nodes/`.

When a Node becomes an Elector, it stores a map of Nodes and their addresses in-memory, populated immediately from object storage, and updated by each new Node registration directly once the Node becomes Elector (which can happen before the object storage backfill is completed). Until the initial load from object storage is complete, an Elector will not perform leader election for a new Primary.

## Elector Leader Election

If the Elector determines no Primary exists (either via a failed health check, or failed 'who is the Primary' request from Peers), the Elector begins the leader election process:

1. Retrieve the current Health State, current Primary State, Start Time, and latest Revision from each Node, including itself.

2. Exit leader election and save the Primary to local memory if a single Node has the `Active` Primary State.

3. Fail leader election if any of the Nodes Primary State is not `Replica`, or if a Node is uncontactable (after 2 retries, which happens once after a 10ms delay, and again after a 100ms delay).

   - This guard exists to prevent data loss in the event of network partitioning when asynchronous object storage writes have occurred.

   - This also prevents a new Primary being elected while the existing Primary is `Starting`, `Active`, or `Draining`.

   - Note that the correct process for Nodes to be removed is they should move to a Degraded Health State and should deregister themselves (first in object storage, and also with the current Elector) to prevent this scenario from happening during planned changes.

4. Filter any Nodes not in the `Healthy` Health State. Fail leader election if the Nodes list is empty.

5. Sort remaining Nodes by the latest Revision from highest to lowest, then by the Node Start Time, and filter any below the largest Revision number.

6. Elects the first of the remaining Nodes which will have the highest Revision number and latest Start Time. That Node may be itself if it has an equal or highest Revision number.

  - The latest Start Time is picked as Nodes may be periodically rotated, and selecting the newest Node will presumably result in the longest leadership lifetime.

Once a Node has been elected as the Primary, no action is taken until the Cluster State is pushed to each Node.

Leader election will continue retrying every 500 milliseconds until successful, and if the Elector crashes mid-leader election of a new Primary it can safely retry.

## Node State Checks & Cluster State Push

Once an Elector is newly elected and has loaded its Service Discovery map, it can begin to push Cluster State to each Node, and periodically polls each node for its Node State information.

- Cluster State includes the current Elector and current Primary.

- Node State periodically retrieved from each Node includes its current Health State, current Primary State, and latest Revision.

This approach is taken to propagate Cluster State as fast as possible.

- The current Elector is immediately pushed to each Node, rather than waiting on each Node to discover who the new Elector is using s3lect.

- Each Node can detect if there's a bug resulting in Elector split-brain because it will have two Electors attempting to connect concurrently and can guard against this.

Cluster State push is triggered immediately as part of Netsy updating its in-memory state for Elector and Primary. When iterating over each Node to push this Cluster State, it will always push the latest Cluster State to the Primary first, followed by all other known Nodes.

- When a Node receives the updated Cluster State indicating it has become the Primary, it will move its Primary State from `Replica` to `Starting` and begin to perform pre-flight checks.
