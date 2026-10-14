---
title: "Use GitLab with Levelrail: dashboard sign-in"
sidebarTitle: "GitLab"
description: "Add a Sign in with GitLab button to the Levelrail dashboard using an OAuth app you register there."
---

Add a Sign in with GitLab button to the Levelrail dashboard using an OAuth app you register there.

This page covers how to let people sign in to the dashboard with GitLab.

## What you get

- A GitLab button on the sign-in page once the provider is enabled.
- Account linking from a live session, so an existing user can attach their GitLab identity.
- PKCE, single use state and an encrypted client secret on every flow.

## Set up

1. Prepare the credential. OAuth 2.0 authorization code with PKCE. Register an app at GitLab, User settings, Applications (mark it Confidential and tick openid, profile and email).
2. In the dashboard, open **Settings > OAuth sign-in** (`/settings/oauth`), then the GitLab row.
3. Open the sign-in page in a private window and confirm the new button completes a login.

From the CLI:

```bash
levelrail-cli settings oauth set gitlab --client-id <CLIENT_ID> --client-secret <CLIENT_SECRET> --issuer-url <INSTANCE_URL>
levelrail-cli settings oauth list
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Scopes requested | `openid email profile` |
| Who may change it | Root. Settings reads need the read ability. |

## Endpoint and format

Callback URL to register at GitLab: `https://<your-dashboard-host>/api/v1/auth/oauth/gitlab/callback`

## Security notes

- Client secrets and signing keys are encrypted under the control plane master key and never returned by the API.
- Changing sign-in settings needs a root session or token, and refusals are written to the audit log.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Self-managed GitLab: set the instance URL on the row (the issuer_url field in the API). It must use https.
- Sign-in requires a verified email address from the provider.
- Enabling needs a client id and a client secret saved first.
- The redirect URI is built from the dashboard URL, so set it under Settings, General before you register the app.

## Troubleshooting

**GitLab says the redirect URI does not match.** Register exactly `https://<dashboard-host>/api/v1/auth/oauth/gitlab/callback`, with the same scheme and host as your dashboard URL.

**Saving the provider is refused.** Enabling requires a client id and a client secret, and an issuer or instance value for this provider. For a self-managed GitLab, set the instance URL; it must be https and blank means gitlab.com.

**Sign-in works for others but one person is refused.** The identity must carry a verified email, and the optional allowed email domain on the row must match it.

## FAQ

**Does this work with self-managed GitLab?**

Yes. Set the instance URL, which must be an https address. Left blank, the row talks to gitlab.com.

**Where is the client secret stored?**

Envelope encrypted under the control plane master key. The API only reveals whether one is set.

**Can people link GitLab to an existing account?**

Yes, from a signed in session through the link flow. The identity can belong to one user only.

## Related

- [GitHub](/integrations/sign-in/github): Sign-in providers
- [GitLab](/integrations/git-providers/gitlab): Git providers
- [All sign-in providers](/integrations/sign-in)
- [GitLab documentation](https://docs.gitlab.com/ee/integration/oauth_provider.html), maintained by the vendor
