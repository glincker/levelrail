# Getting started

## Requirements

- Go 1.26+
- Docker (the control plane and agent talk to the Docker Engine API
  directly, they never shell out to the `docker` CLI, but a running
  Docker daemon is required)
- Node.js and npm, only if you're building the frontend from source.
  The repo doesn't pin an exact Node version; a recent LTS release
  works.

## Build and run

The control plane and the node agent are separate Go binaries:

```
# control plane
go build ./cmd/levelrail

# node agent
go build ./cmd/levelrail-agent
```

There's also a CLI, a thin scriptable HTTP client for the control
plane's API:

```
go build ./cmd/levelrail-cli
```

The frontend lives in `web/` and is a separate Vite project. It's
embedded into the control plane binary via `embed.FS` at build time,
so production deployments don't run a separate Node process:

```
cd web
npm install
npm run dev       # Vite dev server
npm run build      # type-check and produce a production build in dist/
```

See `web/README.md` for the rest of the frontend commands (lint,
format, typecheck, preview) and conventions.

Run the control plane binary and it listens on `:8080` by default.

## Deploy your first app

The app spec is the one declarative file you write in your app's
repo: `app.yaml` (also discovered as `app.yml`, `deploy.yaml`, or
`deploy.yml`). A minimal one looks like this:

```yaml
version: 1
services:
  web:
    build:
      type: dockerfile
    port: 8080
```

`test/fixtures/hello-e2e/Dockerfile` in this repo is a real, working
example of what such a service can build: a tiny busybox image that
listens on port 8080 and serves a static response. It's the fixture
the end-to-end deploy test builds, deploys, and checks an HTTPS
response against, so pairing it with the `app.yaml` above and
building from that directory is a genuine, exercised path, not a
hypothetical one. The full spec, with domains, health checks,
resource limits, env, replicas, and deploy strategy, is documented in
[docs/app-spec-reference.md](app-spec-reference.md).

Before you can deploy anything, the control plane needs an admin
account: either register through the frontend on first run, or set
`APP_ADMIN_USERNAME` and `APP_ADMIN_PASSWORD` before starting it. For
a quick local trial there's a third option: start the control plane
with `APP_DEV_MODE=1` and it bootstraps a fixed `dev`/`dev` admin
account plus a set of fixed API tokens from `dev-fixtures.yml` at the
repo root, so you can skip the register-then-mint-a-token steps
entirely. This mode is for local development only and must never be
used in production; a release build (`-tags embedweb`) ignores
`APP_DEV_MODE` outright.

With the control plane running in dev mode and `levelrail-cli` built
(see above), create and deploy an app in one command:

```
APP_API_TOKEN=dev-root-token ./levelrail-cli apps create \
  --name your-app \
  --file app.yaml \
  --repo https://github.com/your-org/your-app \
  --image-repo registry.example.com/your-org/your-app
```

This creates the app and triggers a real BuildKit build from the git
repository you point it at. Check on it with:

```
APP_API_TOKEN=dev-root-token ./levelrail-cli apps status your-app
```

If you already have a built image and don't need Levelrail to build
it for you, skip `--file`/`--repo`/`--image-repo` and use the
existing-image path instead:

```
APP_API_TOKEN=dev-root-token ./levelrail-cli apps create \
  --name your-app --image registry.example.com/your-org/your-app:latest --port 8080
```

Run `levelrail-cli apps create -h` for the full set of flags, and
`levelrail-cli -h` for the rest of the commands (`apps deploy`,
`apps rollback`, `apps restart`, `apps logs`, `databases create`, and
so on).

Note: today, every app is exactly one container. Multi-service apps
(a web process plus a worker sidecar under one `app.yaml`) aren't
supported end to end yet, even though the spec's `services` field is
a map.

## Where to go next

- [docs/architecture.md](architecture.md): how the control plane, the
  node agent, and the reconciler fit together.
- [docs/app-spec-reference.md](app-spec-reference.md): the full
  `app.yaml` schema.
- [docs/comparison.md](comparison.md): how Levelrail's approach
  differs from Coolify, Dokploy, CapRover, Dokku, and Kamal.
- [docs/roadmap.md](roadmap.md): what's shipped, what's in progress,
  and what's not started yet.
