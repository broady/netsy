---
title: "Loading, Startup, & Shutdown"
weight: 50
description: "Outline of how Node Loading, Primary Startup, and graceful Node Shutdown works."
---

# Node Loading, Primary Startup, & Node Shutdown

## Health: From Loading To Healthy

When a Netsy process starts, it enters the `Loading` Health State and performs a "backfill" process:

1. Validates its local peer-certificate subject and server-certificate SANs against the configured `node_id`, `cluster_id`, and advertise addresses.

2. Uses s3lect to determine the current Elector (if exists).

3. Registers itself via [Service Discovery](leader-election.md#service-discovery-for-nodes), if not already registered.

    - Step 1: ensure a Node file exists in object storage. If an existing file is equivalent, treat it as a no-op. If it exists but contains different values such as different advertise addresses, fail startup rather than silently overwriting it.

    - Step 2: connect to the current Elector (if exists) and register itself with the Elector, which allocates or re-uses the Node's stable etcd `member_id` in `members.json` and returns that `member_id` and the authoritative initial Cluster State in the registration response. If direct registration fails, startup fails rather than continuing with a partially registered Node.

4. Initialise its SQLite database.

    - If its SQLite database does not exist, create an initial schema.

    - Check `PRAGMA user_version` against the expected schema version. If different than expected, refuse to start (prevents an older release from using a database written by a newer release). If lower, a future release may migrate in-place or wipe and rebuild from object storage.

5. If the local `compactions` table is empty, derive the Compaction Revision implied by contiguous `records.compacted_at` rows.

6. If the SQLite database `records` table is empty, downloads and imports the latest Snapshot file from object storage (if one exists).

   - While it might be possible to retrieve this data from another Peer at a lower cost, fetching from object storage provides a durable baseline without causing any load on other Peers. When using synchronous object storage transactions, it's the system of record. When using quorum transactions, object storage may not contain the very latest records (as they are flushed asynchronously). Regardless, it is always checked first as the starting point for backfill. This means you can safely bring multiple new Peers online quickly.

7. If the local `compactions` table is still empty after step 6, derive the Compaction Revision again from the now-populated contiguous `records.compacted_at` rows.

8. If the direct registration response in step 3 returned Cluster State, use that as the authoritative current Cluster State. Otherwise, connect to the current Elector (if one exists) to fetch Cluster State, which provides the current Primary (if one exists) and the current Elector / Primary Peer advertise addresses.

9. Connects to the current Primary (if exists) and starts streaming any Records.

   - On every new `Follow` stream, the Primary immediately sends an `Initial` message carrying the current Committed and current Compaction Revision.

   - If local compaction state is still empty after steps 5-8 and the `Initial` message reports a non-zero Compaction Revision, persist that revision locally before admitting Watches.

   - During this time, there may be a "gap" in its records until the next step completes.  

10. Backfills any records between the latest object storage Snapshot and newly replicated Records from the Primary (if exists), by fetching the necessary Chunk files from object storage.

11. Performs an integrity check to ensure there is no missing data. If there is, empty/truncate the table and restarts the backfill process.

Once this has completed successfully, the Health Status changes to `Healthy`.

On a Cluster with only one Node, steps 8-9 are skipped, step 10 completes from object storage only, and after step 11 the Node shortly becomes the Elector, and quickly becomes the Primary.

### Primary: From Starting To Active

For safety, before an elected Primary becomes `Active` it enters the `Starting` Primary State and performs the following "preflight checks":

1. Attempts to download all of the latest chunks from object storage.

  - This is because if it became partitioned from the Primary for a short period less than the unhealthy threshold, it can recover any missing records written to object storage.

  - Since the Peer with the highest revision number must be elected Primary by the Elector, it must already have the latest known data among Peers.

2. Determines the highest revision already durably present in object storage, and uploads any records in its SQLite database above that revision which are not yet in Chunk files in object storage.

  - This is because if a KV Data record has been replicated to the newly elected Primary but was not yet written to object storage by the previous Primary, the new Primary will have data not yet synced to object storage.

  - As part of syncing to object storage it should also perform replication to other Replicas to ensure they also received the records. Replicas which already have copies will ignore the relayed records, and those which do not will write to their SQLite DBs. Watch events for these records are not delivered until `committed_revision` is advanced in step 3.

3. Sets `committed_revision` to its latest revision. All records on the new Primary are now authoritative — including any tentative records from the previous Primary's rolled-back transactions, which are adopted as committed under the new leadership. This `committed_revision` is later sent to Replicas as a logical Commit message on the replication stream, allowing them to serve reads up to this point and to overwrite any tentative records above their own previous `committed_revision` that conflict with the new Primary's data. Whenever a Replica establishes or re-establishes a `Follow` stream, the Primary first sends an `Initial` message carrying the current Committed and Compaction Revision, and later uses `Commit` and `Compact` messages to advance those.

Once all preflight checks complete successfully, the Primary transitions from `Starting` to `Active` and begins accepting writes.

## Graceful Shutdown (Signal Handling)

When a Netsy process receives a termination signal (e.g. `SIGTERM`, `SIGINT`), the shutdown procedure differs depending on whether the Node is a Primary or a Replica.

The key difference is the Primary must flush all buffered data to object storage before deregistering, to ensure no committed records are lost. A Replica has no buffered writes and can deregister immediately.

### Replica Shutdown

1. Move Health State to `Degraded`.
2. Deregister with the current Elector and delete its registration file from object storage (per [Node Deregistration](leader-election.md#node-deregistration)).
3. Close any open gRPC connections (replication stream to Primary, Heartbeat connection to Elector).
4. Exit.

### Primary Shutdown

1. Move Primary State to `Draining`. The Primary stops accepting new writes immediately.
2. Flush the Chunk Buffer to object storage — all buffered records must be written to object storage before proceeding.
3. Move Health State to `Degraded`.
4. Deregister with the current Elector and delete its registration file from object storage (per [Node Deregistration](leader-election.md#node-deregistration)).
5. Close any open gRPC connections.
6. Exit.
