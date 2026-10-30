---
title: "OAuth and OIDC single sign-on"
description: "Map identity provider groups to roles, create accounts on first sign-in only when allowed, restrict email domains and route users by email domain."
"og:url": "https://docs.levelrail.com/sso-oidc"
---

> **Beta, off by default.** The sign-in policy below shares the `saml-scim` beta feature with [SAML](sso-saml.md) and [SCIM](scim.md): turn it on under **Settings, Beta features** or with `APP_EXPERIMENTAL=saml-scim`. While it is off, the policy settings page and routes answer `404` and the sign-in page shows no "Continue with SSO" button, but the policy you saved is still enforced at sign-in. The generic OpenID Connect provider has been run against Keycloak 26.0.8 (authorization code with PKCE) and is also covered by tests against a fake provider that signs real tokens. Okta, Microsoft Entra ID, Google and Cognito have not been tested (see [What has not been verified](#what-has-not-been-verified)).

OAuth and OpenID Connect providers (enabled under **Settings, OAuth sign-in**, see [Identity and access](identity-and-access.md#oauth-sign-in)) get the same controls SAML connections have. For each provider you decide who may sign in, whether a first sign-in creates an account, which role each group earns and whether the role follows the group on later sign-ins.

<InlineToc default-open />

## The defaults are least privilege

| Setting | Default | Meaning |
| --- | --- | --- |
| Create an account on first sign-in (`--jit`) | off | Someone with no account here is refused with `sso_not_provisioned` unless you invited them first. |
| Reapply the mapped role on every sign-in (`--sync-roles`) | off | The role is set when the account is created and left alone afterwards. |
| Default role (`--default-role`) | `role_viewer` | Given to people none of the group mappings match. A role that holds root is refused. |
| Groups claim (`--groups-claim`) | `groups` | The ID token claim that lists group names. Dotted paths such as `realm_access.roles` work. |
| Allowed email domains (`--allowed-domains`) | none | Empty allows any domain. A list restricts every sign-in and turns on domain routing. |

Providers that were already enabled when you upgraded keep their old behavior for new emails (they create accounts, as before). Turn **Create an account on first sign-in** off to close that. A provider you enable after upgrading starts with it off.

## Setup

1. Enable the provider under **Settings, OAuth sign-in** with its client ID and secret. For a provider that sends groups, use the generic **OpenID Connect** provider (see the table below).
2. Open **Settings, Single sign-on**. The provider is listed under **OAuth and OIDC sign-in**. Choose **Configure**.
3. Set the policy, then map groups to roles in the shared **Group to role mappings** card on the same page.
4. Choose **Test login**. You sign in at the provider and land back on the page with what the sign-in would do: the groups the provider sent, the role they earn, and whether the account would be linked, created or refused. A test login creates no account, role or session. Each failure comes with a typed fix: one click for a wrong claim name, a missing domain or account creation being off, and a plain instruction when the fix is at your identity provider.

From the CLI:

```bash
levelrail-cli sso oidc list
levelrail-cli sso oidc set oidc --jit=true --sync-roles=true --default-role role_viewer --groups-claim groups --allowed-domains example.com
levelrail-cli sso mappings set --map Engineering=role_operator:10 --map everyone=role_viewer
levelrail-cli sso oidc evaluate oidc --email dev@example.com --email-verified --groups Engineering
levelrail-cli sso oidc test oidc
```

`sso oidc test` prints the address that runs **Test login** in a browser signed in as root, since the provider's login page needs a browser.

`sso oidc evaluate` runs the same rules as a real sign-in on the facts you give it and changes nothing. Every command takes `--json`.

### Providers

| Provider family | Groups | How |
| --- | --- | --- |
| Generic OpenID Connect (Keycloak, Okta, Microsoft Entra ID, Auth0, Authentik, Zitadel) | Yes, from the signed ID token | Add a groups claim to the ID token at the provider, then name it in **Groups claim**. Keycloak: a "Group Membership" mapper (claim `groups`) or `realm_access.roles`. Okta: a `groups` claim on the authorization server. Entra ID: "Groups claim" under Token configuration (group object IDs, so map those). |
| Amazon Cognito | Yes, claim `cognito:groups` | Set **Groups claim** to `cognito:groups`. |
| Google | No | Google's ID token has no group claim. Use domain restriction and the default role, or sign in through a provider that sends groups. |
| GitHub, GitLab, Bitbucket, Discord, Slack, LinkedIn and the other named providers | No | They return no verified ID token to read groups from. Everyone gets the default role. GitHub organization and team membership is not supported. |
| Microsoft (named provider) | No | It reads Microsoft Graph, not the ID token. For groups use the generic OpenID Connect provider with your Entra tenant as the issuer. |

## Safety rules

Every rule is enforced in code and covered by a test.

| Rule | What happens |
| --- | --- |
| Group claims come only from a verified ID token | Groups are read from an ID token whose signature, issuer, audience, nonce and expiry the sign-in library already verified. A tampered token fails the sign-in. Groups in the userinfo response, the access token or any other field are ignored. A bare string claim is one group, never split on commas. |
| Email must be verified | A provider that does not mark the email verified is refused (`userinfo_failed`). Microsoft Graph, Facebook, Linear, Figma and Codeberg confirm an address before they return it, so those count as verified for creating an account. |
| No linking by email alone | An identity links to an existing account by email only when the provider marks that email verified, the same rule SAML uses: the identity provider is one you configured and trust. A provider that does not mark emails verified never links to an existing account (`email_in_use`). |
| Claims never grant root | A group mapping or default role that holds root is refused when you save it, and refused again at sign-in (`sso_no_role`) if one got into the table another way. Only a person with root can grant root. |
| Root users are never changed by sync | A user who holds root keeps their role at every sign-in, whatever the groups say. |
| Allowed domains apply to everyone | A sign-in from another domain is refused (`domain_not_allowed`), including people who signed in before the list was set. |
| Downgrades are audited | A role change by role sync writes `sso.oidc.role_changed` with the old role, the new role and `direction=downgrade`, `upgrade` or `change`. |
| Half-done work is undone | If the role cannot be assigned, the session is revoked and an account this sign-in created is removed, so a retry starts clean. |
| Every outcome is audited | `sso.oidc.signed_in`, `sso.oidc.user_provisioned`, `sso.oidc.role_changed` and `sso.oidc.rejected` (with the short reason code) carry the provider name. No token, claim or email is written to the log. |
| Test logins create nothing | The test callback stops after the provider answers. Its cookie is bound to the root session that started it, expires in ten minutes and is spent on first use. |

The people you want to let in without account creation still work the usual way: invite them, or create the account, and they link at their first sign-in when the provider marks their email verified.

## Sign in by email domain

When allowed domains are set, the sign-in page's **Continue with SSO** button asks for a work email and sends the person to the provider (or [SAML connection](sso-saml.md)) that claims that domain. Only the part after the `@` leaves the browser, so the lookup says nothing about any person. A domain no connection claims, a malformed one and a rate limited request all get the same empty answer, and the page shows one neutral message. Two connections cannot claim one domain: saving the second is refused with `409`, and if both somehow exist the domain routes nowhere. The lookup is rate limited per client address (`APP_SSO_ROUTE_RATE_PER_MINUTE`, default 60).

That a domain has single sign-on at all is visible to anyone who tries it, as it is for any identity provider's home realm discovery.

## API

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/sso/oidc` | `root` |
| `GET`, `PUT` | `/api/v1/sso/oidc/{provider}` | `root` |
| `POST` | `/api/v1/sso/oidc/{provider}/evaluate` | `root` |
| `GET` | `/api/v1/sso/oidc/{provider}/test/start`, `/test/result` | `root`, needs a dashboard session |
| `GET` | `/api/v1/auth/login-options?sso_domain=example.com` | public, rate limited |

The policy responses carry no client secret or token. Client credentials stay on `PUT /api/v1/settings/oauth/{provider}`, where the secret is write-only.

## What is not supported

- **Group sync from userinfo, the access token or provider APIs** (GitHub teams, Google Workspace groups, Microsoft Graph groups). Only a groups claim in the verified ID token is read.
- **Several OIDC connections per provider.** There is one generic OpenID Connect provider. Use SAML for a second identity provider of the same kind.
- **Per-connection group mappings.** The mapping table is shared by SAML, OIDC and SCIM.
- **Required SSO per domain.** [Require SSO](sso-saml.md#require-single-sign-on) is instance wide.
- **Deprovisioning.** Removing someone at the identity provider does not end their session or disable the account here. Use [SCIM](scim.md) for that, or disable the account.

## Tested against

Last run on 2026-10-20 against Keycloak 26.0.8 in a local Docker lab (`test/sso-lab/`), driven by a scripted HTTP client that follows the provider's login form (no browser). The test is `internal/api/sso_oidc_live_test.go`, skipped under `-short` and run only with `LEVELRAIL_LIVE_IDP=1`:

```bash
scripts/sso-lab.sh up
export LEVELRAIL_LIVE_IDP=1
CGO_ENABLED=0 go test ./internal/api -run 'TestLiveOIDC' -count=1 -v
scripts/sso-lab.sh down
```

Verified against Keycloak, with the client requiring PKCE `S256`:

- The authorization code flow with PKCE signs in, through the generic OpenID Connect provider discovered from the issuer URL.
- A first sign-in creates the account when account creation is on, applies the mapped role from the `groups` claim (Keycloak's group membership mapper), and a second sign-in links the same account.
- An email outside the allowed domains is refused (`domain_not_allowed`) and leaves no account.
- With account creation off, a person with no account is refused (`sso_not_provisioned`).
- A group mapped to a root-holding role is refused when saved through the API, and refused at sign-in (`sso_no_role`) when forced into the table, so SSO never grants root.

## What has not been verified

- Okta, Microsoft Entra ID, Google Workspace and Amazon Cognito: no tenant was available, so their claim names and token shapes are documented from their specifications, not observed.
- Keycloak was tested with plain http on a loopback address (the control plane's private-address guard was relaxed for the test). Discovery against an https issuer on a public address is covered by the same code path as other providers but was not run against Keycloak.
- A live Test login in the browser was not driven end to end by a script: the dry-run callback and its evaluation are covered by unit tests against the fake provider.
