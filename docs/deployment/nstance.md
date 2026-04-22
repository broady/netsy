---
title: "Nstance"
weight: 70
description: "Deploying Netsy to auto-scaled VMs using Nstance"
---

# Deploying Netsy to auto-scaled VMs using Nstance

[Nstance](https://nstance.dev) is a next-gen VM auto-scaler. It manages the lifecycle of VM instances, and has an integrated certificate authority for fast and secure certificate issuance and renewal/rotation.

This guide covers the relevant Nstance Server configuration for provisioning a Netsy cluster, including per-instance TLS certificate generation, environment configuration, and delivery of the shared cluster config file.

Each Netsy node runs on a VM managed by Nstance. The Nstance instance ID (e.g. `etc-01jxxxxxxxxxxxxxxxxxxx`) is used directly as the Netsy node ID. Nstance generates per-instance TLS certificates: the Nstance agent generates a key pair on the instance, sends the public key to the Nstance Server, which signs and returns a certificate. The private key never leaves the instance.

## Nstance Server Configuration

The sections below show the relevant parts of the Nstance Server config file. Replace `my-cluster`, subnet IDs, security group IDs, IAM ARNs, and bucket names with values for your environment. See the [Nstance server configuration reference](https://nstance.dev/docs/reference/server-config.md) for the full Nstance server config schema.

### Certificates

Netsy requires two certificates per node — a server certificate (for the Client API, Peer API, etc) and a peer client certificate (for outbound peer-to-peer connections). Both require a URI SAN encoding the cluster ID, role, and node ID: `netsy://{cluster_id}/peer/{node_id}`. See [TLS Certificates](tls.md) for the full requirements.

```jsonc
"certificates": {
  // Server certificate — used to serve the Client API, Peer API, etc
  "netsy.server": {
    "kind": "server",
    "cn": "{{ .Instance.ID }}",
    "uri": ["netsy://my-cluster/peer/{{ .Instance.ID }}"],
    "dns": [
      "localhost",
      "{{ .Instance.Hostname }}"
    ],
    "ip": [
      "127.0.0.1",
      "::1",
      "{{ .Instance.IP4 }}"
    ]
  },
  // Peer client certificate — used when connecting outbound to other Netsy nodes
  "netsy.peer": {
    "kind": "client",
    "cn": "{{ .Instance.ID }}",
    "uri": ["netsy://my-cluster/peer/{{ .Instance.ID }}"]
  }
}
```

If Netsy nodes advertise on IPv6 as well, add `"{{ .Instance.IP6 }}"` to the `netsy.server` `ip` list.

### Template

Define an instance template for Netsy nodes. The template delivers signed per-instance TLS certificates, a per-node environment file, and the shared cluster config to each instance on registration:

```jsonc
"templates": {
  "etc": {
    "kind": "etc",    // 3-letter prefix used in instance IDs (e.g. etc-01jxxxxxxxxxxxxxxxxxxx)
    "arch": "arm64",
    "files": {
      // Per-instance server certificate — signed by Nstance using the public key from the agent
      "server.crt": {
        "kind": "certificate",
        "template": "netsy.server",
        "key": {
          "source": "agent",
          "name": "netsy.server.pub"
        }
      },
      // Per-instance peer (client) certificate — signed by Nstance using the public key from the agent
      "peer.crt": {
        "kind": "certificate",
        "template": "netsy.peer",
        "key": {
          "source": "agent",
          "name": "netsy.peer.pub"
        }
      },
      // Per-node environment file — sets node identity, bind/advertise addresses, and cert paths
      "netsy.env": {
        "kind": "env",
        "template": {
          "NETSY_CONFIG": "/etc/netsy/config.jsonc",
          "NETSY_NODE_ID": "{{ .Instance.ID }}",
          "NETSY_BIND_CLIENT": ":2378",
          "NETSY_ADVERTISE_CLIENT": "{{ .Instance.IP4 }}:2378",
          "NETSY_BIND_PEER": ":2381",
          "NETSY_ADVERTISE_PEER": "{{ .Instance.IP4 }}:2381",
          "NETSY_BIND_ELECTION": ":8443",
          "NETSY_ADVERTISE_ELECTION": "{{ .Instance.IP4 }}:8443",
          "NETSY_BIND_HEALTH": ":8080",
          "NETSY_TLS_CA_CERT": "/etc/netsy/certs/ca.crt",
          "NETSY_TLS_SERVER_CERT": "/etc/netsy/certs/server.crt",
          "NETSY_TLS_SERVER_KEY": "/etc/netsy/certs/server.key",
          "NETSY_TLS_CLIENT_CERT": "/etc/netsy/certs/peer.crt",
          "NETSY_TLS_CLIENT_KEY": "/etc/netsy/certs/peer.key",
          "NETSY_DATA_DIR": "/var/lib/netsy"
        }
      },
      // Shared cluster config — same across all nodes, inlined directly in the template
      "config.jsonc": {
        "kind": "json",
        "template": {
          "cluster_id": "my-cluster",
          "storage": {
            "provider": "s3",
            "bucket_name": "my-netsy-bucket",
            "key_prefix": "netsy/",
            "class": "STANDARD",
            "encryption": "provider-managed"
          },
          "heartbeat_interval": "1s",
          "elector": {
            "degradation_count": 2,
            "deregistration_timeout": "3m",
            "primary_prior_timeout": "5s"
          },
          "replication": {
            "quorum": -1,
            "degradation_count": 2,
            "chunk_buffer": {
              "threshold_size_mb": 4,
              "threshold_age_minutes": 1
            }
          },
          "snapshot": {
            "threshold_records": 10000,
            "threshold_size_mb": 10000
          },
          "compaction_interval": "5m"
        }
      }
    },
    "args": {
      "ImageId": "{{ .Image.debian_13_arm64 }}",
      "SecurityGroupIds": ["sg-xxxxxxxxxxxxxxxxx"],
      "IamInstanceProfile": {
        "Arn": "arn:aws:iam::123456789012:instance-profile/netsy-node"
      }
    },
    "userdata": {
      // Example script content shown in Instance Setup below
      "source": "url",
      "content": "https://example.com/userdata/netsy.sh"
    },
    "size": 1,
    "instance_type": "t4g.small"
  }
}
```

### Group

Define a static group of three Netsy nodes:

```jsonc
"groups": {
  "netsy": {
    "template": "etc",
    "size": 3,
    "subnet_pool": "netsy"
  }
}
```

Netsy supports any cluster size, but an odd number of nodes (3, 5, …) is recommended when quorum replication is enabled.

## Instance Setup

Per the above example server configuration reference to `https://example.com/userdata/netsy.sh`, you should create and host a userdata script for configuring your Netsy nodes, and point your Nstance server configuration for those instances at the scripts URL.

Importantly, your userdata script should first setup Nstance - see the [templated Nstance userdata script](https://github.com/nstance-dev/nstance/blob/main/deploy/tf/common/shard/templates/agent-userdata.sh.tpl) as an example base script (note that this template is not raw bash and must be interpolated) — it handles installing the Nstance agent, writing the registration nonce and CA certificate, and starting the agent systemd service.

You will then need your script to install Netsy, move Netsy-related Nstance-delivered files into place, and start the Netsy systemd service, for example:

```bash
# Install Netsy
curl -fsSL "https://github.com/netsy-dev/netsy/releases/latest/download/netsy-linux-arm64" \
  -o /usr/bin/netsy
chmod +x /usr/bin/netsy

# Create netsy user and directories
useradd --system --no-create-home --shell /usr/sbin/nologin netsy
install -d -m 750 -o netsy -g netsy /etc/netsy/certs
install -d -m 750 -o netsy -g netsy /var/lib/netsy

# Move Nstance-delivered files into place
RECV="${NSTANCE_RECV_DIR:-/opt/nstance-agent/recv}"
KEYS="${NSTANCE_KEYS_DIR:-/opt/nstance-agent/keys}"
IDENTITY="${NSTANCE_IDENTITY_DIR:-/opt/nstance-agent/identity}"
install -m 640 -o netsy -g netsy "$RECV/server.crt"        /etc/netsy/certs/server.crt
install -m 600 -o netsy -g netsy "$KEYS/netsy.server.key"  /etc/netsy/certs/server.key
install -m 640 -o netsy -g netsy "$RECV/peer.crt"          /etc/netsy/certs/peer.crt
install -m 600 -o netsy -g netsy "$KEYS/netsy.peer.key"    /etc/netsy/certs/peer.key
install -m 640 -o netsy -g netsy "$RECV/config.jsonc"      /etc/netsy/config.jsonc
install -m 640 -o netsy -g netsy "$RECV/netsy.env"         /etc/netsy/env

# Copy CA certificate from Nstance identity directory
install -m 640 -o netsy -g netsy "$IDENTITY/ca.crt" /etc/netsy/certs/ca.crt

# Install systemd unit
cat > /etc/systemd/system/netsy.service <<'EOF'
[Unit]
Description=Netsy
Wants=network-online.target
After=network-online.target nstance-agent.service

[Service]
Type=exec
Restart=always
User=netsy
Group=netsy
EnvironmentFile=/etc/netsy/env
ExecStart=/usr/bin/netsy

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now netsy
```
