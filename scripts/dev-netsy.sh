#!/usr/bin/env bash
# Netsy <https://netsy.dev>
# Copyright 2026 Nadrama Pty Ltd
# SPDX-License-Identifier: Apache-2.0
#
# Dev server wrapper for running Netsy with Air (live reload).
# Called by overmind via the Procfile — use 'make dev' to start.
set -eo pipefail

CURRENT=$(dirname "$(readlink -f "$0")")
ROOT="${CURRENT}/.."

# AWS S3 (localstack-compatible dev-s3 server)
export AWS_S3_USE_PATH_STYLE=true
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_REGION=us-east-1
export AWS_ENDPOINT_URL=http://localhost:4566

# Netsy per-node settings
export NETSY_CONFIG="${ROOT}/examples/config.jsonc"
export NETSY_NODE_ID=dev-node
export NETSY_DATA_DIR="${ROOT}/temp/data"
export NETSY_BIND_HEALTH=:8080
export NETSY_ADVERTISE_CLIENT=127.0.0.1:2378
export NETSY_ADVERTISE_PEER=127.0.0.1:2381
export NETSY_ADVERTISE_ELECTION=127.0.0.1:8443

# TLS certificates
export NETSY_TLS_CA_CERT="${ROOT}/temp/certs/ca.crt"
export NETSY_TLS_SERVER_CERT="${ROOT}/temp/certs/server.crt"
export NETSY_TLS_SERVER_KEY="${ROOT}/temp/certs/server.key"
export NETSY_TLS_CLIENT_CERT="${ROOT}/temp/certs/peer.crt"
export NETSY_TLS_CLIENT_KEY="${ROOT}/temp/certs/peer.key"

# Ensure temp directory exists
mkdir -p "${ROOT}/temp/data"

# Run Air for live reload
exec air -c "${ROOT}/scripts/.air.toml"
