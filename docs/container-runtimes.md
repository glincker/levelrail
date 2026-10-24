---
title: "Container runtimes"
icon: "/assets/icons/download-simple.svg"
sidebarTitle: "Container runtimes"
description: Which container runtimes Levelrail supports on a Linux server, how install.sh picks one, and the manual steps for Podman.
---

Levelrail talks to a Docker-compatible Engine API socket. Any runtime that serves that API can in principle run it, but only some are tested. This page says which, and how to set up the rest.

## What is supported today

| Runtime | Status | Notes |
| --- | --- | --- |
| Docker Engine, rootful | Supported | The default and the only fully tested path. |
| Docker Engine, rootless | Best-effort | Ports below 1024 need `net.ipv4.ip_unprivileged_port_start=80`. Bind-mount ownership and cgroup limits can differ. |
| Podman, rootful | Best-effort | Through its Docker-compatible socket. Tested against a fake client only, not a real Podman host. |
| Podman, rootless | Best-effort | Per-user socket, no ports below 1024 by default. Set up by hand, the installer does not do it for you. |
| containerd, CRI-O, nerdctl | Not supported | Out of scope for now. |

Image-based deploys and compose are the expected path on Podman. Dockerfile and Railpack source builds are not verified on Podman yet: builds use a BuildKit that runs inside `dockerd`, and Podman has no embedded BuildKit. Build on another node or in CI and deploy the image.

Server installs are Linux only. On a macOS laptop, `levelrail-cli dev` is the path; see [Local development](/local-development).

## How the installer chooses

`install.sh --runtime auto|docker|podman|existing` (or `LEVELRAIL_RUNTIME`):

| Value | Behavior |
| --- | --- |
| `auto` (default) | Use a working Docker engine if `docker info` succeeds. Otherwise use Podman if it is installed and works. Otherwise install Docker via `get.docker.com`, as before. |
| `docker` | Use Docker, and install it if it is missing. |
| `podman` | Require an existing Podman. The installer never installs Podman: if it is missing it exits with the distro command (`apt install podman` or `dnf install podman`). |
| `existing` | Use Docker or Podman already on the host and install nothing. Fails if neither works. |

For Podman the installer enables `podman.socket`, writes `APP_CONTAINER_RUNTIME_SOCKET=unix:///run/podman/podman.sock` into the same systemd drop-in as the other `APP_*` settings, and makes the `levelrail` unit require `podman.socket` instead of `docker.service`. Set `LEVELRAIL_PODMAN_SOCKET` to use another socket path. An upgrade keeps the runtime an install chose.

`install.sh --check` prints the same rows: the detected runtime, its version, rootful or rootless, and the support label. It exits non-zero only on a real failure, such as `--runtime podman` with no Podman installed.

## Manual Podman setup

Install Podman with your distro first (`apt install podman`, `dnf install podman`).

Rootful, as root:

```bash
sudo systemctl enable --now podman.socket
ls -l /run/podman/podman.sock
```

Then run `install.sh --runtime podman`, or set `APP_CONTAINER_RUNTIME_SOCKET=unix:///run/podman/podman.sock` in the `levelrail` service drop-in yourself.

Rootless, as the user that will own the containers (the installer only prints these, it never runs user-level commands for another user):

```bash
systemctl --user enable --now podman.socket
sudo loginctl enable-linger "$USER"
echo "unix://$XDG_RUNTIME_DIR/podman/podman.sock"
```

Linger keeps the user's services running after logout. Point `APP_CONTAINER_RUNTIME_SOCKET` at that socket path. Rootless Podman cannot publish ports 80 and 443 unless you lower `net.ipv4.ip_unprivileged_port_start`.

## Verify

```bash
levelrail-cli doctor
```

The `container_runtime` check reports Docker or Podman, rootful or rootless, and which setting chose the socket. See [Security](/security#rootless-and-podman) for what that detection covers.

## Roadmap

- Test against a real Podman host in CI, then promote it from best-effort.
- A source-build story for Podman (an external BuildKit), once it is verified.
- Other runtimes only on demand.
