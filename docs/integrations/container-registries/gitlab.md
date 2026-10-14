---
title: "Use GitLab Container Registry with Levelrail: private image pulls"
sidebarTitle: "GitLab Container Registry"
description: "Store a credential for GitLab Container Registry so deploys and image checks can pull private images from it."
---

Store a credential for GitLab Container Registry so deploys and image checks can pull private images from it.

This page covers how to pull private images from GitLab Container Registry.

## What you get

- Private images from GitLab Container Registry pulled with the stored credential at deploy time.
- Test connection, which authenticates the typed credential and stores nothing.
- An expiry reminder when you record or derive an expiry date.

## Set up

1. Prepare the credential. a deploy token user as username with a deploy token or personal access token.
2. In the dashboard, open **Settings > Registry credentials** (`/settings/registry-credentials?connect=gitlab`).
3. Use Test connection in the dialog: it authenticates the typed credential against the registry and stores nothing.

From the CLI:

```bash
levelrail-cli registry credentials add --vendor gitlab --username <USER>
levelrail-cli registry credentials test <credential-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Pull images | `read_registry` to pull, plus `write_registry` to push. |

## Endpoint and format

Registry host: `registry.gitlab.com`, or your self-hosted registry host. Self-managed GitLab uses your own registry host.

## Security notes

- The password or token is encrypted at rest and never returned by the API or shown again.
- Creating, testing and deleting credentials needs the write:sensitive ability; listing needs read.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- The credential holds a username and password or token only; it is not used to push images.

## Troubleshooting

**Test connection reports authentication rejected.** The registry answered unauthorized. Create a deploy token with read_registry and use its generated username.

**Test connection says the registry could not be reached.** The host did not answer. Check the registry host spelling and that the control plane can reach it outbound.

**A deploy fails pulling an image that exists.** The image host must match the credential's registry host exactly, and the token must be able to read that repository.

## FAQ

**Is the GitLab Container Registry token stored in plain text?**

No. It is encrypted at rest and never returned by the API or shown again after saving.

**Can I deep link straight to this form?**

Yes. Open /settings/registry-credentials?connect=gitlab on your dashboard.

## Related

- [GitLab](/integrations/git-providers/gitlab): Git providers
- [Harbor](/integrations/container-registries/harbor): Container registries
- [All container registries](/integrations/container-registries)
- [GitLab Container Registry documentation](https://docs.gitlab.com/ee/user/packages/container_registry/), maintained by the vendor
