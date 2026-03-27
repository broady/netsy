---
title: "System Design"
weight: 30
description: "Netsy concepts and system design for data files, multi-node/replication, and watches/compaction"
---

## In This Section

- [Overview](overview.md) – Overview of Netsy terminology, requirements, leader election, and startup process.
- [Leader Election](leader-election.md) - Netsy two-tier leader election system design.
- [Netsy Data Files](data-files.md) – Netsy (.netsy) data file format/specification.
- [Storage & Replication](storage-replication.md) – Netsy data storage and replication system design.
- [Loading & Startup](loading-startup.md) - Outline of how Node Loading and Primary Startup states work.
- [Failure Scenarios](failure-scenarios.md) – Data integrity and safety analysis across quorum configurations and cluster sizes.
- [Watches & Compaction](watches-compaction.md) – Watch support & Compaction system design.
