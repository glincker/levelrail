# Levelrail

Levelrail is a self-hosted PaaS: a deployment platform you run on your own servers. Push to deploy with automatic HTTPS, logs, metrics and one-step rollback. It ships as one Go binary and needs no Kubernetes. Apache 2.0.

This image is the Levelrail control plane. Use it to deploy Docker Compose apps and git push deploys from your own infrastructure.

## Quick start

For a single server, the installer sets up Docker and a systemd unit:

```bash
curl -fsSL https://levelrail.com/install.sh | sudo sh
```

To run the control plane as a container instead, see the Docker guide: https://docs.levelrail.com/docker

## Pull

```bash
docker pull levelrail/levelrail:latest
docker pull ghcr.io/glincker/levelrail:latest
```

Both registries carry the same multi-arch image (`linux/amd64`, `linux/arm64`) with the same digest, signed with cosign.

## Tags

| Tag | Meaning |
| --- | --- |
| `latest` | newest stable release |
| `X.Y.Z`, `vX.Y.Z` | an exact stable release, never moves |
| `X.Y`, `vX.Y` | newest patch of a minor line |
| `X`, `vX` | newest release of a major line |
| `vX.Y.Z-rc.1` | an exact prerelease |
| `beta` | newest prerelease |
| `edge` | unreleased build from the main branch, for testing only |

Pin an exact `X.Y.Z` tag or a digest for anything that must not move.

## Links

- Release notes for this tag: https://github.com/glincker/levelrail/releases/tag/@TAG@
- Documentation: https://docs.levelrail.com
- Source and issues: https://github.com/glincker/levelrail
- Website: https://levelrail.com
- Node agent image: https://hub.docker.com/r/levelrail/levelrail-agent
