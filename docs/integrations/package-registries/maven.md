---
title: "Use Maven with Levelrail: private package builds"
sidebarTitle: "Maven"
description: "Give Dockerfile builds access to a private Maven registry through a BuildKit secret mount, so the token never lands in an image layer."
---

Give Dockerfile builds access to a private Maven registry through a BuildKit secret mount, so the token never lands in an image layer.

This page covers how to install private Maven dependencies in Docker builds.

## What you get

- A generated `settings.xml` file delivered to the build as secret id `maven_settings`.
- Scope control: one credential can apply to every build, one project or one app, and the narrowest scope wins.
- Masking of the secret values in build progress and logs.

## Set up

1. Prepare the credential. A registry URL plus an optional username and a token or password. Omit the username for a bearer token registry.
2. In the dashboard, open **Settings > Registry credentials** (`/settings/registry-credentials`), then use the Package registries section.
3. Press Test: it makes one authenticated request to the URL and reports whether it was reachable and accepted the credential.

From the CLI:

```bash
levelrail-cli registry package-registries add --ecosystem maven --name corp-maven --url <REGISTRY_URL>
levelrail-cli registry package-registries test <registry-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Read packages | The credential needs read access to the private packages your build installs. Levelrail does not publish packages. |

## Endpoint and format

Secret id `maven_settings`. Generated file: a `settings.xml` with a server and a mirror. Dockerfile mount: `RUN --mount=type=secret,id=maven_settings,target=/root/.m2/settings.xml mvn -B package`.

## Security notes

- The credential reaches the build as a BuildKit secret mount, so it is never part of an image layer or the build cache key.
- Secret values are masked in build progress and logs before they are recorded in deploy history.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- The target path in the Dockerfile example is your choice: Maven reads settings from wherever you point it.
- Secret mounts apply to Dockerfile builds on the control plane. A build sent to a remote build node fails with a clear error instead of building without credentials.
- Railpack builds and pipeline build-only steps do not receive the mounts yet.
- There is no token rotation reminder: delete and re-add the registry to rotate.

## Troubleshooting

**Test reports the registry rejected the credential.** Test makes one authenticated request to the URL. Use the repository URL your settings.xml mirror would point at.

**The build cannot find the private package.** The Dockerfile must mount the secret on the install step: `RUN --mount=type=secret,id=maven_settings,target=/root/.m2/settings.xml mvn -B package`.

**The build fails on a remote build node.** Secret mounts are not supported on remote build nodes yet, so route this app's builds to the control plane.

## FAQ

**Does the token end up in my image?**

No. It is mounted as a BuildKit secret for that one RUN step, so it is not part of any layer or the build cache key.

**Can two registries share the Maven ecosystem?**

Yes. Scope them to different projects or apps, and the narrowest scope wins when both match.

## Related

- [NuGet](/integrations/package-registries/nuget): Package registries
- [Sonatype Nexus](/integrations/container-registries/nexus): Container registries
- [All package registries](/integrations/package-registries)
- [Maven documentation](https://maven.apache.org/), maintained by the vendor
