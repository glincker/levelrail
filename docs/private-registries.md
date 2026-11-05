---
title: "Private registries"
description: Connect container registries with vendor presets and give builds private npm, PyPI, Maven, NuGet, Cargo, Go and RubyGems packages through secret mounts.
"og:url": "https://docs.levelrail.com/private-registries"
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

## Run your own registry

The catalog has a template for the open source Docker registry with basic auth, `docker-registry-auth` (image `registry:3`, port 5000, data in the `registry_auth_data` volume). There is also an open one, `registry`, with no auth. Do not expose that one to the internet. Levelrail also has a built in registry, see `levelrail-cli registry enable` in the [CLI reference](/cli-reference#registry).

These steps show the template route. They have not been run end to end on a test host yet, so check each step on a non production server first.

### 1. Deploy it with your own login

The template generates its username and password at deploy time and stores them as secrets. There is no documented command to read them back, so set your own values instead. Save the template, replace the two generated tokens, and map a domain to the `registry` service:

```bash
levelrail-cli templates get docker-registry-auth --output json | jq -r .compose > registry.compose.yaml
sed -i.bak 's/\$SERVICE_USER_REGISTRY/ci/; s/\$SERVICE_PASSWORD_REGISTRY/CHANGE-ME-LONG-PASSWORD/' registry.compose.yaml
printf 'x-levelrail-domains:\n  registry: registry.example.com\n' >> registry.compose.yaml
levelrail-cli compose preview -f registry.compose.yaml
levelrail-cli compose deploy registry -f registry.compose.yaml
```

The registry builds an htpasswd file with bcrypt from those two values every time it starts. Point DNS for `registry.example.com` at your server first. HTTPS is issued automatically once DNS resolves, see [Add a domain and go live](/domains-and-ingress#add-a-domain-and-go-live). Do not allow port 5000 through your firewall; clients use the domain on 443.

### 2. Push from a laptop

```bash
docker login registry.example.com -u ci
docker tag my-app:1.0 registry.example.com/team/my-app:1.0
docker push registry.example.com/team/my-app:1.0
```

The push prints a line like `1.0: digest: sha256:... size: ...`. Copy the `sha256:` value. That is the digest.

### 3. Give Levelrail the login

```bash
levelrail-cli registry-credentials create --name my-registry --registry-host registry.example.com --username ci --password 'CHANGE-ME-LONG-PASSWORD'
levelrail-cli registry-credentials test <id>
```

`test` signs in to the registry without pulling anything. `repositories` and `tags` list what you pushed:

```bash
levelrail-cli registry-credentials tags <id> team/my-app
```

### 4. Deploy from the private image by digest

```bash
levelrail-cli apps create --name my-app --image registry.example.com/team/my-app@sha256:DIGEST --port 3000
```

A digest names exact image content, so a later push to the same tag cannot change what runs. To update, push a new image and deploy its new digest.

### Not yet

- Reading the generated registry login back from Levelrail. Use your own values as above.
- A `--digest` flag. The digest goes in the image reference.

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
