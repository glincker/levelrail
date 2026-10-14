---
title: "Use npm with Levelrail: private package builds"
sidebarTitle: "npm"
description: "Give Dockerfile builds access to a private npm registry through a BuildKit secret mount, so the token never lands in an image layer."
---

Give Dockerfile builds access to a private npm registry through a BuildKit secret mount, so the token never lands in an image layer.

This page covers how to install private npm dependencies in Docker builds.

## What you get

- A generated `.npmrc` file delivered to the build as secret id `npmrc`.
- Scope control: one credential can apply to every build, one project or one app, and the narrowest scope wins.
- Masking of the secret values in build progress and logs.

## Set up

1. Prepare the credential. A registry URL plus an optional username and a token or password. Omit the username for a bearer token registry.
2. In the dashboard, open **Settings > Registry credentials** (`/settings/registry-credentials`), then use the Package registries section.
3. Press Test: it makes one authenticated request to the URL and reports whether it was reachable and accepted the credential.

From the CLI:

```bash
levelrail-cli registry package-registries add --ecosystem npm --name corp-npm --url <REGISTRY_URL>
levelrail-cli registry package-registries test <registry-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Read packages | The credential needs read access to the private packages your build installs. Levelrail does not publish packages. |

## Endpoint and format

Secret id `npmrc`. Generated file: an `.npmrc` with `registry` and `_authToken` or basic auth. Dockerfile mount: `RUN --mount=type=secret,id=npmrc,target=/root/.npmrc npm ci`.

## Security notes

- The credential reaches the build as a BuildKit secret mount, so it is never part of an image layer or the build cache key.
- Secret values are masked in build progress and logs before they are recorded in deploy history.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Secret mounts apply to Dockerfile builds on the control plane. A build sent to a remote build node fails with a clear error instead of building without credentials.
- Railpack builds and pipeline build-only steps do not receive the mounts yet.
- There is no token rotation reminder: delete and re-add the registry to rotate.

## Troubleshooting

**Test reports the registry rejected the credential.** Test makes one authenticated request to the URL. Check the token has read access and that the URL is the registry root your .npmrc would use.

**The build cannot find the private package.** The Dockerfile must mount the secret on the install step: `RUN --mount=type=secret,id=npmrc,target=/root/.npmrc npm ci`.

**The build fails on a remote build node.** Secret mounts are not supported on remote build nodes yet, so route this app's builds to the control plane.

## FAQ

**Does the token end up in my image?**

No. It is mounted as a BuildKit secret for that one RUN step, so it is not part of any layer or the build cache key.

**Can two registries share the npm ecosystem?**

Yes. Scope them to different projects or apps, and the narrowest scope wins when both match.

## Related

- [PyPI](/integrations/package-registries/pypi): Package registries
- [GitHub Container Registry](/integrations/container-registries/ghcr): Container registries
- [All package registries](/integrations/package-registries)
- [npm documentation](https://docs.npmjs.com/), maintained by the vendor
