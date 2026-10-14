---
title: "Use Docker Hub with Levelrail: private image pulls"
sidebarTitle: "Docker Hub"
description: "Store a credential for Docker Hub so deploys and image checks can pull private images from it."
---

Store a credential for Docker Hub so deploys and image checks can pull private images from it.

This page covers how to pull private images from Docker Hub.

## What you get

- Private images from Docker Hub pulled with the stored credential at deploy time.
- Test connection, which authenticates the typed credential and stores nothing.
- An expiry reminder when you record or derive an expiry date.

## Set up

1. Prepare the credential. your Docker Hub user as username with a personal access token.
2. In the dashboard, open **Settings > Registry credentials** (`/settings/registry-credentials?connect=dockerhub`).
3. Use Test connection in the dialog: it authenticates the typed credential against the registry and stores nothing.

From the CLI:

```bash
levelrail-cli registry credentials add --vendor dockerhub --username <USER>
levelrail-cli registry credentials test <credential-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Pull images | A read-only token is enough to pull. |

## Endpoint and format

Registry host: `docker.io`.

## Security notes

- The password or token is encrypted at rest and never returned by the API or shown again.
- Creating, testing and deleting credentials needs the write:sensitive ability; listing needs read.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- The credential holds a username and password or token only; it is not used to push images.

## Troubleshooting

**Test connection reports authentication rejected.** The registry answered unauthorized. Use a Docker Hub personal access token, not the account password, and confirm it is not expired.

**Test connection says the registry could not be reached.** The host did not answer. Check the registry host spelling and that the control plane can reach it outbound.

**A deploy fails pulling an image that exists.** The image host must match the credential's registry host exactly, and the token must be able to read that repository.

## FAQ

**Is the Docker Hub token stored in plain text?**

No. It is encrypted at rest and never returned by the API or shown again after saving.

**Can I deep link straight to this form?**

Yes. Open /settings/registry-credentials?connect=dockerhub on your dashboard.

## Related

- [GitHub Container Registry](/integrations/container-registries/ghcr): Container registries
- [Quay](/integrations/container-registries/quay): Container registries
- [All container registries](/integrations/container-registries)
- [Docker Hub documentation](https://docs.docker.com/docker-hub/), maintained by the vendor
