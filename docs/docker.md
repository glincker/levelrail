---
description: Run Levelrail control plane and node agent as Docker containers.
---

# Running Levelrail in Docker

Levelrail publishes two images to GitHub Container Registry for every tagged release, built for `linux/amd64` and `linux/arm64`:

- `ghcr.io/glincker/levelrail`: the control plane
- `ghcr.io/glincker/levelrail-agent`: the node agent

The `install.sh` one-liner (in the root [README](../README.md)) remains the default for real deployments, as it also sets up Docker and a systemd unit. Use these images if you already run everything as containers and want Levelrail to follow the same pattern.

## Control plane

Both images run as a non-root user (distroless's `nonroot`, uid/gid 65532). The `--group-add $(stat -c '%g' /var/run/docker.sock)` flag adds that user to the socket's host-side group at runtime, since the GID varies per system and cannot be baked into the image.

::: code-group
```bash [docker run]
docker run -d \
  --name levelrail \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  --group-add $(stat -c '%g' /var/run/docker.sock) \
  -v levelrail-data:/var/lib/levelrail-data \
  -e APP_DATA_DIR=/var/lib/levelrail-data \
  -e APP_ADMIN_USERNAME=admin \
  -e APP_ADMIN_PASSWORD=change-me \
  ghcr.io/glincker/levelrail:latest
```

```yaml [docker-compose.yml]
services:
  levelrail:
    image: ghcr.io/glincker/levelrail:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - levelrail-data:/var/lib/levelrail-data
    group_add:
      - "${DOCKER_GID:-999}" # match your host's docker group gid: getent group docker
    environment:
      APP_DATA_DIR: /var/lib/levelrail-data
      APP_ADMIN_USERNAME: admin
      APP_ADMIN_PASSWORD: change-me

volumes:
  levelrail-data:
```
:::

::: warning
Granting access to `/var/run/docker.sock` allows this container to control every other container on the host, including starting privileged ones. This is not a new risk specific to Docker: it's the same trust level that `install.sh`'s systemd install already uses, because that's what's required to manage containers on your behalf (see [architecture.md](architecture.md)). Only run this image on a host you already trust with that level of access.
:::

## Node agent

The agent dials out to the control plane (it does not accept inbound connections, see [architecture.md](architecture.md)). You need to provide Docker socket access and a one-time join token for enrollment on first run.

```bash
docker run -d \
  --name levelrail-agent \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v levelrail-agent-identity:/var/lib/levelrail-agent \
  -e APP_CONTROL_PLANE_ADDR=control-plane-host:9443 \
  -e APP_JOIN_TOKEN=your-one-time-join-token \
  -e APP_AGENT_IDENTITY_FILE=/var/lib/levelrail-agent/identity.json \
  ghcr.io/glincker/levelrail-agent:latest
```

**`APP_JOIN_TOKEN`** is only needed for first enrollment. After that, the agent saves its mTLS identity to `APP_AGENT_IDENTITY_FILE`. Mount that path on a named volume, or the agent will have to re-enroll on every container restart.

**`APP_NODE_NAME`** is optional and defaults to the container's hostname (which Docker randomizes). Set it explicitly for a recognizable name in the dashboard.

## See also

- [Installing](installing.md) for `install.sh` and other installation methods
- [Multi-node](multi-node.md) for enrolling agents and managing additional nodes
- [Architecture](architecture.md) for how the control plane and agents communicate
- [Getting started](getting-started.md) for building and running locally
