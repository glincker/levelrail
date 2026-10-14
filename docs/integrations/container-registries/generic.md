---
title: "Use Docker v2 registry with Levelrail: private image pulls"
sidebarTitle: "Docker v2 registry"
description: "Store a credential for Docker v2 registry so deploys and image checks can pull private images from it."
---

Store a credential for Docker v2 registry so deploys and image checks can pull private images from it.

This page covers how to pull private images from Docker v2 registry.

## What you get

- Private images from Docker v2 registry pulled with the stored credential at deploy time.
- Test connection, which authenticates the typed credential and stores nothing.
- An expiry reminder when you record or derive an expiry date.

## Set up

1. Prepare the credential. your registry user as username with a password or token.
2. In the dashboard, open **Settings > Registry credentials** (`/settings/registry-credentials?connect=generic`).
3. Use Test connection in the dialog: it authenticates the typed credential against the registry and stores nothing.

From the CLI:

```bash
levelrail-cli registry credentials add --vendor generic --username <USER> --registry-host <HOST>
levelrail-cli registry credentials test <credential-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Pull images | Pull access on the repositories you deploy. |

## Endpoint and format

Registry host: your registry host.

## Security notes

- The password or token is encrypted at rest and never returned by the API or shown again.
- Creating, testing and deleting credentials needs the write:sensitive ability; listing needs read.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- The credential holds a username and password or token only; it is not used to push images.

## Troubleshooting

**Test connection reports authentication rejected.** The registry answered unauthorized. Confirm the host serves the Docker Registry HTTP API v2 and that the user can pull the repository.

**Test connection says the registry could not be reached.** The host did not answer. Check the registry host spelling and that the control plane can reach it outbound.

**A deploy fails pulling an image that exists.** The image host must match the credential's registry host exactly, and the token must be able to read that repository.

## FAQ

**Is the Docker v2 registry token stored in plain text?**

No. It is encrypted at rest and never returned by the API or shown again after saving.

**Can I deep link straight to this form?**

Yes. Open /settings/registry-credentials?connect=generic on your dashboard.

## Related

- [Harbor](/integrations/container-registries/harbor): Container registries
- [Docker Hub](/integrations/container-registries/dockerhub): Container registries
- [All container registries](/integrations/container-registries)
- [Docker v2 registry documentation](https://distribution.github.io/distribution/), maintained by the vendor
