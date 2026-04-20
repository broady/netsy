# URI SAN Identity Migration

## Summary

Migrate Netsy's mTLS identity from X.509 subject fields (Organization and Organizational Unit) to URI Subject Alternative Names (SANs), enabling native cert-manager CSI driver support and eliminating init containers from the Kubernetes Helm chart.

## Problem

Netsy currently encodes cluster identity and peer role in X.509 certificate subject fields:

- **Organization (O)** = `cluster_id`
- **Organizational Unit (OU)** = `peer` or `client`

The cert-manager CSI driver — which mints unique per-pod certificates inline via a CSI volume — does not support setting O or OU fields. This would force the Helm chart to use an init container that selects the correct pre-generated certificate for each StatefulSet pod based on its ordinal index, rather than being able to use the cert-manager CSI driver which is preferred.

## Solution

Replace O/OU-based identity with URI SAN-based identity using a SPIFFE-inspired format:

```
netsy://{cluster_id}/{role}/{node_id}
```

Examples:

```
netsy://my-cluster/peer/node-1
netsy://my-cluster/client/my-app
```

The cert-manager CSI driver supports URI SANs with pod-level variable substitution (`${POD_NAME}`, `${POD_NAMESPACE}`), enabling fully dynamic per-pod certificate generation without init containers or pre-created Certificate resources.

## URI Format

```
netsy://{cluster_id}/{role}/{identity}
```

| Component | Description | Example |
|---|---|---|
| `cluster_id` | Cluster identifier (validated against config) | `my-cluster` |
| `role` | Certificate role | `peer` or `client` |
| `identity` | Peer: `node_id`, Client: arbitrary identifier | `node-1`, `my-app` |

### Peer Certificates (Node-to-Node)

Used by Nodes for Peer API connections. The identity component must match the node's configured `node_id`.

```
netsy://my-cluster/peer/node-1
```

### Client Certificates (External Clients)

Used by external etcd clients connecting to the Client API. The identity component is an arbitrary client identifier — no `node_id` matching is required.

```
netsy://my-cluster/client/kube-apiserver
```

## Validation Changes

### Current Validation (O/OU)

```go
// Peer certificate
O == cluster_id   // from config
OU == "peer"
CN == node_id     // for peer certs only
```

### New Validation (URI SAN)

```go
// Parse first netsy:// URI SAN
uri.Scheme == "netsy"
uri.Host == cluster_id    // from config
role := path[1]           // "peer" or "client"
identity := path[2]       // node_id or client name

// Peer certificate: identity must match node_id
// Client certificate: identity is informational only
```

CN continues to be set to `node_id` (or client name) for logging and debugging, but is no longer used for authorization decisions.

## Changes Required

### 1. mTLS Authentication (`internal/mtls/`)

- Add URI SAN parsing function that extracts `cluster_id`, `role`, and `identity` from the first `netsy://` URI SAN
- Replace O-field cluster validation with URI SAN cluster validation
- Replace OU-field role extraction with URI SAN role extraction
- Keep CN validation for peer certs as a secondary check (belt and suspenders), or remove it entirely in favour of the URI identity field
- Update `auth_test.go` with new certificate fixtures

### 2. Startup Certificate Validation (`internal/mtls/`)

- Update local peer certificate subject validation to check URI SAN instead of O/OU
- Update server certificate SAN validation (no change needed — DNS/IP SANs are unaffected)

### 3. Certificate Generation Scripts (`scripts/certs.sh`)

- Add URI SAN to server and client certificate configs:
  ```
  [alt_names]
  URI.1 = netsy://${CLUSTER_ID}/peer/${NODE_ID}
  ```
- Keep O/OU in the subject for human readability (optional, no longer validated)

### 4. TLS Documentation (`docs/deployment/tls.md`)

- Document URI SAN format and requirements
- Update OpenSSL certificate generation examples to include URI SANs
- Note that O/OU are optional and no longer validated

### 5. Helm Chart (`deploy/helm/`)

- Replace `statefulset-certmanager.yaml` init container approach with CSI driver volumes:
  ```yaml
  volumes:
    - name: tls
      csi:
        driver: csi.cert-manager.io
        readOnly: true
        volumeAttributes:
          csi.cert-manager.io/issuer-name: netsy-ca
          csi.cert-manager.io/issuer-kind: Issuer
          csi.cert-manager.io/common-name: "${POD_NAME}"
          csi.cert-manager.io/dns-names: "${POD_NAME}.netsy.${POD_NAMESPACE}.svc.cluster.local,localhost"
          csi.cert-manager.io/ip-sans: "127.0.0.1,::1"
          csi.cert-manager.io/uri-sans: "netsy://CLUSTER_ID/peer/${POD_NAME}"
          csi.cert-manager.io/key-usages: "digital signature,key encipherment,server auth"
          csi.cert-manager.io/duration: "2160h"
          csi.cert-manager.io/renew-before: "720h"
          csi.cert-manager.io/certificate-file: server.crt
          csi.cert-manager.io/privatekey-file: server.key
          csi.cert-manager.io/ca-file: ca.crt
    - name: client-tls
      csi:
        driver: csi.cert-manager.io
        readOnly: true
        volumeAttributes:
          csi.cert-manager.io/issuer-name: netsy-ca
          csi.cert-manager.io/issuer-kind: Issuer
          csi.cert-manager.io/common-name: "${POD_NAME}"
          csi.cert-manager.io/uri-sans: "netsy://CLUSTER_ID/peer/${POD_NAME}"
          csi.cert-manager.io/key-usages: "digital signature,client auth"
          csi.cert-manager.io/duration: "2160h"
          csi.cert-manager.io/renew-before: "720h"
          csi.cert-manager.io/certificate-file: peer.crt
          csi.cert-manager.io/privatekey-file: peer.key
  ```
- Remove `certificates.yaml` template (no longer needed — CSI driver creates certs on the fly)
- Remove init container and per-pod secret volumes from cert-manager StatefulSet
- Keep `ca.yaml` for the CA issuer chain
- Keep `statefulset-manual.yaml` unchanged (manual mode still uses init container with pre-created secrets)
- Add `tls.certManager.csiDriver` boolean (default `true`) to control whether to use CSI driver or fall back to Certificate resources + init container

### 6. Helm Chart Prerequisites

- Document that `cert-manager-csi-driver` must be installed alongside cert-manager when using the default CSI driver mode
- Add a check or clear error message if the CSI driver is not available

### 7. Configuration Documentation (`docs/deployment/config.md`)

- No config changes needed — `cluster_id` and `node_id` validation rules are unchanged

### 8. Kubernetes Documentation (`docs/deployment/kubernetes.md`)

- Update cert-manager section to explain CSI driver approach
- Document `cert-manager-csi-driver` as a prerequisite
- Remove init container explanation

## Backward Compatibility

- Certificates with O/OU fields (no URI SAN) should continue to work during a transition period
- Validation logic should check for URI SAN first; if not present, fall back to O/OU validation
- A future major version can remove the O/OU fallback

## Implementation Plan

1. Add URI SAN parsing and validation to `internal/mtls/`
2. Update mTLS authentication to prefer URI SAN over O/OU (with O/OU fallback)
3. Update startup certificate validation
4. Update `scripts/certs.sh` to include URI SANs in generated dev certs
5. Update `docs/deployment/tls.md` with URI SAN documentation
6. Update Helm chart cert-manager StatefulSet to use CSI driver volumes
7. Update `docs/deployment/kubernetes.md`
8. Update mTLS tests
