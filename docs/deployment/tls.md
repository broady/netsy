---
title: "TLS Certificates"
weight: 10
description: "TLS certificate requirements and generation for Netsy clusters"
---

# TLS Certificates

Netsy uses mutual TLS (mTLS) for authentication for Nodes, Peers, and Clients.

Each Node requires a server certificate (for serving gRPC) and a client certificate (for Nodes connecting to Peers).

Presented client certificates embed a role of either `peer` or `client`, and Netsy authorises the connection based on that role.

## Certificate Requirements

All TLS certificates in a Netsy cluster must be signed by the same Certificate Authority (CA).

### Identity Fields

Certificates used for peer-to-peer communication and client authentication embed identity and role information that Netsy validates during mTLS authentication:

- **Organization (O)**: must match the `cluster_id` configured on the Node. This prevents Nodes and Clients from accidentally joining/connecting to the wrong cluster, even if multiple clusters share the same CA.
- **Organizational Unit (OU)**: must be the Role, either `peer` or `client`
  - `peer` certificates: identify a cluster Node and are used for Node-to-Node communication connecting to the Peer API.
  - `client` certificates: identify an external "etcd client" connecting to the Client API.
- **Common Name (CN)**: the identifier of the Peer/Client
  - for Nodes, must match the `node_id` configured on the Node. This prevents impersonation of other Nodes.

### Client API Client Certificates

External "etcd clients" are not cluster Nodes. When a Client connects to the Client API with mTLS, Netsy verifies that the certificate chains to the configured CA and has the `client` role. Only `peer` certificates are required to match a Node's `node_id`.

### Validation Rules

Both `cluster_id` and `node_id` must be:

- Lowercase alphanumeric characters and hyphens only
- No leading, trailing, or consecutive hyphens
- Maximum 32 characters

### Server Certificate

Used by a Node on the Client API and Peer API gRPC servers. Must include:

- **Subject**: `O={cluster_id}, OU=peer, CN={node_id}`
- **Subject Alternative Names (SANs)**: all bind and advertise addresses for this Node (Client API, Peer API, and s3lect health server)
- **Key Usage**: Digital Signature, Key Encipherment
- **Extended Key Usage**: TLS Web Server Authentication

### Node Client Certificate

Used by a Node when connecting to other Peer gRPC servers. Must include:

- **Subject**: `O={cluster_id}, OU=peer, CN={node_id}`
- **Key Usage**: Digital Signature
- **Extended Key Usage**: TLS Web Client Authentication

## Generating Certificates with OpenSSL

The examples below use the following variables:

```bash
CLUSTER_ID="my-cluster"
NODE_ID="node-1"
DAYS=365

# Addresses for SANs (adjust to your environment)
CLIENT_ADDR_IP="172.16.0.1"
PEER_ADDR_IP="172.16.0.1"
ELECTOR_ADDR_IP="172.16.0.1"
```

Note that if you want to use DNS names instead of IPs for the Client, Peer, and Elector (s3lect) addresses the examples below will need to be adjusted accordingly.

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
O = ${CLUSTER_ID}
OU = peer
CN = ${NODE_ID}

[v3_req]
keyUsage = digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
IP.1 = ${CLIENT_ADDR_IP}
IP.2 = ${PEER_ADDR_IP}
IP.3 = ${ELECTOR_ADDR_IP}
IP.4 = 127.0.0.1
DNS.1 = localhost
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
O = ${CLUSTER_ID}
OU = peer
CN = ${NODE_ID}

[v3_req]
keyUsage = digitalSignature
extendedKeyUsage = clientAuth
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
# Expected: subject=O=my-cluster, OU=peer, CN=node-1

# Check client certificate
openssl x509 -in client.crt -noout -subject
# Expected: subject=O=my-cluster, OU=peer, CN=node-1

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
export NETSY_TLS_CLIENT_CERT=./certs/client.crt
export NETSY_TLS_CLIENT_KEY=./certs/client.key
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
NODE_ID="node-1" CLIENT_ADDR="172.16.0.1" PEER_ADDR="172.16.0.1" S3LECT_ADDR="172.16.0.1"

# Node 2
NODE_ID="node-2" CLIENT_ADDR="172.16.0.2" PEER_ADDR="172.16.0.2" S3LECT_ADDR="172.16.0.2"

# Node 3
NODE_ID="node-3" CLIENT_ADDR="172.16.0.3" PEER_ADDR="172.16.0.3" S3LECT_ADDR="172.16.0.3"
```
