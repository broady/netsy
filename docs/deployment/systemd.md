---
title: "Systemd"
weight: 10
description: "Example systemd unit file"
---

To run `netsy` as a systemd service:

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

You can look at the [.env](../../.env) file for configuration examples.
