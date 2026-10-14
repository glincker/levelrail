---
title: "Use SAML 2.0 with Levelrail: single sign-on"
sidebarTitle: "SAML 2.0"
description: "Connect one or more SAML 2.0 identity providers, map directory groups to roles and optionally require single sign-on for non-root users."
---

Connect one or more SAML 2.0 identity providers, map directory groups to roles and optionally require single sign-on for non-root users.

This page covers how to single sign-on with Okta, Entra ID, Keycloak or any SAML identity provider.

## What you get

- A sign-in button per enabled connection, with SP initiated login only.
- Group to role mappings with priorities, re-applied on each sign-in when sync is on.
- Require SSO for non-root users while root keeps password access as break-glass.

## Set up

1. Prepare the credential. SAML 2.0 signed responses, validated against the stored IdP certificates. Encrypted assertions are supported.
2. In the dashboard, open **Settings > Single sign-on** (`/settings/sso`), then Add SAML connection.
3. Open the sign-in page in a private window and confirm the new button completes a login.

From the CLI:

```bash
levelrail-cli sso saml create "Okta" --default-role role_viewer --jit
levelrail-cli sso saml import-metadata <connection-id> --url <IDP_METADATA_URL>
levelrail-cli sso saml update <connection-id> --enabled --sync-roles
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Root | Every SSO setting needs a root session or token. |
| Dashboard URL | An https dashboard URL must be set; entity ID and ACS URL are built from it, never from the Host header. |
| Master key | The service provider signing key is stored encrypted with it. |

## Endpoint and format

Entity ID and metadata URL: `https://<dashboard>/api/v1/auth/saml/<id>/metadata`. ACS URL (HTTP-POST binding): `https://<dashboard>/api/v1/auth/saml/<id>/acs`.

## Security notes

- Client secrets and signing keys are encrypted under the control plane master key and never returned by the API.
- Changing sign-in settings needs a root session or token, and refusals are written to the audit log.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Interim hand built implementation, behind the saml-scim beta feature and off by default. While off its routes answer 404.
- Sign-in is SP initiated only: IdP initiated responses are refused.
- Clock skew tolerance is three minutes and a response must be no older than 90 seconds.

## Troubleshooting

**Response could not be verified.** The IdP certificate rotated (import the metadata again), clocks differ by more than three minutes, or the IdP signs for a different entity ID or ACS URL.

**Account has not been set up.** Turn on account creation (--jit) or invite the person first. Without it, SAML only signs in people who already have an account.

**Groups do not grant access.** Sync is on, no group matched and there is no default role. Add a mapping or set a default role.

## FAQ

**Does it work with Okta, Entra ID and Keycloak?**

Those three have step by step setup guides in the SAML documentation, and any standards compliant SAML 2.0 IdP can be imported by metadata.

**What if the IdP is down?**

Root users always keep password sign-in. If the root password is lost, reset it on the host with the recover-admin command.

## Related

- [SCIM 2.0](/integrations/sign-in/scim): Sign-in providers
- [Generic OpenID Connect](/integrations/sign-in/oidc): Sign-in providers
- [All sign-in providers](/integrations/sign-in)
- [SAML 2.0 documentation](https://www.oasis-open.org/standard/saml/), maintained by the vendor
