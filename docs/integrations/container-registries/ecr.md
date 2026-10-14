---
title: "Use AWS ECR with Levelrail: private image pulls"
sidebarTitle: "AWS ECR"
description: "Store a credential for AWS ECR so deploys and image checks can pull private images from it."
---

Store a credential for AWS ECR so deploys and image checks can pull private images from it.

This page covers how to pull private images from AWS ECR.

## What you get

- Private images from AWS ECR pulled with the stored credential at deploy time.
- Test connection, which authenticates the typed credential and stores nothing.
- An expiry reminder when you record or derive an expiry date.

## Set up

1. Prepare the credential. Username `AWS` with the output of `aws ecr get-login-password`.
2. In the dashboard, open **Settings > Registry credentials** (`/settings/registry-credentials?connect=ecr`).
3. Use Test connection in the dialog: it authenticates the typed credential against the registry and stores nothing.

From the CLI:

```bash
levelrail-cli registry credentials add --vendor ecr --registry-host <HOST>
levelrail-cli registry credentials test <credential-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Pull images | `ecr:GetAuthorizationToken`, `ecr:BatchGetImage`, `ecr:GetDownloadUrlForLayer` and `ecr:BatchCheckLayerAvailability`. |

## Endpoint and format

Registry host: `<account>.dkr.ecr.<region>.amazonaws.com`.

## Security notes

- The password or token is encrypted at rest and never returned by the API or shown again.
- Creating, testing and deleting credentials needs the write:sensitive ability; listing needs read.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- The token lasts 12 hours. The preset records a 12 hour expiry so the reminder fires, but Levelrail does not refresh it yet.
- The credential holds a username and password or token only; it is not used to push images.

## Troubleshooting

**Test connection reports authentication rejected.** The registry answered unauthorized. ECR tokens last 12 hours. Re-issue one with the AWS CLI and update the credential.

**Test connection says the registry could not be reached.** The host did not answer. Check the registry host spelling and that the control plane can reach it outbound.

**A deploy fails pulling an image that exists.** The image host must match the credential's registry host exactly, and the token must be able to read that repository.

## FAQ

**Is the AWS ECR token stored in plain text?**

No. It is encrypted at rest and never returned by the API or shown again after saving.

**Can I deep link straight to this form?**

Yes. Open /settings/registry-credentials?connect=ecr on your dashboard.

## Related

- [AWS EC2](/integrations/cloud-providers/aws): Cloud node providers
- [Google Artifact Registry](/integrations/container-registries/gar): Container registries
- [All container registries](/integrations/container-registries)
- [AWS ECR documentation](https://docs.aws.amazon.com/AmazonECR/), maintained by the vendor
