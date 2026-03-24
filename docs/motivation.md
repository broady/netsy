---
title: "Motivation"
weight: 10
description: "Background context on why Netsy was created"
---

# Motivation for creating Netsy

## Why create an etcd alternative?

We want to make infrastructure easy.

Running servers that store persistent state can be challenging to get right.

S3 (and S3-compatible) object storage is the de facto solution for simple, reliable, durable storage.

There has been a recent trend in new system designs which leverage S3 as the primary data store.
For example, instead of ELK for logs and Prometheus for metrics, you can use [Loki](https://github.com/grafana/loki)+[Mimir](https://github.com/grafana/mimir) or [OpenObserve](https://github.com/openobserve/openobserve).

By relying on S3 instead of local filesystems, operators are able to treat VMs "like cattle, not pets".
That is to say, VMs become easily replaceable - no longer something to individually manage and maintain.

VM deployments can be as simple as an Auto-Scaling Group (ASG) with some userdata.
Fixing issues is as simple as deleting one VM, and waiting for a new one to come online.

And with ASGs, scaling up and down is greatly simplified - enabling operators to reduce costs.
In fact, most of these systems can often scale down to just a single VM - also great for non-production environments. And some support scale-to-zero, perfect for dev or test environments!

When we looked at the options for how to approach managing Kubernetes and etcd in production, the challenges were:

1. etcd requires 3 nodes (VMs) for fault tolerance.

2. etcd stores data to disk, requiring careful management of persistent volumes.

3. snapshots of etcd are asynchronous, so if a single node cluster VM shutdown even milliseconds after,
   you may lose data unless you correctly manage the persistent volumes.

[Kine](https://github.com/k3s-io/kine/) is an Open Source Kubernetes-compatible etcd alternative.
It's a great project which enables you to use SQLite or an external SQL databases such as MySQL or PostgreSQL.
However, kine is often not recommended for production environment for reasons such as:

1. All reads and writes go to a single database endpoint.

2. Watches are implemented by polling the database.

3. It does not implement etcd leases.

We considered using it with tools like [Litestream](https://github.com/benbjohnson/litestream) to stream
the WAL to S3. However, this would limit reads and writes to a single kine instance.
Additionally, stream WAL files are asynchronous and difficult to guarentee completion prior to system shutdown.

What we wanted is something we knew would ensure data was safely stored in S3 even if a VM is shutdown and deleted.

And so, we created Netsy.
