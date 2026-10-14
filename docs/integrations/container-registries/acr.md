---
title: "Use Azure Container Registry with Levelrail: private image pulls"
sidebarTitle: "Azure Container Registry"
description: "Store a credential for Azure Container Registry so deploys and image checks can pull private images from it."
---

Store a credential for Azure Container Registry so deploys and image checks can pull private images from it.

This page covers how to pull private images from Azure Container Registry.

## What you get

- Private images from Azure Container Registry pulled with the stored credential at deploy time.
- Test connection, which authenticates the typed credential and stores nothing.
- An expiry reminder when you record or derive an expiry date.

## Set up

1. Prepare the credential. a token name or app id as username with a token or service principal secret.
2. In the dashboard, open **Settings > Registry credentials** (`/settings/registry-credentials?connect=acr`).
3. Use Test connection in the dialog: it authenticates the typed credential against the registry and stores nothing.

From the CLI:

```bash
levelrail-cli registry credentials add --vendor acr --username <USER> --registry-host <HOST>
levelrail-cli registry credentials test <credential-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Pull images | The `AcrPull` role. |

## Endpoint and format

Registry host: `<name>.azurecr.io`.

## Security notes

- The password or token is encrypted at rest and never returned by the API or shown again.
- Creating, testing and deleting credentials needs the write:sensitive ability; listing needs read.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- The credential holds a username and password or token only; it is not used to push images.

## Troubleshooting

**Test connection reports authentication rejected.** The registry answered unauthorized. Use a repository scoped token or a service principal holding AcrPull.

**Test connection says the registry could not be reached.** The host did not answer. Check the registry host spelling and that the control plane can reach it outbound.

**A deploy fails pulling an image that exists.** The image host must match the credential's registry host exactly, and the token must be able to read that repository.

## FAQ

**Is the Azure Container Registry token stored in plain text?**

No. It is encrypted at rest and never returned by the API or shown again after saving.

**Can I deep link straight to this form?**

Yes. Open /settings/registry-credentials?connect=acr on your dashboard.

## Related

- [Microsoft Azure](/integrations/cloud-providers/azure): Cloud node providers
- [Google Artifact Registry](/integrations/container-registries/gar): Container registries
- [All container registries](/integrations/container-registries)
- [Azure Container Registry documentation](https://learn.microsoft.com/azure/container-registry/), maintained by the vendor
