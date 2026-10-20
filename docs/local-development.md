---
title: "Run it on your laptop"
icon: "/assets/icons/download-simple.svg"
sidebarTitle: "Run on your laptop"
description: Start a private Levelrail instance on macOS or Linux with one command, deploy apps to name.localhost addresses, and reset it whenever you like.
"og:url": "https://docs.levelrail.com/local-development"
---

`levelrail-cli dev` runs a complete control plane on your own machine, bound to `127.0.0.1`, with its own data directory. Nothing on it is reachable from the network, and it needs no domain, no DNS and no setup token.

## Start it

You need Docker running (Docker Desktop on macOS, or the Docker daemon on Linux) and the `levelrail` control plane binary next to `levelrail-cli` or on your `PATH`.

```bash
levelrail-cli dev
```

The command checks that Docker answers, picks free ports, starts the control plane, waits until it is healthy, prints the dashboard address and a generated admin sign-in, and opens the dashboard in your browser. The password is printed once and kept in `local-admin.json` inside the data directory (mode `0600`). Press Ctrl-C to stop; the control plane shuts down cleanly.

| Flag | What it does |
| --- | --- |
| `--port N` | Dashboard port. Default is 8080, or the next free port. |
| `--data-dir DIR` | Where the instance keeps its data. Default is a per-user directory under `~/.local/share`, or `APP_DEV_DATA_DIR`. |
| `--no-open` | Do not open a browser. |
| `--reset` | Delete the instance's data first and start fresh. |
| `--bin PATH` | Control plane binary to run, or `APP_DEV_CONTROL_PLANE_BIN`. |

From another terminal:

```bash
levelrail-cli dev status    # running or not, dashboard address, data directory
levelrail-cli dev stop      # same as Ctrl-C in the first terminal
```

`dev` never reads `APP_DATA_DIR`, so a server data directory in your environment cannot be wiped by mistake, and a reset refuses any non-empty directory it did not create.

## Where your apps live

Every app without a custom domain gets `<app>.localhost`. Browsers and most operating systems resolve `*.localhost` to your own machine with no DNS, so the address works the moment the app is deployed. The internet-facing `sslip.io` fallback is turned off in this mode.

The ingress listens on port 80 when it is free and falls back to 8081 (and 8443 for the unused TLS listener) when it is not, which is always the case for a non-root user on Linux. The exact address is shown in the dashboard, in `levelrail-cli apps create` output and in `dev status`, for example `http://hello.localhost:8081`.

Local apps are served over plain HTTP, because there is nothing to protect on a loopback interface and a browser warning for a private certificate authority would only get in the way. Nothing needs to be trusted.

```bash
levelrail-cli login --api-url http://localhost:8080
levelrail-cli apps create --name hello --image traefik/whoami:latest --port 80
curl http://hello.localhost:8081/
```

## macOS and Docker Desktop

The control plane finds the Docker socket on its own, in this order: `APP_CONTAINER_RUNTIME_SOCKET`, `DOCKER_HOST`, `~/.docker/run/docker.sock` (Docker Desktop), `~/.docker/desktop/docker.sock`, Colima and Rancher Desktop sockets, the rootless Linux socket, then `/var/run/docker.sock`. If none accepts a connection, `dev` stops with the fix for your platform.

Docker Desktop runs containers in a virtual machine, so a container's own address is not reachable from the host. Levelrail does not depend on it: the ingress dials each app through the port Docker publishes on `127.0.0.1`, which works the same on Docker Desktop and on Linux.

## Reset

The dashboard shows a banner on every page of a local instance with the data directory and a **Reset** button. Reset deletes the instance's data and relaunches it with a new admin password, printed in the terminal that runs `dev`. Reset needs that terminal: an instance started some other way shows the banner without the button.

Stopping or resetting does not remove containers that apps already started. Remove them with `docker rm -f` when you are done.

## What it is not

Local mode is for developing and trying things out. It is not a way to host anything for other people: it refuses to bind a non-loopback address, serves plain HTTP, and keeps its admin password in a file in your home directory. For a real server, see [Installing](/installing).

<Note>
Checked on macOS (Apple silicon) with Docker Desktop. The same code path runs on Linux, where unprivileged users always get the 8081 and 8443 ingress ports.
</Note>
