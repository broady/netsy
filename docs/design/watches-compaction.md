---
title: "Watches & Compaction"
weight: 70
description: "Watch support & Compaction system design"
---

# Watches & Compaction

## Watch vs Watcher

Note the distinction between a Watch and a Watcher:

- __Watch__: a long-lived subscription to key/key-range changes that streams ordered updates (puts/deletes) in real time as they are committed.
    - Watch events are only delivered for Records at or below the `committed_revision` — tentative Records above the `committed_revision` are never visible to Watchers.
- __Watcher__: a process connected to the etcd API with one or more __Watch__.

All Netsy __Nodes__ can have a set of independent __Watchers__ with multiple __Watches__.

For example, each `kubectl` client can have an active __Watch__, and they would be connected to a `kube-apiserver`, which is a __Watcher__.

## Minimum Watch Revisions

Each __Watcher__ and __Watch__ is tracked in-memory on each __Node__. Critically, when a new __Watch__ is created, each __Node__ must calculate the min(imum) revision for that __Watch__.

The __Peer__ API of each __Node__ exposes an endpoint whereby the global min(inimum) version for all of its __Watches__ can be queried by the __Primary__, which is critical information for Compaction.

If a Node has no active Watches, it returns its current `committed_revision` as the minimum revision.

Each Node also persists the latest accepted Compaction Revision locally. Restarting Replicas can also restore watch-admission gating from the Primary's `Initial` message on the `Follow` stream, without waiting for the next compaction cycle.

## What is Compaction?

Compaction is the process of removing historical data from the __KV Data__ store.

Due to the nature of etcd's API design, every create/update/delete operation writes a new record:

- Create "example" key with value "example1"
    - KV Data will now have revision 1 `example=example1`.
- Update "example" key with value "example2"
    - KV Data will now have revision 1 `example=example1`, and revision 2 `example=example2`.
- Delete "example" key.
    - KV Data will now have revision 1 `example=example1`, and revision 2 `example=example2`, and revision 3 `example (record deleted)`.

If the first or second revision is no longer tracked by a Watch, they can be safely removed from the __KV Data__ store.

## How Compaction Works in Netsy

>  __STATUS__: Compaction is not currently implemented in Netsy.

The current __Primary__ can periodically schedule Compaction across all __Nodes__.

To do this, it retrieves the global min revision of all __Watches__ for each __Node__ via the __Peer__ API, and then finds the global minimum of that, which becomes the "Compaction Revision" where every revision prior to that is considered safe to "compact".

- If a __Node__ cannot be successfully queried for the min revision, the Compaction process ends early and awaits its next scheduled occurrence.

Once the Compaction Revision has been identified, if it is greater than the previous Compaction Revision:

1. The __Primary__ will send a notice to every __Node__ that the new minimum revision will be this "compaction revision". Each __Node__ must, under a single lock, atomically: first raise its watch-admission floor to the compaction revision (blocking any new __Watch__ requests for revisions below it), then validate that no existing active __Watch__ has a revision below the compaction revision. If validation fails, the __Node__ rolls back the floor and rejects the notice. Only if both steps succeed does the __Node__ confirm. If any of the __Nodes__ fail to confirm, they are retried once, or otherwise the Compaction process exits until the next interval.

2. Once the notice has been accepted cluster-wide, the __Primary__ sends a logical `Compact` message on `Follow` streams. On receiving that confirmation, each __Node__ persists the Compaction Revision into its local `compactions` table and keeps the watch-admission gate in place durably. If a restarting Node or a newly elected __Primary__ finds this table empty during startup or preflight, it must seed it from the Compaction Revision implied by contiguous existing `records` rows with `compacted_at` already set before accepting new Watches or writes.

__Nodes__ must then enqueue an async compaction task, where it simply sets the compacted_at timestamp and value to NULL for any record not already compacted with a revision number lower than the compacted_revision. Note that unlike etcd, Netsy does not remove the record entirely, only the value blob.

## Compaction & Snapshots

Because of the design of this compaction mechanism, all future snapshots created will be effectively compacted - retaining a full history of revisions, but without the overhead of (often large) values. No new records/chunks will be produced as a result of the process.

To avoid compaction impacting snapshots in-progress, we must ensure the snapshotting and compaction processes do not take place concurrently.  
