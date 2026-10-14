---
title: "Private registries"
description: Connect container registries with vendor presets and give builds private npm, PyPI, Maven, NuGet, Cargo, Go and RubyGems packages through secret mounts.
---

Two different things live under **Settings > Registry credentials** (`/settings/registry-credentials`):

- **Container registries** are where your images are pulled from. A credential here is a username and a password or token.
- **Package registries** are where a build installs dependencies from. A credential here is mounted into the Dockerfile build as a secret.

## Container registry vendors

Each vendor tile opens a dialog you can deep link to, for example `/settings/registry-credentials?connect=ghcr`. The preset pre-fills the host, names the auth method and lists the scopes the token needs. **Test connection** authenticates the typed credential against the registry and stores nothing. **Save credential** stores it, encrypted at rest and never shown again.

| Vendor | Host | Username | Password | Scopes to grant |
| --- | --- | --- | --- | --- |
| Docker Hub | `docker.io` | Docker Hub user | Personal access token | Read-only to pull |
| GitHub Container Registry | `ghcr.io` | GitHub user | Personal access token (classic) | `read:packages`, plus `write:packages` to push |
| GitLab | `registry.gitlab.com` | Deploy token user | Deploy token or PAT | `read_registry`, plus `write_registry` to push |
| AWS ECR | `<account>.dkr.ecr.<region>.amazonaws.com` | `AWS` | `aws ecr get-login-password` output | `ecr:GetAuthorizationToken`, `BatchGetImage`, `GetDownloadUrlForLayer`, `BatchCheckLayerAvailability` |
| Google Artifact Registry | `<region>-docker.pkg.dev` | `_json_key` | Service account key JSON | `roles/artifactregistry.reader` |
| Azure Container Registry | `<name>.azurecr.io` | Token name or app id | Token or service principal secret | `AcrPull` |
| Harbor | your host | `robot$name` | Robot secret | Pull on the project |
| Quay | `quay.io` | `org+robot` | Robot token | Read on the repository |
| JFrog Artifactory | `<name>.jfrog.io` | User | Identity token | Read on the Docker repository |
| Sonatype Nexus | your host and port | User | Password | `nx-repository-view-docker-*-read` |
| Docker v2 | your host | User | Password or token | Pull |

ECR tokens last 12 hours. The preset records a 12 hour expiry so the existing expiry reminder fires, but the control plane does not refresh the token itself yet: re-issue it with the CLI before it lapses.

The table lists which services pull with each credential.

```bash
levelrail-cli registry credentials vendors
levelrail-cli registry credentials add --vendor ghcr --username octocat
levelrail-cli registry credentials add --vendor ecr --registry-host 123456789012.dkr.ecr.eu-west-1.amazonaws.com
levelrail-cli registry credentials test <id>
levelrail-cli registry credentials delete <id>
```

`add` reads the password from stdin or a no-echo prompt when `--password` is omitted. Every command takes `--json`.

## Package registries for builds

A package registry credential holds an ecosystem, a URL, an optional username and a token. It applies to every build, one project or one app, and the narrowest scope wins when two registries share an ecosystem.

| Ecosystem | Secret id | What is generated |
| --- | --- | --- |
| npm | `npmrc` | `.npmrc` with `registry` and `_authToken` or basic auth |
| PyPI | `pip_conf` | `pip.conf` with `index-url` |
| Maven | `maven_settings` | `settings.xml` with a server and a mirror |
| NuGet | `nuget_config` | `nuget.config` with source credentials |
| Cargo | `cargo_config` | `config.toml` with the registry index and token |
| Go | `netrc` | `.netrc`, plus the `GOPROXY` and `GOPRIVATE` build args |
| RubyGems | `bundle_config` | Bundler config with the gem host credential |

The files reach the build as BuildKit secret mounts, so they are never part of an image layer or the build cache key. Mount the one you need in your Dockerfile:

```dockerfile
RUN --mount=type=secret,id=npmrc,target=/root/.npmrc npm ci
RUN --mount=type=secret,id=pip_conf,target=/etc/pip.conf pip install -r requirements.txt
RUN --mount=type=secret,id=netrc,target=/root/.netrc go mod download
```

Only non-secret values (`GOPROXY`, `GOPRIVATE`) are passed as build args. The secret values are masked in build progress and logs before they are recorded in the deploy history.

```bash
levelrail-cli registry package-registries add --ecosystem npm --name corp-npm --url https://npm.example.com/repo
levelrail-cli registry package-registries list
levelrail-cli registry package-registries test <id>
levelrail-cli registry package-registries delete <id>
```

**Test** makes one authenticated request to the URL and reports whether it was reachable and accepted the credential.

### Limits

- Secret mounts apply to Dockerfile builds on the control plane. A build dispatched to a remote build node fails with a clear error instead of building without credentials, and Railpack and pipeline `build-only` steps do not receive them yet. The wiring points are `internal/build/router.go` (`Router.Build`), `internal/build/remote.go` (`RemoteRequest`) and `internal/deploy/build_only.go`.
- There is no automatic token rotation reminder. Rotate by deleting and re-adding the registry.

## Access

Listing and reading usage needs `read`. Adding, testing and removing credentials needs `write:sensitive`. Credential values are never returned by the API.
