# Running Levelrail in Docker

Levelrail publishes two images to GitHub Container Registry, built for
`linux/amd64` and `linux/arm64` on every tagged release:

- `ghcr.io/glincker/levelrail`: the control plane.
- `ghcr.io/glincker/levelrail-agent`: the node agent.

`install.sh` (the one-liner in the root [README](../README.md)) remains
the default path for a real deployment, since it also sets up Docker
itself and a systemd unit. These images are for operators who already
run everything else as a container and want Levelrail to fit the same
pattern.

## Control plane

```
docker run -d \
  --name levelrail \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v levelrail-data:/var/lib/levelrail-data \
  -e APP_DATA_DIR=/var/lib/levelrail-data \
  -e APP_ADMIN_USERNAME=admin \
  -e APP_ADMIN_PASSWORD=change-me \
  ghcr.io/glincker/levelrail:latest
```

**Security note:** mounting `/var/run/docker.sock` gives this container
the ability to control every other container on the host, including
starting privileged ones. This is not a new risk specific to Docker: it's
the same trust level `install.sh`'s systemd-based install already runs
the control plane binary with, since that's exactly what letting
Levelrail manage containers on your behalf requires (see
[architecture.md](architecture.md)). Only run this image on a host you
already trust with that level of access.

### docker-compose.yml

```yaml
services:
  levelrail:
    image: ghcr.io/glincker/levelrail:latest
    restart: unless-stopped
    ports:
      - "8080:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - levelrail-data:/var/lib/levelrail-data
    environment:
      APP_DATA_DIR: /var/lib/levelrail-data
      APP_ADMIN_USERNAME: admin
      APP_ADMIN_PASSWORD: change-me

volumes:
  levelrail-data:
```

## Node agent

The agent dials out to the control plane; it never accepts an inbound
connection (see [architecture.md](architecture.md)). It needs its own
access to the local Docker socket, and a one-time join token to enroll
on first run.

```
docker run -d \
  --name levelrail-agent \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v levelrail-agent-identity:/var/lib/levelrail-agent \
  -e APP_CONTROL_PLANE_ADDR=control-plane-host:9443 \
  -e APP_JOIN_TOKEN=your-one-time-join-token \
  -e APP_AGENT_IDENTITY_FILE=/var/lib/levelrail-agent/identity.json \
  ghcr.io/glincker/levelrail-agent:latest
```

`APP_JOIN_TOKEN` is only needed for the first run: once enrolled, the
agent persists its mTLS identity to `APP_AGENT_IDENTITY_FILE` and reads
it back on every restart, so mount that path on a named volume or it
will have to re-enroll every time the container restarts.
`APP_NODE_NAME` is optional and defaults to the container's hostname,
which Docker randomizes per container unless you set `--hostname`
explicitly, so setting `APP_NODE_NAME` yourself is worth doing for a
name you can actually recognize in the dashboard.
