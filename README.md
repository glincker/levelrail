# Levelrail

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

Levelrail is a self-hosted deployment platform. Point it at one or more
Linux boxes and it turns them into a private cloud: push to a git repo,
get a running app with TLS, logs, metrics, and rollback.

## Status

Early, active development. The control plane and node agent run
single-node today; multi-node support (agent enrollment, WireGuard mesh,
node placement) is in progress. There is no stable release yet and the
project is not ready for production workloads. APIs, the app spec format,
and the on-disk data layout can all still change without notice.

## Why not Coolify or Dokploy

Most self-hosted platforms in this category drive remote servers by
SSHing in and shelling out to the `docker` CLI, then parsing its text
output. That's a common source of flakiness and it forces polling loops
to detect state changes. Levelrail takes a different approach:

- **Agent-based control plane.** A small agent runs on each node, talks
  to the local Docker Engine API directly (no shelling out to the
  `docker` CLI), and streams container events up to the control plane.
  The control plane never polls.
- **Observability built in, not bolted on.** Metrics and log storage are
  first-class parts of the core, not something you're told to install
  separately.
- **Low idle footprint.** A single static Go binary for the control
  plane, a single static Go binary for the agent, no separate database
  server, no message queue, no extra containers just to run the
  platform itself.

Levelrail is not a Kubernetes competitor. It targets teams running
somewhere between 3 and 50 services across 1 to 10 machines who want a
private cloud without taking on Kubernetes's operational surface.

## Architecture at a glance

- **Control plane** (`cmd/levelrail`): a single Go binary. Reconciles
  declarative resource records against observed Docker state, the same
  pattern Kubernetes controllers use, without the rest of the Kubernetes
  runtime.
- **Node agent** (`cmd/levelrail-agent`): dials out to the control plane
  (no inbound ports on managed servers) and talks to the local Docker
  socket via the Engine API.
- **Ingress**: [Caddy](https://caddyserver.com/) embedded as a Go
  library (`internal/ingress`), driven in-process, for automatic TLS and
  domain routing.
- **Builds**: [BuildKit](https://github.com/moby/buildkit) as a Go
  library (`internal/build`), not `docker build`, for remote cache and
  parallel stage execution.
- **State**: embedded SQLite in WAL mode (`internal/store`), pure Go via
  `modernc.org/sqlite`, so cross-compiling the binary stays simple.
- **API**: an HTTP API under `internal/api`, versioned at `/api/v1`.
- **Frontend** (`web/`): React, Vite, TypeScript, Tailwind, TanStack
  Router and Query. Built as static assets and embedded into the control
  plane binary via `embed.FS`, so there's no separate Node process to
  run in production.

Everything ships as two binaries: `levelrail` (control plane, with the
frontend embedded) and `levelrail-agent` (node agent). In single-node
mode the agent's transport runs in-process instead of over the network,
so the code path is the same whether you're running one node or ten.

## Building and running locally

Requires Go 1.25+ and Docker.

```
# control plane
go build ./cmd/levelrail

# node agent
go build ./cmd/levelrail-agent
```

The frontend lives in `web/` and is a separate Vite project:

```
cd web
npm install
npm run dev       # Vite dev server
npm run build      # production build, embedded into the control plane binary
```

See `web/README.md` for frontend-specific commands and conventions.

## How it compares

Positioning, not a ranking. All of these are worth using; the differences
below are the ones that matter for choosing between them.

| Project | Node control | Orchestration | Observability | Ingress |
| --- | --- | --- | --- | --- |
| **Levelrail** | Agent (gRPC, dials out) | Custom reconciler over Docker Engine API | Node-local metrics and logs, federated query | Caddy, embedded |
| Coolify | SSH + Docker CLI | Docker Compose / Swarm | Bolt-on (install Grafana yourself) | Traefik |
| Dokploy | SSH + Docker CLI | Docker Swarm | Bolt-on | Traefik |
| CapRover | SSH + Docker CLI | Docker Swarm | Bolt-on | nginx |
| Dokku | Local Docker CLI, single node | Docker (no orchestration) | Bolt-on | nginx |
| Kamal | SSH + Docker CLI | None (deploy script, not a control plane) | None built in | Configurable |

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for how to propose changes,
commit conventions, and how to run tests and the linter locally.

## License

Apache 2.0, see [LICENSE](LICENSE).
