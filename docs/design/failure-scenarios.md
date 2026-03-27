---
title: "Failure Scenarios"
weight: 60
description: "Data integrity and safety analysis across quorum configurations and cluster sizes"
---

# Failure Scenarios & Data Integrity

Below you will find a list of failure scenarios at various cluster sizes with an explaination how each quorum configuration handles them.

It serves as a reference for operators choosing a quorum setting, and as a validation of the system design.

## Quorum Configurations

- **`0` (Disabled)**: all writes synchronous to S3. No quorum transactions.
- **`-1` (Majority - Default)**: replica ACKs required = `floor(N/2) + 1` where N = registered Nodes.
- **`2` (Static)**: exactly 2 replica ACKs required regardless of cluster size.

See [Quorum Configuration](storage-replication.md#quorum-configuration) for full details.

---

## Scenario 1: Primary Crashes, All Replicas Healthy

The Primary process crashes unexpectedly. All Replicas are healthy and reachable.

| | `0` (Disabled) | `-1` (Majority) | `2` (Static) |
|---|---|---|---|
| **3-node cluster** | All data in S3. Elector elects Replica with highest revision. No data loss. | 2 Replicas have all ACK'd data. Elector contacts majority (2), elects highest revision. No data loss. | Same as majority for 3 nodes (both require 2 ACKs). No data loss. |
| **5-node cluster** | Same as above. No data loss. | 3 Replicas have all ACK'd data. Elector contacts majority (3), elects highest revision. No data loss. | 2 Replicas have ACK'd data, others may not. Elector must contact all 4 Replicas. No data loss. |
| **7-node cluster** | Same as above. No data loss. | 4 Replicas have all ACK'd data (majority = 4). Elector contacts majority (4), elects highest revision. No data loss. | Only 2 Replicas needed to ACK — lower write latency than majority (2 vs 4 ACKs). Elector must contact all 6 Replicas. No data loss. |
| **Operator action** | None | None | None |

---

## Scenario 2: Primary Crashes, One Replica Partitioned

The Primary crashes. One Replica is network-partitioned from the Elector (self-degraded).

| | `0` (Disabled) | `-1` (Majority) | `2` (Static) |
|---|---|---|---|
| **3-node cluster** | S3 has all data. Elector elects from known Nodes. No data loss. | Partitioned Replica self-degraded -> Primary fell back to sync S3 before crashing (only 1 healthy Replica < majority of 2). Elector contacts majority (2): if partitioned node is unreachable, election blocks until it recovers or a majority responds (could mean old Primary is back online). No data loss. | Partitioned Replica self-degraded -> Primary fell back to sync S3. Elector must contact all Nodes. Election blocks until partitioned Replica recovers or is manually deregistered. No data loss. |
| **5-node cluster** | Same as above. No data loss. | 3 healthy Replicas still >= majority (3). Quorum writes continued. Elector contacts majority (3) from 4 remaining reachable Nodes. No data loss. | 2 healthy Replicas still >= 2. Quorum writes continued. Elector must contact all Nodes — blocks until partitioned Replica recovers or is deregistered. No data loss. |
| **7-node cluster** | Same as above. No data loss. | 5 healthy Replicas still >= majority (4). Quorum writes continued. Elector contacts majority (4) from 6 remaining reachable Nodes. No data loss. | 2 healthy Replicas still >= 2. Quorum writes continued without interruption — the partition has no impact on write performance. Elector must contact all Nodes — blocks until partitioned Replica recovers or is deregistered. No data loss. |
| **Operator action** | None | None (3-node: may need to wait for partition to heal or majority to respond) | None if `node_deregistration_timeout` > 0 (auto-deregistered after timeout); otherwise manually deregister partitioned Node to unblock election |

---

## Scenario 3: Primary + One Replica Crash Simultaneously

The Primary and one Replica both crash at the same time (e.g. shared rack failure).

| | `0` (Disabled) | `-1` (Majority) | `2` (Static) |
|---|---|---|---|
| **3-node cluster** | S3 has all data. Sole surviving Node elected. No data loss. | Majority = 2 ACKs. The crashed Replica ACK'd all committed records (durable in SQLite). Only 1 Node reachable - cannot reach majority (2). Election blocks until crashed Replica restarts (SQLite has the data) or is deregistered. No data loss if Replica recovers. | Must contact all Nodes. Election blocks until both crashed Nodes restart or are deregistered. No data loss if they recover (durable SQLite). |
| **5-node cluster** | Same as above. No data loss. | 3 healthy Replicas remain. Elector contacts majority (3) - possible from remaining 3 Nodes. Crashed Replica had ACK'd data, but majority overlap guarantees a reachable Node also has it. No data loss. | Must contact all Nodes. Election blocks until crashed Nodes restart or are deregistered. No data loss if they recover. |
| **Operator action** | None | 3-node: wait for recovery or deregister. 5-node: none. | Wait for recovery, or deregister crashed Nodes (auto if `node_deregistration_timeout` > 0, otherwise manual) |

---

## Scenario 4: Node's Disk Physically Destroyed

A Node's VM is terminated with ephemeral storage - all local data (SQLite) is permanently lost.

| | `0` (Disabled) | `-1` (Majority) | `2` (Static) |
|---|---|---|---|
| **3-node cluster** | S3 has all data. Node deregistered (auto or manual). No data loss. | If destroyed Node had ACK'd records not yet in S3: another Replica in the majority also ACK'd them (majority = 2), so data exists on a reachable Node. No data loss. | If destroyed Node was the only Replica that ACK'd a record (quorum = 2, but other Replica hadn't ACK'd yet - not possible, both must ACK). No data loss. |
| **5-node cluster** | Same as above. No data loss. | Majority overlap guarantees another Node has the data. No data loss. | If destroyed Node was one of only 2 that ACK'd: the other ACK'd Replica still has it. Election blocks until all Nodes contactable - operator deregisters destroyed Node. No data loss. |
| **Operator action** | Deregister (auto or manual) | Deregister (auto or manual) | Auto-deregistered if `node_deregistration_timeout` > 0; otherwise must manually deregister destroyed Node to unblock election |

---

## Scenario 5: Primary Partitioned from Elector (But Has S3 Access)

The Primary loses connectivity to the Elector and all Replicas, but can still reach S3.

| | `0` (Disabled) | `-1` (Majority) | `2` (Static) |
|---|---|---|---|
| **All cluster sizes** | Primary self-degrades (can't reach Elector) -> transitions to Draining -> flushes buffered records to S3 -> stops accepting writes. Elector marks Primary as Degraded, attempts to contact previous Primary (step 1 of election). After timeout, elects new Primary. No data loss - all data flushed to S3 during Draining. | Same Draining behavior. Primary falls back to sync S3 when Replicas are unreachable. All data in S3 before Primary stops. Elector contacts majority to elect new Primary. No data loss. | Same Draining behavior. All data in S3. Elector must contact all Nodes - blocks until old Primary recovers (now as Replica) or is deregistered. No data loss. |
| **Operator action** | None | None | Auto-deregistered if `node_deregistration_timeout` > 0; otherwise may need to manually deregister old Primary if it doesn't recover |

---

## Scenario 6: Scaling Down Without Graceful Shutdown

An operator or auto-scaler terminates a Node's VM without allowing graceful deregistration.

| | `0` (Disabled) | `-1` (Majority) | `2` (Static) |
|---|---|---|---|
| **3-node cluster** | S3 has all data. Node auto-deregistered after `node_deregistration_timeout`. No data loss. | If terminated Node was a Replica: auto-deregistered, N drops from 3->2, majority drops from 2->2 (still need 2 ACKs, but only 1 Replica - falls back to sync S3). No data loss. If terminated Node was Primary: same as Scenario 1. | Must contact all Nodes. Leader election of new Primary blocks until terminated Node is deregistered (auto if `node_deregistration_timeout` > 0, otherwise manual). No data loss. |
| **5-node cluster** | Same as above. No data loss. | Auto-deregistered, N drops from 5->4, majority = 3 ACKs. If only 3 Replicas remain, quorum still achievable. No data loss. | Must contact all Nodes. Election blocks until deregistered. No data loss. |
| **Operator action** | None (auto-deregistration) | None (auto-deregistration) | None if `node_deregistration_timeout` > 0; otherwise manual deregistration |

---

## Scenario 7: Primary Crashes + Two Replicas' Disks Destroyed

The Primary crashes and two Replicas have their VMs terminated with ephemeral storage (disks lost). If the Primary were still alive, it would hold all data in its own SQLite and no election would be needed — this scenario is only relevant when the Primary also fails (3 Nodes failing at once, including the Primary, is not likely).

| | `0` (Disabled) | `-1` (Majority) | `2` (Static) |
|---|---|---|---|
| **5-node cluster** | S3 has all data (all writes were synchronous). Deregister destroyed Nodes. Elector elects from remaining Nodes. No data loss. | Majority = 3 ACKs. Two destroyed Replicas may have been 2 of the 3 that ACK'd a record — but the 3rd ACK'd Replica is still alive and has the data (majority overlap with remaining 2 healthy Replicas + recovered Primary = guaranteed). After deregistration, N=3, majority=2. No data loss. | 2 ACKs required. Both destroyed Replicas could be the only 2 that ACK'd a record. The remaining 2 Replicas don't have it, and the Primary crashed before flushing to S3. Data loss possible. |
| **7-node cluster** | Same as above. No data loss. | Majority = 4 ACKs. Two destroyed Replicas may have been 2 of the 4 that ACK'd — but 2 other ACK'd Replicas are still alive with the data. After deregistration, N=5, majority=3. No data loss. | Same as 5-node: 2 ACKs required, both destroyed Replicas could be the only 2 that ACK'd. However, with a 7-node cluster using static quorum of 2, this is more likely since the 2 ACK'd Replicas are a smaller fraction of total Nodes (2 of 6 Replicas vs 2 of 4). Data loss possible. |
| **Operator action** | Deregister destroyed Nodes | Deregister destroyed Nodes. No data loss. | Auto-deregistered if `node_deregistration_timeout` > 0; otherwise manually deregister destroyed Nodes — potential (though unlikely) data loss if record only existed on destroyed Nodes' disks and Primary hadn't flushed to S3 |

---

## Scenario 8: Primary Crashes After Quorum Rollback (Tentative Record)

The Primary sends a record to Replicas, but quorum is not met. The Primary rolls back the transaction, then crashes before retrying. One Replica has stored the tentative record (above `committed_revision`), the others have not.

Example: 3-node cluster, Primary A sends revision 57 to B and C. Only B ACKs (commits to SQLite). Quorum not met. Primary rolls back, then crashes. B has revision 57 (tentative), C has revision 56. `committed_revision` remains at 56.

| | `0` (Disabled) | `-1` (Majority) | `2` (Static) |
|---|---|---|---|
| **3-node cluster** | N/A — quorum is disabled, all writes are synchronous to S3. The Primary would not have sent a quorum transaction. | B has tentative revision 57, C has revision 56. Elector contacts majority (2). B reports latest revision 57, C reports 56. B is elected. During Starting preflight, B reconciles with S3 (revision 57 is not in S3), adopts it as committed, uploads to S3, sets `committed_revision` = 57. The client received an error for the original write, but the data is preserved — no inconsistency since the client can retry and discover the record exists. No data loss. | Same behavior as majority for 3 nodes (both require 2 ACKs). B elected with highest revision, adopts tentative record. No data loss. |
| **5-node cluster** | N/A | B has tentative revision 57. Elector contacts majority (3). If B is among them, B has the highest revision and is elected. If B is not among the majority, all contacted Nodes have revision 56 — one is elected, and B's tentative revision 57 is overwritten when B connects to the new Primary (since 57 > `committed_revision` of 56, it is tentative and can be overwritten). Either way, no inconsistency — the client received an error. | Elector contacts all Nodes. B has the highest revision and is elected. Same as majority outcome. No data loss. |
| **7-node cluster** | N/A | Same as 5-node. B elected if in the majority, otherwise tentative record overwritten. No inconsistency. | Same as 5-node. B elected (highest revision). No data loss. |
| **Operator action** | None | None | None |

---

## Summary

| Quorum | Safety | Availability During Failure | Operator Burden |
|---|---|---|---|
| `0` (Disabled) | Strongest - S3 has every committed write | Highest - Elector can elect without contacting Nodes | Lowest |
| `-1` (Majority) | Strong - majority overlap prevents data loss unless majority of disks destroyed simultaneously | High - only needs majority of Nodes for election | Low - auto-deregistration handles most cases |
| Static (e.g. `2`) | Depends on value vs cluster size - data loss possible if the only Nodes with a record have disks destroyed | Lowest - requires all Nodes contactable for election | Dependent on `node_deregistration_timeout` - auto-deregistration if > 0, otherwise manual deregistration required |
