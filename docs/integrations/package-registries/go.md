---
title: "Use Go modules with Levelrail: private package builds"
sidebarTitle: "Go modules"
description: "Give Dockerfile builds access to a private Go modules registry through a BuildKit secret mount, so the token never lands in an image layer."
---

Give Dockerfile builds access to a private Go modules registry through a BuildKit secret mount, so the token never lands in an image layer.

This page covers how to install private Go modules dependencies in Docker builds.

## What you get

- A generated `.netrc` file delivered to the build as secret id `netrc`.
- Scope control: one credential can apply to every build, one project or one app, and the narrowest scope wins.
- Masking of the secret values in build progress and logs.

## Set up

1. Prepare the credential. A registry URL plus an optional username and a token or password. Omit the username for a bearer token registry.
2. In the dashboard, open **Settings > Registry credentials** (`/settings/registry-credentials`), then use the Package registries section.
3. Press Test: it makes one authenticated request to the URL and reports whether it was reachable and accepted the credential.

From the CLI:

```bash
levelrail-cli registry package-registries add --ecosystem go --name corp-go --url <REGISTRY_URL> --module-patterns 'example.com/*'
levelrail-cli registry package-registries test <registry-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Read packages | The credential needs read access to the private packages your build installs. Levelrail does not publish packages. |

## Endpoint and format

Secret id `netrc`. Generated file: a `.netrc`, plus the `GOPROXY` and `GOPRIVATE` build args. Dockerfile mount: `RUN --mount=type=secret,id=netrc,target=/root/.netrc go mod download`.

## Security notes

- The credential reaches the build as a BuildKit secret mount, so it is never part of an image layer or the build cache key.
- Secret values are masked in build progress and logs before they are recorded in deploy history.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Only GOPROXY and GOPRIVATE are passed as build args, and neither carries the secret.
- Secret mounts apply to Dockerfile builds on the control plane. A build sent to a remote build node fails with a clear error instead of building without credentials.
- Railpack builds and pipeline build-only steps do not receive the mounts yet.
- There is no token rotation reminder: delete and re-add the registry to rotate.

## Troubleshooting

**Test reports the registry rejected the credential.** Test makes one authenticated request to the URL. Check the proxy URL and that the .netrc login matches the proxy host. Set module patterns so private paths bypass the public proxy.

**The build cannot find the private package.** The Dockerfile must mount the secret on the install step: `RUN --mount=type=secret,id=netrc,target=/root/.netrc go mod download`.

**The build fails on a remote build node.** Secret mounts are not supported on remote build nodes yet, so route this app's builds to the control plane.

## FAQ

**Does the token end up in my image?**

No. It is mounted as a BuildKit secret for that one RUN step, so it is not part of any layer or the build cache key.

**Can two registries share the Go modules ecosystem?**

Yes. Scope them to different projects or apps, and the narrowest scope wins when both match.

## Related

- [npm](/integrations/package-registries/npm): Package registries
- [GitHub](/integrations/git-providers/github): Git providers
- [All package registries](/integrations/package-registries)
- [Go modules documentation](https://go.dev/ref/mod), maintained by the vendor
