---
title: "Use Linear with Levelrail: dashboard sign-in"
sidebarTitle: "Linear"
description: "Add a Sign in with Linear button to the Levelrail dashboard using an OAuth app you register there."
---

Add a Sign in with Linear button to the Levelrail dashboard using an OAuth app you register there.

This page covers how to let people sign in to the dashboard with Linear.

## What you get

- A Linear button on the sign-in page once the provider is enabled.
- Account linking from a live session, so an existing user can attach their Linear identity.
- PKCE, single use state and an encrypted client secret on every flow.

## Set up

1. Prepare the credential. OAuth 2.0 authorization code with PKCE. Register an app at Linear, Settings, API, OAuth applications.
2. In the dashboard, open **Settings > OAuth sign-in** (`/settings/oauth`), then the Linear row.
3. Open the sign-in page in a private window and confirm the new button completes a login.

From the CLI:

```bash
levelrail-cli settings oauth set linear --client-id <CLIENT_ID> --client-secret <CLIENT_SECRET>
levelrail-cli settings oauth list
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Scopes requested | `read` |
| Who may change it | Root. Settings reads need the read ability. |

## Endpoint and format

Callback URL to register at Linear: `https://<your-dashboard-host>/api/v1/auth/oauth/linear/callback`

## Security notes

- Client secrets and signing keys are encrypted under the control plane master key and never returned by the API.
- Changing sign-in settings needs a root session or token, and refusals are written to the audit log.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Sign-in requires a verified email address from the provider.
- Enabling needs a client id and a client secret saved first.
- The redirect URI is built from the dashboard URL, so set it under Settings, General before you register the app.

## Troubleshooting

**Linear says the redirect URI does not match.** Register exactly `https://<dashboard-host>/api/v1/auth/oauth/linear/callback`, with the same scheme and host as your dashboard URL.

**Saving the provider is refused.** Enabling requires a client id and a client secret. Create an OAuth application and add the callback as a redirect URL.

**Sign-in works for others but one person is refused.** The identity must carry a verified email, and the optional allowed email domain on the row must match it.

## FAQ

**Where is the client secret stored?**

Envelope encrypted under the control plane master key. The API only reveals whether one is set.

**Can people link Linear to an existing account?**

Yes, from a signed in session through the link flow. The identity can belong to one user only.

## Related

- [Figma](/integrations/sign-in/figma): Sign-in providers
- [GitHub](/integrations/sign-in/github): Sign-in providers
- [All sign-in providers](/integrations/sign-in)
- [Linear documentation](https://linear.app/developers/oauth-2-0-authentication), maintained by the vendor
