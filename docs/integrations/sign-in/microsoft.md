---
title: "Use Microsoft Entra ID with Levelrail: dashboard sign-in"
sidebarTitle: "Microsoft Entra ID"
description: "Add a Sign in with Microsoft Entra ID button to the Levelrail dashboard using an OAuth app you register there."
---

Add a Sign in with Microsoft Entra ID button to the Levelrail dashboard using an OAuth app you register there.

This page covers how to let people sign in to the dashboard with Microsoft Entra ID.

## What you get

- A Microsoft Entra ID button on the sign-in page once the provider is enabled.
- Account linking from a live session, so an existing user can attach their Microsoft Entra ID identity.
- PKCE, single use state and an encrypted client secret on every flow.

## Set up

1. Prepare the credential. OAuth 2.0 authorization code with PKCE. Register an app at the Microsoft Entra admin center under App registrations.
2. In the dashboard, open **Settings > OAuth sign-in** (`/settings/oauth`), then the Microsoft Entra ID row.
3. Open the sign-in page in a private window and confirm the new button completes a login.

From the CLI:

```bash
levelrail-cli settings oauth set microsoft --client-id <CLIENT_ID> --client-secret <CLIENT_SECRET>
levelrail-cli settings oauth list
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Scopes requested | `openid email profile https://graph.microsoft.com/User.Read` |
| Who may change it | Root. Settings reads need the read ability. |

## Endpoint and format

Callback URL to register at Microsoft Entra ID: `https://<your-dashboard-host>/api/v1/auth/oauth/microsoft/callback`

## Security notes

- Client secrets and signing keys are encrypted under the control plane master key and never returned by the API.
- Changing sign-in settings needs a root session or token, and refusals are written to the audit log.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- It talks to the `common` tenant endpoint. For a single directory with group mapping, use SAML instead.
- Sign-in requires a verified email address from the provider.
- Enabling needs a client id and a client secret saved first.
- The redirect URI is built from the dashboard URL, so set it under Settings, General before you register the app.

## Troubleshooting

**Microsoft Entra ID says the redirect URI does not match.** Register exactly `https://<dashboard-host>/api/v1/auth/oauth/microsoft/callback`, with the same scheme and host as your dashboard URL.

**Saving the provider is refused.** Enabling requires a client id and a client secret. The provider uses the common multi tenant endpoint, so set the app registration to accept the account types you expect.

**Sign-in works for others but one person is refused.** The identity must carry a verified email, and the optional allowed email domain on the row must match it.

## FAQ

**Where is the client secret stored?**

Envelope encrypted under the control plane master key. The API only reveals whether one is set.

**Can people link Microsoft Entra ID to an existing account?**

Yes, from a signed in session through the link flow. The identity can belong to one user only.

## Related

- [SAML 2.0](/integrations/sign-in/saml): Sign-in providers
- [Google](/integrations/sign-in/google): Sign-in providers
- [All sign-in providers](/integrations/sign-in)
- [Microsoft Entra ID documentation](https://learn.microsoft.com/entra/identity-platform/), maintained by the vendor
