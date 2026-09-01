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

### Guided setup

Don't want to hand-write `app.yaml` or look up every flag first? Run
the wizard instead:

```
./levelrail-cli apps create --interactive
```

It asks for the app name, its source (a git repository URL or an
existing Docker image reference), the container port, an optional
domain, an optional health check path (defaults to `/healthz`), and
optional memory/CPU limits, then asks whether to write the result to
`app.yaml` in the current directory or create the app directly against
the control plane API. `-i` is the short form of `--interactive`, and
it cannot be combined with `--name`/`--image`/`--repo`/`--file`, since
the wizard prompts for those itself.

Note: this wizard only creates single-service apps; it always writes
exactly one entry under `services:`. Multi-service apps (a web process
plus a worker sidecar under one `app.yaml`) do work end to end, just
not through this wizard: hand-write multiple entries in `app.yaml`'s
`services:` map, or use `POST /api/v1/apps/{name}/deploy-spec` (and
its dashboard equivalent, an app's Services tab) to fan a `services:`
map out into independent per-service builds and deploys. See
[docs/roadmap.md](roadmap.md) for the current caveats on that path
(the wizard gap here, and end-to-end test coverage still being
single-service-only).

`databases create` has the same guided mode:

```
./levelrail-cli databases create --interactive
```

It asks for the database name, its engine (from the live registry at
`GET /api/v1/database-engines`, so a newly added engine shows up here
automatically), a version (defaulting to the engine's own suggested
version), optional memory/CPU limits, whether to expose it outside the
Docker network, and an optional backup schedule. Unlike the apps wizard,
this one always creates the database against the control plane API:
`app.yaml`'s `databases:` block has no field for resource limits or
public access, and nothing in the deploy pipeline reads it to provision
a real database today, so there is no file-output mode to fall back to.
`-i` is the short form of `--interactive` here too, and it cannot be
combined with `--name`/`--engine`/`--version`.

### Output formats and filtering

Every command accepts `--output json|table|text` and `--query EXPR`
alongside the existing `--token`/`--api-url`/`--profile`/`--json` flags.
`table` is the default, human-readable form; `text` is a plain
tab-separated form with no headers or alignment, meant for further
piping through `awk`/`cut`; `--json` is kept as shorthand for `--output
json`, so existing scripts built against it keep working unchanged.
`--query` takes a [JMESPath](https://jmespath.org) expression and
filters or projects the result before it's printed, in any of the
three formats, the same feature AWS CLI users know:

```
# every app's name, whatever node it's running on
./levelrail-cli apps list --query "[].name"

# just the apps on a specific node
./levelrail-cli apps list --query "[?node_id=='node-1'].name"

# a single field off a single app, with no JSON wrapper at all
./levelrail-cli apps get your-app --query image --output text
```

`apps list`'s own response is a bare JSON array (not wrapped in an
`"apps"` key), so the expression indexes straight into it rather than
starting with `apps[...]`. Run `levelrail-cli <command> -h` for a given
command's own flags: `--output`/`--query` show up there too, since
they're wired into the same shared flag-parsing helper every command
already builds its flag set from.

### Shell completion

`levelrail-cli` can generate a completion script for bash, zsh, or fish,
covering every command and subcommand (`apps organizations env-set`,
`channels deliveries`, and so on) plus the global `--token`/`--api-url`/
`--json`/`--output`/`--query`/`--help` flags. It does not complete flag
values or positional arguments like app names.

```
# bash
source <(levelrail-cli completion bash)
# or, to install permanently:
levelrail-cli completion bash | sudo tee /etc/bash_completion.d/levelrail-cli > /dev/null

# zsh
source <(levelrail-cli completion zsh)
# or, to install permanently, save it as a file named "_levelrail-cli"
# somewhere on $fpath:
levelrail-cli completion zsh > "${fpath[1]}/_levelrail-cli"

# fish
levelrail-cli completion fish | source
# or, to install permanently:
levelrail-cli completion fish > ~/.config/fish/completions/levelrail-cli.fish
```

Run `levelrail-cli completion -h` for the same instructions from the CLI
itself.

## Where to go next

- [docs/architecture.md](architecture.md): how the control plane, the
  node agent, and the reconciler fit together.
- [docs/app-spec-reference.md](app-spec-reference.md): the full
  `app.yaml` schema.
- [docs/comparison.md](comparison.md): how Levelrail's approach
  differs from Coolify, Dokploy, CapRover, Dokku, and Kamal.
- [docs/roadmap.md](roadmap.md): what's shipped, what's in progress,
  and what's not started yet.
- [docs/master-key-rotation.md](master-key-rotation.md): rotating the
  envelope-encryption master key.
- [CHANGELOG.md](../CHANGELOG.md): what changed between releases.
