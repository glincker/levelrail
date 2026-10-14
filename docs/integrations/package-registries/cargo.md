---
title: "Use Cargo with Levelrail: private package builds"
sidebarTitle: "Cargo"
description: "Give Dockerfile builds access to a private Cargo registry through a BuildKit secret mount, so the token never lands in an image layer."
---

Give Dockerfile builds access to a private Cargo registry through a BuildKit secret mount, so the token never lands in an image layer.

This page covers how to install private Cargo dependencies in Docker builds.

## What you get

- A generated `config.toml` file delivered to the build as secret id `cargo_config`.
- Scope control: one credential can apply to every build, one project or one app, and the narrowest scope wins.
- Masking of the secret values in build progress and logs.

## Set up

1. Prepare the credential. A registry URL plus an optional username and a token or password. Omit the username for a bearer token registry.
2. In the dashboard, open **Settings > Registry credentials** (`/settings/registry-credentials`), then use the Package registries section.
3. Press Test: it makes one authenticated request to the URL and reports whether it was reachable and accepted the credential.

From the CLI:

```bash
levelrail-cli registry package-registries add --ecosystem cargo --name corp-cargo --url <REGISTRY_URL>
levelrail-cli registry package-registries test <registry-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Read packages | The credential needs read access to the private packages your build installs. Levelrail does not publish packages. |

## Endpoint and format

Secret id `cargo_config`. Generated file: a Cargo `config.toml` with the registry index and token. Dockerfile mount: `RUN --mount=type=secret,id=cargo_config,target=/usr/local/cargo/config.toml cargo build --release`.

## Security notes

- The credential reaches the build as a BuildKit secret mount, so it is never part of an image layer or the build cache key.
- Secret values are masked in build progress and logs before they are recorded in deploy history.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- The target path in the Dockerfile example is your choice: it must be a location Cargo reads.
- Secret mounts apply to Dockerfile builds on the control plane. A build sent to a remote build node fails with a clear error instead of building without credentials.
- Railpack builds and pipeline build-only steps do not receive the mounts yet.
- There is no token rotation reminder: delete and re-add the registry to rotate.

## Troubleshooting

**Test reports the registry rejected the credential.** Test makes one authenticated request to the URL. Use the registry index URL, and a token valid for that registry.

**The build cannot find the private package.** The Dockerfile must mount the secret on the install step: `RUN --mount=type=secret,id=cargo_config,target=/usr/local/cargo/config.toml cargo build --release`.

**The build fails on a remote build node.** Secret mounts are not supported on remote build nodes yet, so route this app's builds to the control plane.

## FAQ

**Does the token end up in my image?**

No. It is mounted as a BuildKit secret for that one RUN step, so it is not part of any layer or the build cache key.

**Can two registries share the Cargo ecosystem?**

Yes. Scope them to different projects or apps, and the narrowest scope wins when both match.

## Related

- [Go modules](/integrations/package-registries/go): Package registries
- [npm](/integrations/package-registries/npm): Package registries
- [All package registries](/integrations/package-registries)
- [Cargo documentation](https://doc.rust-lang.org/cargo/), maintained by the vendor
