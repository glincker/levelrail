---
title: "Use GitHub with Levelrail: push deploys and previews"
sidebarTitle: "GitHub"
description: "Connect GitHub once, then point apps at repositories for push deploys, release deploys and per pull request previews."
---

Connect GitHub once, then point apps at repositories for push deploys, release deploys and per pull request previews.

This page covers how to deploy from GitHub pushes and pull requests.

## What you get

- Push, tag or pull request events that deploy an app or build a preview.
- Repository and branch listing plus webhook registration through `use-as-source`.
- Webhook history with replay, and PR comments and commit statuses.

## Set up

1. Prepare the credential. A GitHub App created by a manifest flow, or entered by hand. Tokens are short lived and minted per API call.
2. In the dashboard, open **Settings > GitHub App** (`/settings/github-app`).
3. List repositories with the provider command, then connect one to an app with `use-as-source` and push a commit.

From the CLI:

```bash
levelrail-cli github-app repos
levelrail-cli github-app use-as-source <owner> <repo> --app-name my-app --branch main
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| App installation | Choose which account or organization and which repositories to grant. |
| repository_hooks:write | Needed for use-as-source to register the webhook. An installation that predates it reports webhook_registered false. |

## Endpoint and format

Webhook URL per app: `/api/v1/webhooks/github/<app-name>`.

## Security notes

- Provider secrets are encrypted with the master key; each app has its own webhook secret.
- Disconnecting removes the connection and its secrets from the control plane only. Revoke the token or app at the provider as well.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- GitHub Enterprise Server works through instance_url.
- A private App owned by a personal account cannot be installed on an organization; own it from the organization or make it public.
- The manifest flow needs a primary domain set in ingress settings first, otherwise it returns 409.
- The connection needs a master key on the control plane; routes return 501 without one.

## Troubleshooting

**Creating the App returns 409.** GitHub needs a reachable callback URL. Set a primary domain in ingress settings, or connect a manually created App.

**The organization is not offered on the install page.** The App is private and owned by a personal account. Make it public in the App's Advanced settings, or create it under the organization.

**use-as-source reports webhook_registered false.** The installation lacks repository_hooks:write. Accept the App's updated permissions, then run use-as-source again.

## FAQ

**Do I paste a personal access token?**

No. A GitHub App is a control plane wide credential, and each app gets its own webhook secret.

**Does disconnecting delete the App on GitHub?**

No. It removes the connection and secrets from Levelrail only. Remove the App at github.com/settings/apps.

## Related

- [GitHub](/integrations/sign-in/github): Sign-in providers
- [GitHub Container Registry](/integrations/container-registries/ghcr): Container registries
- [All git providers](/integrations/git-providers)
- [GitHub documentation](https://docs.github.com/apps), maintained by the vendor
