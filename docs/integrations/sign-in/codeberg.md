---
title: "Use Codeberg with Levelrail: dashboard sign-in"
sidebarTitle: "Codeberg"
description: "Add a Sign in with Codeberg button to the Levelrail dashboard using an OAuth app you register there."
---

Add a Sign in with Codeberg button to the Levelrail dashboard using an OAuth app you register there.

This page covers how to let people sign in to the dashboard with Codeberg.

## What you get

- A Codeberg button on the sign-in page once the provider is enabled.
- Account linking from a live session, so an existing user can attach their Codeberg identity.
- PKCE, single use state and an encrypted client secret on every flow.

## Set up

1. Prepare the credential. OAuth 2.0 authorization code with PKCE. Register an app at Codeberg, Settings, Applications, Manage OAuth2 applications (confidential client).
2. In the dashboard, open **Settings > OAuth sign-in** (`/settings/oauth`), then the Codeberg row.
3. Open the sign-in page in a private window and confirm the new button completes a login.

From the CLI:

```bash
levelrail-cli settings oauth set codeberg --client-id <CLIENT_ID> --client-secret <CLIENT_SECRET>
levelrail-cli settings oauth list
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Scopes requested | the provider default (no scopes are requested explicitly) |
| Who may change it | Root. Settings reads need the read ability. |

## Endpoint and format

Callback URL to register at Codeberg: `https://<your-dashboard-host>/api/v1/auth/oauth/codeberg/callback`

## Security notes

- Client secrets and signing keys are encrypted under the control plane master key and never returned by the API.
- Changing sign-in settings needs a root session or token, and refusals are written to the audit log.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Sign-in requires a verified email address from the provider.
- Enabling needs a client id and a client secret saved first.
- The redirect URI is built from the dashboard URL, so set it under Settings, General before you register the app.

## Troubleshooting

**Codeberg says the redirect URI does not match.** Register exactly `https://<dashboard-host>/api/v1/auth/oauth/codeberg/callback`, with the same scheme and host as your dashboard URL.

**Saving the provider is refused.** Enabling requires a client id and a client secret. Create a confidential OAuth2 application so that it issues a client secret.

**Sign-in works for others but one person is refused.** The identity must carry a verified email, and the optional allowed email domain on the row must match it.

## FAQ

**Where is the client secret stored?**

Envelope encrypted under the control plane master key. The API only reveals whether one is set.

**Can people link Codeberg to an existing account?**

Yes, from a signed in session through the link flow. The identity can belong to one user only.

## Related

- [GitLab](/integrations/sign-in/gitlab): Sign-in providers
- [GitHub](/integrations/sign-in/github): Sign-in providers
- [All sign-in providers](/integrations/sign-in)
- [Codeberg documentation](https://docs.codeberg.org/), maintained by the vendor
