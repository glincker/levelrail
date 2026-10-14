---
description: Sign in through Okta, Microsoft Entra ID, Keycloak or any SAML 2.0 identity provider, map directory groups to roles, require single sign-on, and keep a break-glass path.
---

# SAML single sign-on

> **Interim implementation, off by default.** This SAML and SCIM support is hand built for now and sits behind the `saml-scim` beta feature: turn it on under **Settings, Beta features** or with `APP_EXPERIMENTAL=saml-scim`. While off, its routes answer `404`, the settings page and the sign-in button are hidden, and an earlier "require single sign-on" setting is not applied. It will move onto the sign-in library's native SAML and SCIM once its SQLite storage supports them.

Connect one or more SAML 2.0 identity providers (IdPs). Each enabled connection adds a button to the sign-in page. Directory groups can decide each person's role, and you can require single sign-on for everyone except root users.

<InlineToc default-open />

## Before you start

- **An https dashboard URL.** Set it under **Settings, General**. Identity providers post sign-in responses to it, and the SP entity ID and assertion consumer service (ACS) URL are built from it, never from the request's Host header.
- **The control plane master key.** The service provider signing key is generated on first use and stored encrypted with it.
- **Root.** Every SSO setting needs a root session or token.

## Set up a connection

1. Open **Settings, Single sign-on** and choose **Add SAML connection**. Give it the name people will see on the button and save it.
2. Open **Configure**. Copy the three service provider values into a new SAML app at your IdP:

   | Value | Where it goes |
   | --- | --- |
   | Entity ID (audience) | `https://<dashboard>/api/v1/auth/saml/<id>/metadata` |
   | ACS URL | `https://<dashboard>/api/v1/auth/saml/<id>/acs`, HTTP-POST binding |
   | Metadata URL | Same as the entity ID. Many IdPs can import it directly |

3. Back in the sheet, paste the IdP metadata URL or XML and choose **Import metadata**. That fills the IdP entity ID, sign-in URL and signing certificates.
4. Pick who may sign in (see [Accounts](#accounts-and-roles)), turn on **Show on the sign-in page** and save.

From the CLI:

```bash
levelrail-cli sso saml create "Okta" --default-role role_viewer --jit
levelrail-cli sso saml import-metadata saml_abc --url https://example.okta.com/app/xyz/sso/saml/metadata
levelrail-cli sso saml update saml_abc --enabled --sync-roles
levelrail-cli sso saml metadata saml_abc > sp-metadata.xml
levelrail-cli sso saml list
```

## What is checked on every sign-in

Sign-in is SP-initiated only: it always starts from the button on the sign-in page. A response is accepted only when all of these hold, otherwise the browser lands back on the sign-in page with a short error and the refusal is written to the audit log.

- The response or the assertion carries a valid XML signature from one of the stored IdP certificates.
- The issuer is the configured IdP entity ID.
- `Destination`, the bearer `Recipient` and the audience are exactly this connection's ACS URL and entity ID. An assertion without an audience restriction is refused.
- `NotBefore` and `NotOnOrAfter` hold (three minutes of clock skew), and the response is no older than 90 seconds.
- `InResponseTo` names the AuthnRequest this browser started. Each request ID is single use, expires after ten minutes, and is bound to the browser by a secure cookie, so an IdP-initiated or forwarded response is refused.
- The assertion ID has not been seen before. A replayed assertion is refused even with a fresh request.

Encrypted assertions are supported: the IdP can encrypt to the service provider certificate shown at the bottom of the settings page.

## Accounts and roles

A person is matched in this order:

1. A subject (NameID) already linked to an account through this connection.
2. An existing account with the same email. The subject is linked to it.
3. A new account, only when **Create accounts on first sign-in** (`--jit`) is on. Without it, people need an account first (for example an invite).

**Allowed email domains** restrict every sign-in, linked or not, to those domains.

### Group to role mappings

Map IdP groups to stored roles under **Group to role mappings**, or:

```bash
levelrail-cli sso mappings set --map Admins=role_admin:100 --map Engineering=role_operator:10 --map everyone=role_viewer
levelrail-cli sso mappings list
```

The highest priority group the person belongs to decides the role. Group names match case-insensitively. A person in no mapped group gets the connection's default role. With **Apply the mapped role on every sign-in** (`--sync-roles`), the role is re-applied at each sign-in, so a group change at the IdP takes effect at the next one. With sync on and no default role, a person in no mapped group is refused. The last root user is never demoted by a mapping, so directory drift cannot lock the instance out. The same mappings drive [SCIM provisioning](scim.md) groups.

## Require single sign-on

Turn on **Require SSO for non-root users** (or `levelrail-cli sso settings set --enforce=true`). Password, passkey and email code sign-in are then refused for every user without the `root` ability. OAuth and OIDC sign-in stay available, since those are single sign-on too. It can only be turned on while at least one connection is enabled.

### Break-glass access

Root users always keep password sign-in, so a broken IdP never locks the instance. Keep at least one root account with a strong password in your password manager. If that password is lost, reset it on the host:

```bash
sudo APP_DATA_DIR=/var/lib/levelrail-data /usr/local/bin/levelrail recover-admin --username admin
```

See [Emergency access](emergency-access.md) for the host-side recovery routes.

## IdP setup guides

### Okta

1. **Applications, Create App Integration, SAML 2.0.**
2. **Single sign-on URL:** the ACS URL. Check **Use this for Recipient URL and Destination URL**.
3. **Audience URI (SP Entity ID):** the entity ID.
4. **Name ID format:** EmailAddress. **Application username:** Email.
5. **Attribute statements:** `email` = `user.email`, `displayName` = `user.displayName`.
6. **Group attribute statements:** name `groups`, filter `Matches regex` `.*` (or a narrower filter).
7. Finish, then on the **Sign On** tab copy the **Metadata URL** and import it.

### Microsoft Entra ID

1. **Enterprise applications, New application, Create your own application**, then **Set up single sign on, SAML**.
2. **Basic SAML Configuration:** Identifier (Entity ID) = the entity ID, Reply URL (ACS) = the ACS URL. Leave Sign on URL empty.
3. **Attributes & Claims:** keep the default email claim. Add a group claim (**Groups assigned to the application** is usually right). Choose **Cloud-only group display names** to receive names, or put the group object IDs in your mappings.
4. Copy the **App Federation Metadata Url** from the SAML Certificates section and import it.
5. Assign users or groups to the application, or Entra refuses them.

The default Entra claim names (`http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress` and `http://schemas.microsoft.com/ws/2008/06/identity/claims/groups`) are recognised without configuring attribute names.

### Keycloak

1. In the realm, **Clients, Create client**, type **SAML**, Client ID = the entity ID.
2. **Valid redirect URIs:** the ACS URL. **Master SAML Processing URL:** the ACS URL.
3. **Keys tab:** turn **Client signature required** off unless you upload the service provider certificate.
4. **Signature and Encryption:** keep **Sign documents** or **Sign assertions** on.
5. **Client scopes, dedicated scope, Add mapper:** a **User Property** mapper for `email` (SAML attribute name `email`) and a **Group list** mapper with attribute name `groups` and **Full group path** off.
6. Import `https://<keycloak>/realms/<realm>/protocol/saml/descriptor`.

## Troubleshooting

| Sign-in page says | Check |
| --- | --- |
| response could not be verified | The IdP certificate rotated (import the metadata again), the clocks differ by more than three minutes, or the IdP signs for a different entity ID or ACS URL |
| sign-in link expired or was already used | The sign-in took longer than ten minutes, or was started in another browser. Start again from the button |
| account has not been set up | Turn on account creation, or invite the person first |
| groups do not grant access | Sync is on, no group matched, and there is no default role |
| restricted to specific email domains | The email domain is not in the allowed list |

The control plane log and the audit log (`sso.saml.rejected`) record the exact reason.

## API

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/auth/saml/providers` | public |
| `GET` | `/api/v1/auth/saml/{id}/metadata` | public |
| `GET` | `/api/v1/auth/saml/{id}/login` | public |
| `POST` | `/api/v1/auth/saml/{id}/acs` | public, signed assertion |
| `GET`, `POST` | `/api/v1/sso/saml/connections` | `root` |
| `GET`, `PUT`, `DELETE` | `/api/v1/sso/saml/connections/{id}` | `root` |
| `POST` | `/api/v1/sso/saml/connections/{id}/metadata` | `root` |
| `GET`, `PUT` | `/api/v1/sso/group-mappings` | `root` |
| `GET`, `PUT` | `/api/v1/sso/settings` | `root` |

| Variable | Default | Purpose |
| --- | --- | --- |
| `APP_SAML_RATE_PER_MINUTE` | `30` | SAML sign-in starts and responses accepted per client IP per minute |
