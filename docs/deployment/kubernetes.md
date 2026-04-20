---
title: "Kubernetes"
weight: 40
description: "Deploying Netsy on Kubernetes using the Helm chart"
---

# Kubernetes

Netsy provides a Helm chart at `deploy/helm/` for deploying to Kubernetes.

## Why StatefulSet?

The chart deploys Netsy as a StatefulSet, not a Deployment. Netsy nodes require stable identity across restarts:

- **mTLS certificates**: each node's certificate CN must match its `node_id`. StatefulSet pod names are predictable (`netsy-0`, `netsy-1`, ...), so the cert-manager CSI driver can mint per-pod certificates dynamically. Deployments produce random pod names, making certificate identity unstable.
- **Node registration**: each `node_id` is assigned a stable etcd `member_id` in object storage (`members.json`). Random pod names would leak stale registrations on every restart.
- **Peer discovery**: each node registers in object storage under `nodes/{node_id}.json`. Stable names keep these files valid across restarts.

Persistent storage is not required — Netsy rebuilds its local SQLite database from object storage on startup. The chart uses `emptyDir` by default, with an option to enable PersistentVolumeClaims for faster restarts.

## Prerequisites

- Kubernetes 1.26+
- Helm 3
- [cert-manager](https://cert-manager.io/)
- [cert-manager CSI driver](https://cert-manager.io/docs/usage/csi-driver/) (`csi.cert-manager.io`)
- Object storage bucket (S3 or GCS) with appropriate IAM permissions

## Install

```bash
helm install netsy deploy/helm/ --set clusterID=my-cluster
```

The chart creates a self-signed CA via cert-manager and uses the CSI driver to mint unique per-pod certificates automatically. No manual certificate management required.

## How TLS Works

The chart uses the cert-manager CSI driver to dynamically generate TLS certificates for each pod. This means:

- **No init containers** — certificates are mounted directly via CSI volumes
- **No pre-created Secrets** — the CSI driver creates ephemeral certificates on the fly
- **Automatic renewal** — certificates are renewed transparently before expiry
- **Unique per pod** — each pod gets its own certificate with the correct identity

The chart creates a CA issuer chain via cert-manager (self-signed issuer -> CA certificate -> CA issuer). The CSI driver uses this CA to sign per-pod certificates with:

- `CN={pod-name}` (matches `NETSY_NODE_ID`)
- URI SAN: `netsy://{clusterID}/peer/{pod-name}` (encodes cluster identity and role)
- DNS SANs covering the pod's headless service DNS name

```yaml
tls:
  caDuration: "87600h"    # CA lifetime (default: 10 years)
  duration: "2160h"       # Pod cert lifetime (default: 90 days)
  renewBefore: "720h"     # Renew 30 days before expiry
```

## Configuration

### Cluster ID

The `clusterID` value must match the `cluster_id` in the config file. It is used in URI SANs for mTLS identity and is validated by the schema.

```bash
helm install netsy deploy/helm/ --set clusterID=my-cluster
```

### Replicas

The default is 3 replicas, which is the minimum for majority quorum (`"quorum": -1`).

### Cluster Config

The cluster config file is provided via the `config` value and mounted as a ConfigMap. See [Configuration](config.md) for the full reference.

### Persistence

By default, the data directory uses `emptyDir` — Netsy rebuilds from object storage on every restart. Enable persistence for faster restarts by preserving the local SQLite database across pod restarts:

```yaml
persistence:
  enabled: true
  size: "1Gi"
  # storageClass: "gp3"
```

### AWS Credentials

On EKS, use IAM Roles for Service Accounts (IRSA):

```yaml
serviceAccount:
  annotations:
    eks.amazonaws.com/role-arn: arn:aws:iam::123456789012:role/netsy
```

On non-EKS clusters, use [kube2iam](https://github.com/jtblin/kube2iam) or [kiam](https://github.com/uswitch/kiam):

```yaml
podAnnotations:
  iam.amazonaws.com/role: arn:aws:iam::123456789012:role/netsy
```

As a last resort, pass credentials directly via `extraEnv`:

```yaml
extraEnv:
  - name: AWS_ACCESS_KEY_ID
    valueFrom:
      secretKeyRef:
        name: aws-credentials
        key: access-key-id
  - name: AWS_SECRET_ACCESS_KEY
    valueFrom:
      secretKeyRef:
        name: aws-credentials
        key: secret-access-key
  - name: AWS_DEFAULT_REGION
    value: us-east-1
```

### GCS Credentials

On GKE, use Workload Identity:

```yaml
serviceAccount:
  annotations:
    iam.gke.io/gcp-service-account: netsy@my-project.iam.gserviceaccount.com
```

## Connecting kube-apiserver

Configure kube-apiserver with all Netsy pod endpoints via the headless service DNS names:

```
--etcd-servers=https://netsy-0.netsy.default.svc.cluster.local:2378,https://netsy-1.netsy.default.svc.cluster.local:2378,https://netsy-2.netsy.default.svc.cluster.local:2378
```

Adjust the namespace and release name to match your deployment.

## DNS-Based Advertise Addresses

The chart uses DNS-based advertise addresses derived from the headless service:

```
{pod-name}.{release-name}.{namespace}.svc.cluster.local:{port}
```

This is more stable than pod IP-based addressing — DNS names survive pod restarts without changing, which keeps node registration files in object storage valid and ensures TLS certificate SANs remain correct.

The headless service sets `publishNotReadyAddresses: true` so that DNS records are available during pod startup, before the readiness probe passes.

## Values Reference

| Key | Default | Description |
|---|---|---|
| `replicas` | `3` | Number of Netsy pods |
| `clusterID` | `netsy` | Cluster identifier (must match `cluster_id` in config) |
| `image.repository` | `ghcr.io/netsy-dev/netsy` | Container image repository |
| `image.tag` | Chart `appVersion` | Container image tag |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy |
| `tls.caDuration` | `87600h` | CA certificate lifetime |
| `tls.duration` | `2160h` | Pod certificate lifetime (90 days) |
| `tls.renewBefore` | `720h` | Renew certificates this long before expiry |
| `config` | See `values.yaml` | Cluster config file content (JSONC) |
| `persistence.enabled` | `false` | Enable PersistentVolumeClaims for data directory |
| `persistence.size` | `1Gi` | PVC size |
| `persistence.storageClass` | — | PVC storage class |
| `resources` | `{}` | CPU/memory requests and limits |
| `serviceAccount.create` | `true` | Create a ServiceAccount |
| `serviceAccount.name` | — | ServiceAccount name override |
| `serviceAccount.annotations` | `{}` | ServiceAccount annotations (e.g. IRSA, Workload Identity) |
| `extraEnv` | `[]` | Additional environment variables |
| `podAnnotations` | `{}` | Pod annotations |
| `podLabels` | `{}` | Pod labels |
| `nodeSelector` | `{}` | Node selector |
| `tolerations` | `[]` | Tolerations |
| `affinity` | `{}` | Affinity rules |
