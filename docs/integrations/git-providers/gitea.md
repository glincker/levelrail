---
title: "Use Gitea with Levelrail: push deploys and previews"
sidebarTitle: "Gitea"
description: "Connect Gitea once, then point apps at repositories for push deploys, release deploys and per pull request previews."
---

Connect Gitea once, then point apps at repositories for push deploys, release deploys and per pull request previews.

This page covers how to deploy from Gitea pushes and pull requests.

## What you get

- Push, tag or pull request events that deploy an app or build a preview.
- Repository and branch listing plus webhook registration through `use-as-source`.
- Webhook history with replay, and PR comments and commit statuses.

## Set up

1. Prepare the credential. An OAuth2 Application you register in Gitea. Paste its instance_url, client_id and client_secret, then authorize. Tokens are refreshed.
2. In the dashboard, open **Settings > Gitea App** (`/settings/gitea-app`).
3. List repositories with the provider command, then connect one to an app with `use-as-source` and push a commit.

From the CLI:

```bash
levelrail-cli gitea-app repos
levelrail-cli gitea-app use-as-source <owner> <repo> --app-name my-app
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| OAuth2 application | Authorized through the redirect to your instance's /login/oauth/authorize. |

## Endpoint and format

Authorization endpoint: `<instance_url>/login/oauth/authorize`.

## Security notes

- Provider secrets are encrypted with the master key; each app has its own webhook secret.
- Disconnecting removes the connection and its secrets from the control plane only. Revoke the token or app at the provider as well.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Almost always self-hosted, so instance_url is required.
- If webhook registration fails, the whole use-as-source request fails with 502.
- The connection needs a master key on the control plane; routes return 501 without one.

## Troubleshooting

**Authorize lands on a 404.** instance_url must point at your Gitea root, so that /login/oauth/authorize resolves.

**use-as-source returns 502.** Gitea webhook registration failed. Confirm the authorizing user can manage webhooks on the repository.

**The control plane cannot reach Gitea.** The Gitea host must be reachable from the control plane, and the control plane's webhook URL reachable from Gitea.

## FAQ

**Does Forgejo work?**

It is not listed as supported. Only Gitea is documented.

**Does preview support PR comments here?**

Yes. All four providers support previews, PR comments and commit statuses.

## Related

- [GitLab](/integrations/git-providers/gitlab): Git providers
- [GitHub](/integrations/git-providers/github): Git providers
- [All git providers](/integrations/git-providers)
- [Gitea documentation](https://docs.gitea.com/), maintained by the vendor
