---
title: "Use Slack with Levelrail: dashboard sign-in"
sidebarTitle: "Slack"
description: "Add a Sign in with Slack button to the Levelrail dashboard using an OAuth app you register there."
---

Add a Sign in with Slack button to the Levelrail dashboard using an OAuth app you register there.

This page covers how to let people sign in to the dashboard with Slack.

## What you get

- A Slack button on the sign-in page once the provider is enabled.
- Account linking from a live session, so an existing user can attach their Slack identity.
- PKCE, single use state and an encrypted client secret on every flow.

## Set up

1. Prepare the credential. OAuth 2.0 authorization code with PKCE. Register an app at api.slack.com/apps, your app, OAuth and Permissions (Sign in with Slack).
2. In the dashboard, open **Settings > OAuth sign-in** (`/settings/oauth`), then the Slack row.
3. Open the sign-in page in a private window and confirm the new button completes a login.

From the CLI:

```bash
levelrail-cli settings oauth set slack --client-id <CLIENT_ID> --client-secret <CLIENT_SECRET>
levelrail-cli settings oauth list
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Scopes requested | `openid email profile` |
| Who may change it | Root. Settings reads need the read ability. |

## Endpoint and format

Callback URL to register at Slack: `https://<your-dashboard-host>/api/v1/auth/oauth/slack/callback`

## Security notes

- Client secrets and signing keys are encrypted under the control plane master key and never returned by the API.
- Changing sign-in settings needs a root session or token, and refusals are written to the audit log.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Sign-in requires a verified email address from the provider.
- Enabling needs a client id and a client secret saved first.
- The redirect URI is built from the dashboard URL, so set it under Settings, General before you register the app.

## Troubleshooting

**Slack says the redirect URI does not match.** Register exactly `https://<dashboard-host>/api/v1/auth/oauth/slack/callback`, with the same scheme and host as your dashboard URL.

**Saving the provider is refused.** Enabling requires a client id and a client secret. Use the Sign in with Slack flow of your app, and add the callback as a redirect URL.

**Sign-in works for others but one person is refused.** The identity must carry a verified email, and the optional allowed email domain on the row must match it.

## FAQ

**Where is the client secret stored?**

Envelope encrypted under the control plane master key. The API only reveals whether one is set.

**Can people link Slack to an existing account?**

Yes, from a signed in session through the link flow. The identity can belong to one user only.

## Related

- [Discord](/integrations/sign-in/discord): Sign-in providers
- [Slack](/integrations/notifications/slack): Notification channels
- [All sign-in providers](/integrations/sign-in)
- [Slack documentation](https://api.slack.com/authentication/sign-in-with-slack), maintained by the vendor
