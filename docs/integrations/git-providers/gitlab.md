---
title: "Use GitLab with Levelrail: push deploys and previews"
sidebarTitle: "GitLab"
description: "Connect GitLab once, then point apps at repositories for push deploys, release deploys and per pull request previews."
---

Connect GitLab once, then point apps at repositories for push deploys, release deploys and per pull request previews.

This page covers how to deploy from GitLab pushes and pull requests.

## What you get

- Push, tag or pull request events that deploy an app or build a preview.
- Repository and branch listing plus webhook registration through `use-as-source`.
- Webhook history with replay, and PR comments and commit statuses.

## Set up

1. Prepare the credential. An OAuth Application you register in GitLab. Paste its instance_url, client_id and client_secret, then authorize.
2. In the dashboard, open **Settings > GitLab App** (`/settings/gitlab-app`).
3. List repositories with the provider command, then connect one to an app with `use-as-source` and push a commit.

From the CLI:

```bash
levelrail-cli gitlab-app projects
levelrail-cli gitlab-app use-as-source <project-id> --app-name my-app
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| OAuth application | Authorized through the redirect to your instance's /oauth/authorize. |

## Endpoint and format

Authorization endpoint: `<instance_url>/oauth/authorize`.

## Security notes

- Provider secrets are encrypted with the master key; each app has its own webhook secret.
- Disconnecting removes the connection and its secrets from the control plane only. Revoke the token or app at the provider as well.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Works with self-managed GitLab through instance_url.
- Projects are addressed by numeric project id.
- If webhook registration fails, the whole use-as-source request fails with 502.
- The connection needs a master key on the control plane; routes return 501 without one.

## Troubleshooting

**Authorize redirects to the wrong host.** instance_url must be your GitLab address, with https.

**use-as-source returns 502.** GitLab webhook registration failed. Check the connected user can manage webhooks on that project.

**Project not found.** Use the numeric project id from `gitlab-app projects`, not the path.

## FAQ

**Does it work with self-managed GitLab?**

Yes, set instance_url to your instance.

**Is the same application used for sign-in?**

No. Sign in with GitLab is a separate OAuth app under OAuth sign-in.

## Related

- [GitLab](/integrations/sign-in/gitlab): Sign-in providers
- [GitLab Container Registry](/integrations/container-registries/gitlab): Container registries
- [All git providers](/integrations/git-providers)
- [GitLab documentation](https://docs.gitlab.com/ee/integration/oauth_provider.html), maintained by the vendor
