---
title: "Use Amazon Cognito with Levelrail: dashboard sign-in"
sidebarTitle: "Amazon Cognito"
description: "Add a Sign in with Amazon Cognito button to the Levelrail dashboard using an OAuth app you register there."
---

Add a Sign in with Amazon Cognito button to the Levelrail dashboard using an OAuth app you register there.

This page covers how to let people sign in to the dashboard with Amazon Cognito.

## What you get

- A Amazon Cognito button on the sign-in page once the provider is enabled.
- Account linking from a live session, so an existing user can attach their Amazon Cognito identity.
- PKCE, single use state and an encrypted client secret on every flow.

## Set up

1. Prepare the credential. OAuth 2.0 authorization code with PKCE. Register an app at AWS Cognito, your user pool, App integrations, app client (confidential, with a secret).
2. In the dashboard, open **Settings > OAuth sign-in** (`/settings/oauth`), then the Amazon Cognito row.
3. Open the sign-in page in a private window and confirm the new button completes a login.

From the CLI:

```bash
levelrail-cli settings oauth set cognito --client-id <CLIENT_ID> --client-secret <CLIENT_SECRET> --issuer-url <USER_POOL_ID>
levelrail-cli settings oauth list
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Scopes requested | `openid profile email` |
| Who may change it | Root. Settings reads need the read ability. |

## Endpoint and format

Callback URL to register at Amazon Cognito: `https://<your-dashboard-host>/api/v1/auth/oauth/cognito/callback`

## Security notes

- Client secrets and signing keys are encrypted under the control plane master key and never returned by the API.
- Changing sign-in settings needs a root session or token, and refusals are written to the audit log.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- The issuer field holds the user pool id, not a URL. A value that does not look like region_id is refused as an invalid user pool id.
- Sign-in requires a verified email address from the provider.
- Enabling needs a client id and a client secret saved first.
- The redirect URI is built from the dashboard URL, so set it under Settings, General before you register the app.

## Troubleshooting

**Amazon Cognito says the redirect URI does not match.** Register exactly `https://<dashboard-host>/api/v1/auth/oauth/cognito/callback`, with the same scheme and host as your dashboard URL.

**Saving the provider is refused.** Enabling requires a client id and a client secret, and an issuer or instance value for this provider. Put the user pool id, such as us-east-1_AbC123, in the issuer field; the region is read from its prefix.

**Sign-in works for others but one person is refused.** The identity must carry a verified email, and the optional allowed email domain on the row must match it.

## FAQ

**Where is the client secret stored?**

Envelope encrypted under the control plane master key. The API only reveals whether one is set.

**Can people link Amazon Cognito to an existing account?**

Yes, from a signed in session through the link flow. The identity can belong to one user only.

## Related

- [Generic OpenID Connect](/integrations/sign-in/oidc): Sign-in providers
- [AWS EC2](/integrations/cloud-providers/aws): Cloud node providers
- [All sign-in providers](/integrations/sign-in)
- [Amazon Cognito documentation](https://docs.aws.amazon.com/cognito/), maintained by the vendor
