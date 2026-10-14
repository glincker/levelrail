---
description: Provision and deprovision accounts and groups from Okta, Microsoft Entra ID or any SCIM 2.0 client, with hashed bearer tokens, immediate session revocation and audited changes.
---

# SCIM provisioning

SCIM 2.0 lets your identity provider create accounts, keep their profile up to date, put them in groups and deactivate them. Group membership drives roles through the same [group to role mappings](sso-saml.md#group-to-role-mappings) SAML uses. People still sign in with [SAML](sso-saml.md), OAuth or a password.

<InlineToc default-open />

## Set up

1. Open **Settings, Single sign-on**, section **Directory sync (SCIM)**.
2. Copy the **SCIM base URL**: `https://<dashboard>/api/v1/scim/v2`. SCIM only answers over https.
3. Create a token, give it a name like `okta-provisioning`, and copy it. It is shown once and only its SHA-256 hash is stored.
4. Paste the base URL and the token (HTTP header, bearer) into your identity provider.
5. Pick the **Role for new directory users**. It applies when no group mapping matches.

```bash
levelrail-cli sso scim tokens create okta-provisioning
levelrail-cli sso scim tokens list
levelrail-cli sso scim tokens rotate scimtok_abc
levelrail-cli sso scim tokens revoke scimtok_abc
levelrail-cli sso settings set --scim-default-role role_viewer
```

**Rotate** issues a new token with the same name and revokes the old one in the same step, so update the IdP right after.

## What the directory can do

| Endpoint | Supported |
| --- | --- |
| `/Users` | list, get, create, replace (`PUT`), `PATCH`, delete |
| `/Groups` | list, get, create, replace, `PATCH` members and name, delete |
| `/ServiceProviderConfig`, `/ResourceTypes` | get |

Filters: `attribute eq "value"` with `userName`, `externalId`, `emails.value` or `id` on users, and `displayName`, `externalId` or `id` on groups. Paging uses `startIndex` and `count` (at most 200). Bulk, sorting and ETags are not supported.

Every existing account is visible under `/Users`, so an IdP can match and adopt accounts created before provisioning was turned on.

## Deprovisioning

Setting `active` to false (or deleting the user) takes effect immediately:

- The account is marked disabled first, so every session and API token of it is refused from that moment, even while revocation is still running.
- All sessions are ended and trusted browsers are forgotten.
- The change is written to the audit log as `scim.user.deactivated` or `scim.user.deleted`, with the SCIM token as the actor.

A deleted user disappears from `/Users` but the account is kept, disabled, so history and ownership stay intact. Creating the same email again through SCIM restores it. Setting `active` back to true re-enables the account; the person signs in again.

The last user holding `root` can never be deactivated or deleted through SCIM (the request answers 409), so directory drift cannot lock every administrator out. Grant root to someone else first.

## Groups and roles

When a group's members change, or a group is deleted, everyone affected gets the role of their highest priority mapped group, or the default directory role when none matches. With no mappings configured, SCIM never changes roles and you manage them by hand. Profile email changes at the IdP do not rename the account here; the account keeps the email it was created with.

## IdP guides

### Okta

1. In the app you created for SAML (or a new one), **General, App Settings, Provisioning: SCIM**.
2. **Provisioning, Integration:** SCIM connector base URL = the SCIM base URL, unique identifier field = `userName`, actions: Push New Users, Push Profile Updates, Push Groups. Authentication mode **HTTP Header**, paste the token.
3. **Provisioning, To App:** enable Create Users, Update User Attributes and Deactivate Users.
4. **Push Groups** for the groups you mapped to roles.

### Microsoft Entra ID

1. In the enterprise application, **Provisioning, Get started**, mode **Automatic**.
2. **Tenant URL** = the SCIM base URL, **Secret Token** = the token. **Test Connection**, then save.
3. Under **Mappings**, keep the default user and group mappings. Entra sends `active` as the strings `"True"` and `"False"`, which is accepted.
4. Turn **Provisioning Status** on. Entra syncs every 40 minutes; use **Provision on demand** to test one user.

### Keycloak

Keycloak has no built-in SCIM client. Use a SCIM provider extension for Keycloak, or keep Keycloak for SAML sign-in only and turn on account creation on the SAML connection.

## API

| Method | Path | Ability |
| --- | --- | --- |
| `GET`, `POST` | `/api/v1/sso/scim/tokens` | `root` |
| `POST` | `/api/v1/sso/scim/tokens/{id}/rotate` | `root` |
| `DELETE` | `/api/v1/sso/scim/tokens/{id}` | `root` |
| any | `/api/v1/scim/v2/...` | SCIM bearer token, https only |

| Variable | Default | Purpose |
| --- | --- | --- |
| `APP_SCIM_AUTH_FAILURES_PER_MINUTE` | `30` | Failed SCIM authentications per client IP per minute before 429 |
