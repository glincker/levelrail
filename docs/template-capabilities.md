---
title: "Template capabilities"
description: "How a template asks for elevated access (Docker API, host network, devices, net_admin, kernel modules), which settings gate it, and how to grant it at install."
"og:url": "https://docs.levelrail.com/template-capabilities"
---

A template that needs more than a plain container declares it in its manifest. Levelrail shows the request before install, blocks it unless the matching setting is on, and records what you granted.

## Capability classes

| Class | What it gives | Notes |
| --- | --- | --- |
| `docker_api` | A filtered Docker socket for the app, never the host socket | Scope is `READ_ONLY` (inspect and logs of the app's own containers), `PROJECT` (only that app's containers) or `LOGS` (see below) |
| `host_network` | The app runs on the host network | It sees every host interface and port |
| `devices` | Maps `/dev/dri`, `/dev/ttyUSB*`, `/dev/ttyACM*` or `/dev/video*` | No other device is ever allowed |
| `net_admin` | The `NET_ADMIN` Linux capability | For VPN and routing apps |

A template can also list `requires_kernel_modules` (for example `wireguard`). Levelrail checks the host for each module before install and stops with a clear message when one is missing, instead of failing on first start.

Privileged containers are never allowed in a template.

## Settings

Each class is off by default and applies live. Turning one on needs `root`.

| Setting | Gates |
| --- | --- |
| `templates.cap_docker_api` | `docker_api` |
| `templates.cap_host_network` | `host_network` |
| `templates.cap_devices` | `devices` |
| `templates.capability_net_admin_enabled` | `net_admin` |

Turning a setting off cuts the grant for running apps at the next reconcile.

### The `LOGS` scope

`LOGS` is for log viewers such as Dozzle. It lists and reads the containers and log text of every app on the host. It is read only: no start, stop, exec, archive, attach, image, volume, network, secret or swarm access.

Inspect and list answers are filtered on the proxy: environment variables are removed, host paths and bind sources are reduced to names, and labels that look like secrets are dropped. The Levelrail control plane and containers that do not belong to an app are not visible. Containers carry no stack marker, so the scope is every app on the host, not one stack. Logs themselves are not filtered, and they can contain secrets, so grant `LOGS` only when everyone who can open the viewer may read every app's logs. Granting needs `root`.

## Declaring a capability

```yaml
capabilities:
  - class: docker_api
    service: agent
    reason: Reads container state to show a status board
    caps: [READ_ONLY]
requires_kernel_modules: [wireguard]
```

Every entry needs a `service` and a `reason`. The reason is shown to the person installing.

## Granting at install

Only `root` can grant an elevated class. From the CLI, confirm each class the template declares:

```bash
levelrail-cli templates install <id> --grant docker_api
levelrail-cli templates install <id> --grant host_network --grant devices
```

Without `--grant`, install stops and prints the flags you need. The dashboard shows the same request as a confirmation step.

## Architecture mismatch

A template with no verified build for the host's architecture refuses to install. Override it only when you know the image works:

```bash
levelrail-cli templates install <id> --allow-platform-mismatch
```

## Why did an install fail

```bash
levelrail-cli templates compat
```

Lists templates whose install failed on this host, the gap class (capability, Docker socket, architecture, port, kernel module, device) and what unlocks it. A row clears when the template installs. The record is local and never uploaded.

See also [Templates and registry](/templates-and-registry), [Arm64 compatibility](/arm-compatibility) and [Runtime settings](/runtime-settings).
