---
title: "Use Google Artifact Registry with Levelrail: private image pulls"
sidebarTitle: "Google Artifact Registry"
description: "Store a credential for Google Artifact Registry so deploys and image checks can pull private images from it."
---

Store a credential for Google Artifact Registry so deploys and image checks can pull private images from it.

This page covers how to pull private images from Google Artifact Registry.

## What you get

- Private images from Google Artifact Registry pulled with the stored credential at deploy time.
- Test connection, which authenticates the typed credential and stores nothing.
- An expiry reminder when you record or derive an expiry date.

## Set up

1. Prepare the credential. Username `_json_key` with the service account key JSON.
2. In the dashboard, open **Settings > Registry credentials** (`/settings/registry-credentials?connect=gar`).
3. Use Test connection in the dialog: it authenticates the typed credential against the registry and stores nothing.

From the CLI:

```bash
levelrail-cli registry credentials add --vendor gar --registry-host <HOST>
levelrail-cli registry credentials test <credential-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Pull images | The `roles/artifactregistry.reader` role on the repository. |

## Endpoint and format

Registry host: `<region>-docker.pkg.dev`.

## Security notes

- The password or token is encrypted at rest and never returned by the API or shown again.
- Creating, testing and deleting credentials needs the write:sensitive ability; listing needs read.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- The credential holds a username and password or token only; it is not used to push images.

## Troubleshooting

**Test connection reports authentication rejected.** The registry answered unauthorized. Paste the whole service account key JSON as the password and keep the username as _json_key.

**Test connection says the registry could not be reached.** The host did not answer. Check the registry host spelling and that the control plane can reach it outbound.

**A deploy fails pulling an image that exists.** The image host must match the credential's registry host exactly, and the token must be able to read that repository.

## FAQ

**Is the Google Artifact Registry token stored in plain text?**

No. It is encrypted at rest and never returned by the API or shown again after saving.

**Can I deep link straight to this form?**

Yes. Open /settings/registry-credentials?connect=gar on your dashboard.

## Related

- [Azure Container Registry](/integrations/container-registries/acr): Container registries
- [Google Cloud](/integrations/cloud-providers/gcp): Cloud node providers
- [All container registries](/integrations/container-registries)
- [Google Artifact Registry documentation](https://cloud.google.com/artifact-registry/docs), maintained by the vendor
