#!/usr/bin/env bash
# Netsy <https://netsy.dev>
# Copyright 2026 Nadrama Pty Ltd
# SPDX-License-Identifier: Apache-2.0
#
# Generates development TLS certificates for local Netsy development.
# Certificates follow the TLS requirements documented in docs/deployment/tls.md.
#
# Usage: ./scripts/certs.sh
#
# Generated files in temp/certs/:
#
#   File                  CN           URI SAN                                    Purpose
#   ──────────────────────────────────────────────────────────────────────────────────────────────
#   ca.crt / ca.key       dev-cluster  —                                          Cluster CA (signs all certs)
#   server.crt / .key     dev-node     netsy://dev-cluster/peer/dev-node          Node gRPC server (Client, Peer, election APIs)
#   peer.crt / .key       dev-node     netsy://dev-cluster/peer/dev-node          Node connecting to other Nodes' Peer API
#   client.crt / .key     etcd-client  netsy://dev-cluster/client/etcd-client     External tools (etcdctl, kube-apiserver)
#   service-account.key   —            —                                          Kubernetes service account signing key
#
set -eo pipefail

CURRENT=$(dirname "$(readlink -f "$0")")
CERTS_DIR="${CURRENT}/../temp/certs"

CLUSTER_ID="dev-cluster"
NODE_ID="dev-node"
DAYS_CA=3650
DAYS_CERT=365

# Idempotent: skip if certs already exist
if [ -d "${CERTS_DIR}" ]; then
    echo "Development certificates already exist in temp/certs/."
    echo "To regenerate, run: rm -rf temp/certs/"
    exit 0
fi

command -v openssl >/dev/null 2>&1 || { echo >&2 "openssl is required but not installed. Aborting."; exit 1; }

mkdir -p "${CERTS_DIR}"

echo "Generating development TLS certificates..."
echo "  Cluster ID: ${CLUSTER_ID}"
echo "  Node ID:    ${NODE_ID}"
echo ""

# --- CA (RSA 4096, self-signed) ---
echo "Generating CA key and certificate..."
openssl genrsa -out "${CERTS_DIR}/ca.key" 4096 2>/dev/null
openssl req -x509 -new -nodes \
    -key "${CERTS_DIR}/ca.key" \
    -sha256 \
    -days ${DAYS_CA} \
    -out "${CERTS_DIR}/ca.crt" \
    -subj "/O=${CLUSTER_ID}/CN=${CLUSTER_ID}-ca"

# --- Server certificate (CN=node, URI SAN=netsy://cluster/peer/node) ---
echo "Generating server certificate..."
cat > "${CERTS_DIR}/server.cnf" <<EOF
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
DNS.1 = localhost
DNS.2 = host.containers.internal
IP.1 = 127.0.0.1
IP.2 = ::1
URI.1 = netsy://${CLUSTER_ID}/peer/${NODE_ID}
EOF

openssl genrsa -out "${CERTS_DIR}/server.key" 4096 2>/dev/null
openssl req -new \
    -key "${CERTS_DIR}/server.key" \
    -out "${CERTS_DIR}/server.csr" \
    -config "${CERTS_DIR}/server.cnf"
openssl x509 -req \
    -in "${CERTS_DIR}/server.csr" \
    -CA "${CERTS_DIR}/ca.crt" \
    -CAkey "${CERTS_DIR}/ca.key" \
    -CAcreateserial \
    -out "${CERTS_DIR}/server.crt" \
    -days ${DAYS_CERT} \
    -sha256 \
    -extensions v3_req \
    -extfile "${CERTS_DIR}/server.cnf"

# --- Peer client certificate (CN=node, URI SAN=netsy://cluster/peer/node) ---
echo "Generating peer client certificate..."
cat > "${CERTS_DIR}/peer.cnf" <<EOF
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

openssl genrsa -out "${CERTS_DIR}/peer.key" 4096 2>/dev/null
openssl req -new \
    -key "${CERTS_DIR}/peer.key" \
    -out "${CERTS_DIR}/peer.csr" \
    -config "${CERTS_DIR}/peer.cnf"
openssl x509 -req \
    -in "${CERTS_DIR}/peer.csr" \
    -CA "${CERTS_DIR}/ca.crt" \
    -CAkey "${CERTS_DIR}/ca.key" \
    -CAcreateserial \
    -out "${CERTS_DIR}/peer.crt" \
    -days ${DAYS_CERT} \
    -sha256 \
    -extensions v3_req \
    -extfile "${CERTS_DIR}/peer.cnf"

# --- External client certificate (CN=etcd-client, URI SAN=netsy://cluster/client/etcd-client) ---
echo "Generating client certificate..."
cat > "${CERTS_DIR}/client.cnf" <<EOF
[req]
distinguished_name = req_dn
req_extensions = v3_req
prompt = no

[req_dn]
CN = etcd-client

[v3_req]
keyUsage = digitalSignature
extendedKeyUsage = clientAuth
subjectAltName = @alt_names

[alt_names]
URI.1 = netsy://${CLUSTER_ID}/client/etcd-client
EOF

openssl genrsa -out "${CERTS_DIR}/client.key" 4096 2>/dev/null
openssl req -new \
    -key "${CERTS_DIR}/client.key" \
    -out "${CERTS_DIR}/client.csr" \
    -config "${CERTS_DIR}/client.cnf"
openssl x509 -req \
    -in "${CERTS_DIR}/client.csr" \
    -CA "${CERTS_DIR}/ca.crt" \
    -CAkey "${CERTS_DIR}/ca.key" \
    -CAcreateserial \
    -out "${CERTS_DIR}/client.crt" \
    -days ${DAYS_CERT} \
    -sha256 \
    -extensions v3_req \
    -extfile "${CERTS_DIR}/client.cnf"

# --- Service account key (RSA 2048, Ed25519 not supported by K8s) ---
echo "Generating service account key..."
openssl genrsa -out "${CERTS_DIR}/service-account.key" 2048 2>/dev/null

# Clean up temporary files
rm -f "${CERTS_DIR}"/*.csr "${CERTS_DIR}"/*.cnf "${CERTS_DIR}"/*.srl

echo ""
echo "Development certificates generated in temp/certs/:"
ls -1 "${CERTS_DIR}"
