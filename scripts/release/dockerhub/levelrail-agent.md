# Levelrail node agent

Levelrail is a self-hosted PaaS: a deployment platform you run on your own servers. Push to deploy with automatic HTTPS, logs, metrics and one-step rollback. It ships as one Go binary and needs no Kubernetes. Apache 2.0.

This image is the Levelrail node agent. It runs deployments on an additional server that a Levelrail control plane manages, so you can deploy Docker Compose apps and git push deploys across several machines.

## Quick start

Install the control plane first:

```bash
curl -fsSL https://github.com/glincker/levelrail/releases/latest/download/install.sh | sudo sh
```

Then add a node: https://docs.levelrail.com/multi-node

## Pull

```bash
docker pull levelrail/levelrail-agent:latest
docker pull ghcr.io/glincker/levelrail-agent:latest
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

Keep the agent on the same version as the control plane.

## Links

- Release notes for this tag: https://github.com/glincker/levelrail/releases/tag/@TAG@
- Documentation: https://docs.levelrail.com
- Source and issues: https://github.com/glincker/levelrail
- Website: https://levelrail.com
- Control plane image: https://hub.docker.com/r/levelrail/levelrail
