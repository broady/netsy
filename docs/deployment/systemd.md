---
title: "Systemd"
weight: 30
description: "Example systemd unit file"
---

# Systemd

To run Netsy as a systemd service:

```
[Unit]
Description=Netsy
Wants=network-online.target
After=network-online.target
[Service]
Type=exec
Restart=always
User=netsy
Group=netsy
EnvironmentFile=/etc/netsy/env
ExecStart=/usr/bin/netsy
[Install]
WantedBy=multi-user.target
```

The environment file (`/etc/netsy/env`) must include the per-node settings and the path to the cluster config file. See [Configuration](config.md) for the full reference.

Example `/etc/netsy/env`:

```bash
NETSY_CONFIG=/etc/netsy/config.jsonc
NETSY_NODE_ID=node-1
NETSY_BIND_CLIENT=:2378
NETSY_ADVERTISE_CLIENT=172.16.0.1:2378
NETSY_BIND_PEER=:2381
NETSY_ADVERTISE_PEER=172.16.0.1:2381
NETSY_BIND_ELECTION=:8443
NETSY_ADVERTISE_ELECTION=172.16.0.1:8443
NETSY_BIND_HEALTH=:8080
NETSY_TLS_CA_CERT=/etc/netsy/certs/ca.crt
NETSY_TLS_SERVER_CERT=/etc/netsy/certs/server.crt
NETSY_TLS_SERVER_KEY=/etc/netsy/certs/server.key
NETSY_TLS_CLIENT_CERT=/etc/netsy/certs/peer.crt
NETSY_TLS_CLIENT_KEY=/etc/netsy/certs/peer.key
NETSY_DATA_DIR=/var/lib/netsy
```
