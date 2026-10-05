---
description: Run Levelrail control plane and node agent as Docker containers.
---

# Running Levelrail in Docker

Levelrail publishes two images to GitHub Container Registry, built for `linux/amd64` and `linux/arm64`, signed with cosign:

- `ghcr.io/glincker/levelrail`: the control plane
- `ghcr.io/glincker/levelrail-agent`: the node agent

See [Installing: image tags](installing.md#option-2-docker) for the `:latest`/`:beta`/`:edge` channels and how to verify a signature.

The `install.sh` one-liner (in the root [README](../README.md)) remains the default for real deployments, as it also sets up Docker and a systemd unit. Use these images if you already run everything as containers and want Levelrail to follow the same pattern.

## Control plane

The control plane image runs as a non-root user (distroless's `nonroot`, uid/gid 65532); the [agent image](#node-agent) runs as root. The `--group-add $(stat -c '%g' /var/run/docker.sock)` flag adds that user to the socket's host-side group at runtime, since the GID varies per system and cannot be baked into the image.

```bash
docker run -d \
  --name levelrail \
  -p 80:80 -p 443:443 -p 127.0.0.1:8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  --group-add $(stat -c '%g' /var/run/docker.sock) \
  -v levelrail-data:/var/lib/levelrail-data \
  -e APP_DATA_DIR=/var/lib/levelrail-data \
  -e APP_ADMIN_USERNAME=admin \
  -e APP_ADMIN_PASSWORD=change-me \
  ghcr.io/glincker/levelrail:beta
```

A [`docker-compose.yml`](../docker-compose.yml) is committed at the repo root and does the same thing. Before running it, find your host's docker group GID and export it, since the container's nonroot user needs to be added to that group to reach the socket:

```bash
export DOCKER_GID=$(getent group docker | cut -d: -f3)
docker compose up -d
```

Port 8080 is the plain-HTTP dashboard and API, so both the `docker run` example and the compose file bind it to `127.0.0.1` only. For the first sign-in, either tunnel to it (`ssh -L 8080:127.0.0.1:8080 user@host`, then open `http://127.0.0.1:8080`) or preset the admin with `APP_ADMIN_USERNAME`/`APP_ADMIN_PASSWORD`. Print the setup token with `docker compose exec levelrail levelrail setup-token`. Once a domain and `https://` dashboard URL are configured, the dashboard is served over 80/443 by the embedded Caddy. To publish 8080 on every interface anyway (for example on a private network), set `LEVELRAIL_HTTP_BIND=0.0.0.0` in the environment or `.env` file; for `docker run`, drop the `127.0.0.1:` prefix.

If `DOCKER_GID` is unset, the compose file falls back to `999`, the common default on Debian/Ubuntu, but always check with `getent group docker` first since it varies per system.

The image ships a Docker `HEALTHCHECK` (`levelrail healthcheck --ready`, a GET against its own [`/readyz`](/troubleshooting#readiness-readyz)), so `docker ps` and `docker compose ps` show a real health status without extra compose config. Distroless has no shell or `curl`/`wget`, which is why this is a dedicated subcommand on the binary itself rather than a shell one-liner.

::: warning
Granting access to `/var/run/docker.sock` allows this container to control every other container on the host, including starting privileged ones. This is not a new risk specific to Docker: it's the same trust level that `install.sh`'s systemd install already uses, because that's what's required to manage containers on your behalf (see [architecture.md](architecture.md)). Only run this image on a host you already trust with that level of access.
:::

## Node agent

The agent dials out to the control plane (it does not accept inbound connections, see [architecture.md](architecture.md)). You need to provide Docker socket access and a one-time join token for enrollment on first run.

Unlike the control plane image, the agent image runs as **root**. It needs `/var/run/docker.sock` (root-equivalent on the host regardless of the user inside the container) and, for the WireGuard mesh, `--cap-add NET_ADMIN --device /dev/net/tun -e APP_MESH_ENABLED=1`. Running as root also means a named volume or a Docker-created host directory for the identity file is writable with no `chown`. The agent additionally refuses to send its join token until it has proven it can write the identity file, so a permissions mistake never burns the token.

```bash
docker run -d \
  --name levelrail-agent \
  --network host \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v levelrail-agent-identity:/var/lib/levelrail-agent \
  -e APP_CONTROL_PLANE_ADDR=control-plane-host:9443 \
  -e APP_JOIN_TOKEN=your-one-time-join-token \
  -e APP_CA_FINGERPRINT=control-plane-ca-fingerprint \
  -e APP_AGENT_IDENTITY_FILE=/var/lib/levelrail-agent/identity.json \
  ghcr.io/glincker/levelrail-agent:beta
```

**`APP_JOIN_TOKEN`** and **`APP_CA_FINGERPRINT`** are only needed for first enrollment (the fingerprint is printed next to the token, see [Multi-node](multi-node.md)). After that, the agent saves its mTLS identity to `APP_AGENT_IDENTITY_FILE`. Mount that path on a named volume, or the agent will have to re-enroll on every container restart.

**`APP_NODE_NAME`** is optional and defaults to the container's hostname (which Docker randomizes). Set it explicitly for a recognizable name in the dashboard.

### Without a container

Releases after `v0.2.0-beta.15` attach the agent as a plain binary (`levelrail-agent-linux-amd64`, `levelrail-agent-linux-arm64`) and as `.deb` and `.rpm` packages, all listed in the signed `checksums.txt`. On a host that already has Docker, install a package, fill in the three enrollment values, and start the service:

```bash
sudo dpkg -i levelrail-agent-linux-amd64.deb     # or: sudo rpm -i levelrail-agent-linux-amd64.rpm
sudoedit /etc/levelrail-agent/agent.env           # APP_CONTROL_PLANE_ADDR, APP_JOIN_TOKEN, APP_CA_FINGERPRINT
sudo systemctl enable --now levelrail-agent
```

The service keeps its enrolled identity in `/var/lib/levelrail-agent`, and upgrading a package keeps your edits to `agent.env`. The plain binary reads the same environment variables as the image, so it also runs under any other process manager.

## See also

- [Installing](installing.md) for `install.sh` and other installation methods
- [Multi-node](multi-node.md) for enrolling agents and managing additional nodes
- [Architecture](architecture.md) for how the control plane and agents communicate
- [Getting started](getting-started.md) for building and running locally
