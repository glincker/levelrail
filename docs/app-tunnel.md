---
title: "Private app tunnel"
description: "Reach an admin port of an app from your laptop without publishing it. The CLI listens on 127.0.0.1 and relays each connection through the control plane."
"og:url": "https://docs.levelrail.com/app-tunnel"
---

Reach an admin port of an app from your laptop without publishing it. The CLI listens on `127.0.0.1` and relays each connection through the control plane to the container's port on the platform network.

```
levelrail-cli apps tunnel web --port 9000
levelrail-cli apps tunnel shop --service db --port 5432 --local-port 15432
```

The command prints the local address and runs until Ctrl-C. Open that address in a browser, psql or any TCP client.

## Flags

| Flag | Meaning |
| --- | --- |
| `--port` | Container port to reach. Defaults to the app's routed port. |
| `--service` | A service of a multi-service app, when the name is the app group. |
| `--local-port` | Local port to listen on. Defaults to a free port. |

## What is allowed

- Only a port the service declares: its routed port, a published container port, or a port the image EXPOSEs. Anything else is refused with the allowed list.
- Only that app's own container. Another app's service is never reachable, and a service name cannot be used to reach a sibling.
- Only apps on the control plane node, like the database tunnel.
- The same permission as exec and the terminal: operator (root ability) on the app. Viewers are refused.

## Limits and audit

Each open and close is written to the audit log (`app.tunnel.open`, `app.tunnel.close`) with the service, port and bytes in each direction.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_TUNNEL_ENABLED` | `true` | Set to `false` to refuse every tunnel. |
| `APP_TUNNEL_IDLE_TIMEOUT` | `15m` | Close a tunnel with no traffic for this long. |
| `APP_TUNNEL_MAX_PER_USER` | `5` | Open tunnels per account. |

The dashboard shows the exact command, with a Copy button, on the app Overview tab.
