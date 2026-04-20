---
title: "TLS Certificates"
weight: 20
description: "TLS certificate requirements and generation for Netsy clusters"
---

# TLS Certificates

Netsy uses mutual TLS (mTLS) for authentication for Nodes, Peers, and Clients.

Each Node requires a server certificate (for serving gRPC and the Elector/s3lect election health listener) and a client certificate (for Nodes connecting to Peers).

Presented client certificates embed a role of either `peer` or `client`, and Netsy authorises the connection based on that role.

## Certificate Requirements

All TLS certificates in a Netsy cluster must be signed by the same Certificate Authority (CA).

### Identity via URI SANs

A TLS certificate can carry Subject Alternative Names (SANs) — additional identifiers beyond the certificate's subject line. SANs are commonly DNS names or IP addresses, but they can also be URIs. A URI SAN embeds a structured identifier inside the certificate itself, so the receiving side can parse it to learn *who* is connecting without relying on out-of-band configuration.

Netsy identifies certificates using a URI SAN with the format:

```
netsy://{cluster_id}/{role}/{identity}
```

| Component | Description | Example |
|---|---|---|
| `cluster_id` | Cluster ID (must match Node config) | `my-cluster` |
| `role` | Certificate role | `peer` or `client` |
| `identity` | Peer: `node_id`, Client: arbitrary identifier | `node-1`, `my-app` |

**Peer certificates** (Node-to-Node):

```
netsy://my-cluster/peer/node-1
```

The identity component must match the node's configured `node_id`.

**Client certificates** (external etcd clients):

```
netsy://my-cluster/client/kube-apiserver
```

The identity component is an arbitrary client identifier — no `node_id` matching is required.

### Validation Rules

Both `cluster_id` and `node_id` must be:

- Lowercase alphanumeric characters and hyphens only
- No leading, trailing, or consecutive hyphens
- Maximum 32 characters

### Server Certificate

Used by a Node on the Client API and Peer API gRPC servers. Must include:

- **URI SAN**: `netsy://{cluster_id}/peer/{node_id}`
- **Subject Alternative Names (SANs)**: the advertise addresses and any literal hostnames/IPs that peers or clients actually dial for this Node (Client API, Peer API, and Elector/s3lect peer and health server)
- **Key Usage**: Digital Signature, Key Encipherment
- **Extended Key Usage**: TLS Web Server Authentication

At startup, Netsy validates that the certificate identity matches the configured `node_id` / `cluster_id`, and that the server certificate SANs cover the configured Client, Peer, and election advertise addresses.

### Node Client Certificate

Used by a Node when connecting to other Peer gRPC servers. Must include:

- **URI SAN**: `netsy://{cluster_id}/peer/{node_id}`
- **Key Usage**: Digital Signature
- **Extended Key Usage**: TLS Web Client Authentication

### Client API Client Certificates

External "etcd clients" are not cluster Nodes. When a Client connects to the Client API with mTLS, Netsy verifies the URI SAN has the `client` role and the correct `cluster_id`. Only `peer` certificates are required to match a Node's `node_id`.

## Generating Certificates with OpenSSL

The examples below use the following variables:

```bash
CLUSTER_ID="my-cluster"
NODE_ID="node-1"
DAYS=365

# Addresses for SANs (adjust to your environment)
CLIENT_ADDR_IP4="172.16.0.1"
CLIENT_ADDR_IP6="2001:db8::10"
PEER_ADDR_IP4="172.16.0.1"
PEER_ADDR_IP6="2001:db8::10"
ELECTOR_ADDR_IP4="172.16.0.1"
ELECTOR_ADDR_IP6="2001:db8::10"
```

Note that if you want to use DNS names instead of IPs for the Client, Peer, and Elector (s3lect) addresses the examples below will need to be adjusted accordingly. Include SANs only for the actual addresses or hostnames that callers use.

### 1. Create the Cluster CA

Generate a CA certificate and key. This CA is shared across all Nodes in the cluster.

```bash
openssl genrsa -out ca.key 4096

openssl req -x509 -new -nodes \
  -key ca.key \
  -sha256 \
  -days $DAYS \
  -out ca.crt \
  -subj "/O=${CLUSTER_ID}/CN=${CLUSTER_ID}-ca"
```

### 2. Generate the Node Server Certificate

Create a config file for the server certificate SANs:

```bash
cat > server.cnf <<EOF
[req]
distinguished_name = req_dn
req_extensions = v3_req
prompt = no

[req_dn]
CN = ${NODE_ID}

[v3_req]
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
IP.1 = ${CLIENT_ADDR_IP4}
IP.2 = ${CLIENT_ADDR_IP6}
IP.3 = ${PEER_ADDR_IP4}
IP.4 = ${PEER_ADDR_IP6}
IP.5 = ${ELECTOR_ADDR_IP4}
IP.6 = ${ELECTOR_ADDR_IP6}
IP.7 = 127.0.0.1
IP.8 = ::1
DNS.1 = localhost
URI.1 = netsy://${CLUSTER_ID}/peer/${NODE_ID}
EOF
```

Generate the server key, CSR, and signed certificate:

```bash
openssl genrsa -out server.key 4096

openssl req -new \
  -key server.key \
  -out server.csr \
  -config server.cnf

openssl x509 -req \
  -in server.csr \
  -CA ca.crt \
  -CAkey ca.key \
  -CAcreateserial \
  -out server.crt \
  -days $DAYS \
  -sha256 \
  -extensions v3_req \
  -extfile server.cnf
```

### 3. Generate the Node Client Certificate

Create a config file for the client certificate:

```bash
cat > client.cnf <<EOF
[req]
distinguished_name = req_dn
req_extensions = v3_req
prompt = no

[req_dn]
CN = ${NODE_ID}

[v3_req]
keyUsage = digitalSignature
extendedKeyUsage = clientAuth
subjectAltName = @alt_names

[alt_names]
URI.1 = netsy://${CLUSTER_ID}/peer/${NODE_ID}
EOF
```

Generate the client key, CSR, and signed certificate:

```bash
openssl genrsa -out client.key 4096

openssl req -new \
  -key client.key \
  -out client.csr \
  -config client.cnf

openssl x509 -req \
  -in client.csr \
  -CA ca.crt \
  -CAkey ca.key \
  -CAcreateserial \
  -out client.crt \
  -days $DAYS \
  -sha256 \
  -extensions v3_req \
  -extfile client.cnf
```

### 4. Verify Certificates

Verify the subject fields and SANs are correct:

```bash
# Check server certificate
openssl x509 -in server.crt -noout -subject -ext subjectAltName
# Expected: subject=CN=node-1
# Expected SAN: URI:netsy://my-cluster/peer/node-1, IP:..., DNS:localhost

# Check client certificate
openssl x509 -in client.crt -noout -subject -ext subjectAltName
# Expected: subject=CN=node-1
# Expected SAN: URI:netsy://my-cluster/peer/node-1

# Verify both are signed by the CA
openssl verify -CAfile ca.crt server.crt
openssl verify -CAfile ca.crt client.crt
```

## Netsy Configuration

Once certificates are generated, configure the Node. TLS cert paths and node identity are per-node settings (env vars), while `cluster_id` is a per-cluster setting (config file).

Per-node env vars:

```bash
export NETSY_NODE_ID="node-1"
export NETSY_TLS_CA_CERT=./certs/ca.crt
export NETSY_TLS_SERVER_CERT=./certs/server.crt
export NETSY_TLS_SERVER_KEY=./certs/server.key
export NETSY_TLS_CLIENT_CERT=./certs/peer.crt
export NETSY_TLS_CLIENT_KEY=./certs/peer.key
```

Per-cluster config file (see [Configuration](config.md) for the full reference):

```jsonc
{
  "cluster_id": "my-cluster"
}
```

## Multi-Node Example

For a 3-node cluster, repeat steps 2–3 for each node with a different `NODE_ID` and the appropriate addresses. The CA (step 1) is created once and shared. All nodes use the same `ca.crt` and cluster config file.

```bash
# Node 1
NODE_ID="node-1" CLIENT_ADDR_V4="172.16.0.1" CLIENT_ADDR_V6="2001:db8::11" PEER_ADDR_V4="172.16.0.1" PEER_ADDR_V6="2001:db8::11" ELECTOR_ADDR_V4="172.16.0.1" ELECTOR_ADDR_V6="2001:db8::11"

# Node 2
NODE_ID="node-2" CLIENT_ADDR_V4="172.16.0.2" CLIENT_ADDR_V6="2001:db8::12" PEER_ADDR_V4="172.16.0.2" PEER_ADDR_V6="2001:db8::12" ELECTOR_ADDR_V4="172.16.0.2" ELECTOR_ADDR_V6="2001:db8::12"

# Node 3
NODE_ID="node-3" CLIENT_ADDR_V4="172.16.0.3" CLIENT_ADDR_V6="2001:db8::13" PEER_ADDR_V4="172.16.0.3" PEER_ADDR_V6="2001:db8::13" ELECTOR_ADDR_V4="172.16.0.3" ELECTOR_ADDR_V6="2001:db8::13"
```
