---
title: "Loading & Startup"
weight: 50
description: "Outline of how Node Loading and Primary Startup states work."
---

# Node Loading & Primary Startup Process

## Health: From Loading To Healthy

When a Netsy process starts, it enters the `Loading` Health State and performs a "backfill" process:

1. If its SQLite database does not exist, it initialises its schema.  

2. Any unapplied schema migrations are run.  
     
3. If the SQLite database `records` table is empty, downloads and imports the latest Snapshot file from object storage (if one exists).  

   - While it might be possible to retrieve this data from another Peer at a lower cost, by fetching it from object storage it ensures that it is receiving the most up-to-date source-of-truth data, and does so without causing any load on other Peers. This means you can safely bring multiple new Peers online quickly.

3. Identifies and connects to the current Leader and starts streaming any Records.

   - During this time, there may be a "gap" in its records until the next step completes.  
    
4. Backfills any records between the Snapshot and newly replicated Records from the Leader, by fetching the necessary Chunk files from object storage.  
     
5. Performs an integrity check to ensure there is no missing data. If there is, empty the table and restarts the process to restart backfilling.

Once this has completed successfully, the Health Status changes to `Healthy`.

On a Cluster with only one Node, the Node shortly becomes the Elector, and quickly becomes the Primary.

### Primary: From Starting To Active

For safety, before an elected Primary becomes `Active` it enters the `Starting` Primary State and performs the following "preflight checks":

1. Attempts to download all of the latest chunks from object storage.

  - This is because if it became partitioned from the Primary for a short period less than the unhealthy threshold, it can recover any missing records written to object storage.

  - Since the Peer with the highest revision number must be elected Primary by the Elector, it must already have the latest known data among Peers.

2. Uploads any records in its SQLite database which are not yet in chunk files in object storage.

  - This is because if a KV Data record has been replicated to the newly elected Primary but was not yet written to object storage by the previous Primary, the new Primary will have data not yet synced to object storage.

  - As part of syncing to object storage it should also perform replication to other Replicas and watchers to ensure they also received the records. Replicas which already have copies will ignore the relayed records, and those which do not will write to their SQLite DBs and send to their watchers.
