---
title: "Use Generic OpenID Connect with Levelrail: dashboard sign-in"
sidebarTitle: "Generic OpenID Connect"
description: "Add a Sign in with Generic OpenID Connect button to the Levelrail dashboard using an OAuth app you register there. Works with any OpenID Connect provider."
---

Add a Sign in with Generic OpenID Connect button to the Levelrail dashboard using an OAuth app you register there. Works with any OpenID Connect provider.

This page covers how to let people sign in to the dashboard with Generic OpenID Connect.

## What you get

- A Generic OpenID Connect button on the sign-in page once the provider is enabled.
- Account linking from a live session, so an existing user can attach their Generic OpenID Connect identity.
- PKCE, single use state and an encrypted client secret on every flow.

## Set up

1. Prepare the credential. OAuth 2.0 authorization code with PKCE. Register an app at your identity provider's admin console (Keycloak, Authentik, Okta and similar).
2. In the dashboard, open **Settings > OAuth sign-in** (`/settings/oauth`), then the Generic OpenID Connect row.
3. Open the sign-in page in a private window and confirm the new button completes a login.

From the CLI:

```bash
levelrail-cli settings oauth set oidc --client-id <CLIENT_ID> --client-secret <CLIENT_SECRET> --issuer-url <ISSUER_URL>
levelrail-cli settings oauth list
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Scopes requested | `openid email profile` |
| Who may change it | Root. Settings reads need the read ability. |

## Endpoint and format

Callback URL to register at Generic OpenID Connect: `https://<your-dashboard-host>/api/v1/auth/oauth/oidc/callback`

## Security notes

- Client secrets and signing keys are encrypted under the control plane master key and never returned by the API.
- Changing sign-in settings needs a root session or token, and refusals are written to the audit log.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Needs an issuer URL that supports discovery. Provider specific behaviors such as group mapping use SAML, not this row.
- Sign-in requires a verified email address from the provider.
- Enabling needs a client id and a client secret saved first.
- The redirect URI is built from the dashboard URL, so set it under Settings, General before you register the app.

## Troubleshooting

**Generic OpenID Connect says the redirect URI does not match.** Register exactly `https://<dashboard-host>/api/v1/auth/oauth/oidc/callback`, with the same scheme and host as your dashboard URL.

**Saving the provider is refused.** Enabling requires a client id and a client secret, and an issuer or instance value for this provider. The issuer URL must serve OpenID discovery, and plain http is refused unless insecure OIDC is allowed.

**Sign-in works for others but one person is refused.** The identity must carry a verified email, and the optional allowed email domain on the row must match it.

## FAQ

**Where is the client secret stored?**

Envelope encrypted under the control plane master key. The API only reveals whether one is set.

**Can people link Generic OpenID Connect to an existing account?**

Yes, from a signed in session through the link flow. The identity can belong to one user only.

## Related

- [SAML 2.0](/integrations/sign-in/saml): Sign-in providers
- [Amazon Cognito](/integrations/sign-in/cognito): Sign-in providers
- [All sign-in providers](/integrations/sign-in)
- [Generic OpenID Connect documentation](https://openid.net/specs/openid-connect-core-1_0.html), maintained by the vendor
