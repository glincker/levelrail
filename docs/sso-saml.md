---
description: Sign in through Okta, Microsoft Entra ID, Keycloak or any SAML 2.0 identity provider, map directory groups to roles, require single sign-on, and keep a break-glass path.
---

# SAML single sign-on

> **Experimental, off by default.** SAML sign-in and [SCIM provisioning](scim.md) sit behind the `saml-scim` beta feature: turn it on under **Settings, Beta features** or with `APP_EXPERIMENTAL=saml-scim`. While off, the routes answer `404`, the settings page and the sign-in button are hidden, and a "require single sign-on" setting is not applied. The service provider is the sign-in library's native SAML implementation; Levelrail adds a stricter response check, account policy and role mapping around it. It is tested against a signing test identity provider, not yet against a live Okta, Entra ID or Keycloak tenant (see [What has not been verified](#what-has-not-been-verified)).

Connect one or more SAML 2.0 identity providers (IdPs). Each enabled connection adds a button to the sign-in page. Directory groups can decide each person's role, and you can require single sign-on for everyone except root users.

<InlineToc default-open />

## Before you start

- **An https dashboard URL.** Set it under **Settings, General**. Identity providers post sign-in responses to it, and the SP entity ID and assertion consumer service (ACS) URL are built from it, never from the request's Host header. When the dashboard URL changes, the connection's URLs follow at its next use.
- **The control plane master key.** The service provider signing key is generated at the first start with a master key and stored encrypted with it. Without one, SAML stays off and the settings page says so.
- **Root.** Every SSO setting needs a root session or token.

## Set up a connection

1. Open **Settings, Single sign-on** and choose **Add SAML connection**. Give it the name people will see on the button and save it.
2. Open **Configure**. Copy the three service provider values into a new SAML app at your IdP:

   | Value | Where it goes |
   | --- | --- |
   | Entity ID (audience) | `https://<dashboard>/api/v1/auth/saml/<id>/metadata` |
   | ACS URL | `https://<dashboard>/api/v1/auth/saml/<id>/acs`, HTTP-POST binding |
   | Metadata URL | Same as the entity ID. Many IdPs can import it directly |

3. Back in the sheet, paste the IdP metadata URL or XML and choose **Import metadata**. That fills the IdP entity ID, sign-in URL and signing certificate. When the metadata lists several signing certificates, the newest one valid today is kept.
4. Pick who may sign in (see [Accounts](#accounts-and-roles)), turn on **Show on the sign-in page** and save.

A connection is saved in two halves: the IdP binding (entity ID, sign-in URL, certificate, attribute names) goes to the sign-in library as soon as it is complete, and the Levelrail half (name, enabled, allowed domains, account creation, default role, role sync) stays in Levelrail. An incomplete connection is kept as a draft and cannot be enabled. Once a connection is complete its IdP fields can be changed but not cleared; delete it and add a new one instead. Deleting a connection also removes every account link made through it.

From the CLI:

```bash
levelrail-cli sso saml create "Okta" --default-role role_viewer --jit
levelrail-cli sso saml import-metadata saml_abc --url https://example.okta.com/app/xyz/sso/saml/metadata
levelrail-cli sso saml update saml_abc --enabled --sync-roles
levelrail-cli sso saml metadata saml_abc > sp-metadata.xml
levelrail-cli sso saml list
```

## What is checked on every sign-in

Sign-in is SP-initiated only: it always starts from the button on the sign-in page. Every response passes two checks, and the browser lands back on the sign-in page with a short error (and the reason goes to the control plane log and the audit log) if either fails.

Levelrail's check, before the library sees the response:

- `InResponseTo` names the AuthnRequest this browser started, and the assertion carries a bearer confirmation for that same request. Each request ID is single use, expires after ten minutes, and is bound to the browser by a secure cookie. This is what refuses IdP-initiated, forwarded and replayed responses: the library on its own accepts IdP-initiated responses and does not check `InResponseTo`.
- The assertion names this connection's entity ID as its audience. An assertion without an audience restriction is refused.
- The response carries a valid XML signature from the stored IdP certificate, the issuer is the configured IdP entity ID, `Destination` and the bearer `Recipient` are this connection's ACS URL, `NotBefore` and `NotOnOrAfter` hold (three minutes of clock skew), and the response and assertion are no older than 90 seconds.

The library's check, which then links the identity and issues the session:

- The same signature, issuer, audience and time checks, plus: the assertion itself must be signed. A response that is signed only on the outer element is refused.

In-flight sign-ins are held in memory, so a control plane restart in the middle of one means starting again from the button.

Encrypted assertions should work (the IdP can encrypt to the service provider certificate shown at the bottom of the settings page), but are not covered by a test yet.

## Accounts and roles

A person is matched in this order:

1. A subject (NameID) already linked to an account through this connection.
2. An existing account with the same email. The subject is linked to it.
3. A new account, only when **Create accounts on first sign-in** (`--jit`) is on. Without it, people need an account first (for example an invite).

Levelrail decides which of these applies and prepares the account before the library runs, then checks that the library signed in exactly that account. If anything after that fails, an account created by this sign-in is removed again, so a refused sign-in never leaves an account or a session behind.

**Allowed email domains** restrict every sign-in, linked or not, to those domains. A disabled account is refused.

### How this maps onto the library

The sign-in library groups SAML connections, SCIM tokens and directory users under organizations. Levelrail runs exactly one library organization, the instance, created at first start. It is the SAML and SCIM tenant and nothing else: Levelrail teams and their roles stay in Levelrail and are not library organizations.

### Group to role mappings

Map IdP groups to stored roles under **Group to role mappings**, or:

```bash
levelrail-cli sso mappings set --map Admins=role_admin:100 --map Engineering=role_operator:10 --map everyone=role_viewer
levelrail-cli sso mappings list
```

The highest priority group the person belongs to decides the role. Group names match case-insensitively. A person in no mapped group gets the connection's default role. With **Apply the mapped role on every sign-in** (`--sync-roles`), the role is re-applied at each sign-in, so a group change at the IdP takes effect at the next one. With sync on and no default role, a person in no mapped group is refused. The last root user is never demoted by a mapping, so directory drift cannot lock the instance out. The same mappings drive [SCIM provisioning](scim.md) groups.

The groups attribute defaults to `http://schemas.xmlsoap.org/claims/Group`, the email attribute to `http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress` and the name attribute to `http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name`. Set the attribute names on the connection when your IdP sends something else (for example `email` and `groups`).

## Require single sign-on

Turn on **Require SSO for non-root users** (or `levelrail-cli sso settings set --enforce=true`). Password, passkey and email code sign-in are then refused for every user without the `root` ability. OAuth and OIDC sign-in stay available, since those are single sign-on too. It can only be turned on while at least one connection is enabled.

### Break-glass access

Root users always keep password sign-in, so a broken IdP never locks the instance. Keep at least one root account with a strong password in your password manager. If that password is lost, reset it on the host:

```bash
sudo APP_DATA_DIR=/var/lib/levelrail-data /usr/local/bin/levelrail recover-admin --username admin
```

See [Emergency access](emergency-access.md) for the host-side recovery routes.

## IdP setup guides

These follow each IdP's documented SAML setup. They have not been run end to end against these IdPs yet.

### Okta

1. **Applications, Create App Integration, SAML 2.0.**
2. **Single sign-on URL:** the ACS URL. Check **Use this for Recipient URL and Destination URL**.
3. **Audience URI (SP Entity ID):** the entity ID.
4. **Name ID format:** EmailAddress. **Application username:** Email.
5. **Attribute statements:** `email` = `user.email`, `displayName` = `user.displayName`. Set the connection's email attribute to `email` and name attribute to `displayName`.
6. **Group attribute statements:** name `groups`, filter `Matches regex` `.*` (or a narrower filter). Set the connection's groups attribute to `groups`.
7. Finish, then on the **Sign On** tab copy the **Metadata URL** and import it.

### Microsoft Entra ID

1. **Enterprise applications, New application, Create your own application**, then **Set up single sign on, SAML**.
2. **Basic SAML Configuration:** Identifier (Entity ID) = the entity ID, Reply URL (ACS) = the ACS URL. Leave Sign on URL empty.
3. **Attributes & Claims:** keep the default email claim, which matches the default email attribute. Add a group claim and set the connection's groups attribute to `http://schemas.microsoft.com/ws/2008/06/identity/claims/groups`. Choose **Cloud-only group display names** to receive names, or put the group object IDs in your mappings.
4. Copy the **App Federation Metadata Url** from the SAML Certificates section and import it.
5. Assign users or groups to the application, or Entra refuses them.

### Keycloak

1. In the realm, **Clients, Create client**, type **SAML**, Client ID = the entity ID.
2. **Valid redirect URIs:** the ACS URL. **Master SAML Processing URL:** the ACS URL.
3. **Keys tab:** turn **Client signature required** off unless you upload the service provider certificate.
4. **Signature and Encryption:** keep **Sign assertions** on. Signing only the document is refused.
5. **Client scopes, dedicated scope, Add mapper:** a **User Property** mapper for `email` (SAML attribute name `email`) and a **Group list** mapper with attribute name `groups` and **Full group path** off. Set the connection's attribute names to match.
6. Import `https://<keycloak>/realms/<realm>/protocol/saml/descriptor`.

## What has not been verified

- Sign-in against a live Okta, Entra ID or Keycloak tenant. The tests sign responses with a local test IdP built on the same SAML library.
- Encrypted assertions.
- An IdP certificate rotation: a connection holds one signing certificate, so the new certificate has to be imported at the moment the IdP switches.

## Troubleshooting

| Sign-in page says | Check |
| --- | --- |
| response could not be verified | The IdP certificate rotated (import the metadata again), the clocks differ by more than three minutes, the IdP signs only the outer response and not the assertion, or it signs for a different entity ID or ACS URL |
| sign-in link expired or was already used | The sign-in took longer than ten minutes, was started in another browser, or the control plane restarted. Start again from the button |
| account has not been set up | Turn on account creation, or invite the person first |
| groups do not grant access | Sync is on, no group matched, and there is no default role |
| restricted to specific email domains | The email domain is not in the allowed list |
| account is disabled | An administrator or the directory disabled the account |

The control plane log and the audit log (`sso.saml.rejected`) record the exact reason. Assertion contents are never logged.

## API

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/auth/saml/providers` | public |
| `GET` | `/api/v1/auth/saml/{id}/metadata` | public |
| `GET` | `/api/v1/auth/saml/{id}/login` | public |
| `POST` | `/api/v1/auth/saml/{id}/acs` | public, signed assertion plus browser-bound request |
| `GET`, `POST` | `/api/v1/sso/saml/connections` | `root` |
| `GET`, `PUT`, `DELETE` | `/api/v1/sso/saml/connections/{id}` | `root` |
| `POST` | `/api/v1/sso/saml/connections/{id}/metadata` | `root` |
| `GET`, `PUT` | `/api/v1/sso/group-mappings` | `root` |
| `GET`, `PUT` | `/api/v1/sso/settings` | `root` |

The sign-in library's own SAML and organization routes are not exposed; only the routes above reach it.

| Variable | Default | Purpose |
| --- | --- | --- |
| `APP_SAML_RATE_PER_MINUTE` | `30` | SAML sign-in starts and responses accepted per client IP per minute |
