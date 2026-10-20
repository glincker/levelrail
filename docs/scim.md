---
title: "SCIM provisioning"
description: "Provision and deprovision accounts and groups from Okta, Microsoft Entra ID or any SCIM 2.0 client, with hashed bearer tokens, instant revocation and audit."
"og:url": "https://docs.levelrail.com/scim"
---

> **Beta, off by default.** SCIM provisioning and [SAML sign-in](sso-saml.md) sit behind the `saml-scim` beta feature: turn it on under **Settings, Beta features** or with `APP_EXPERIMENTAL=saml-scim`. While off, the routes answer `404` and the settings are hidden. The SCIM 2.0 endpoints run on the sign-in library; Levelrail authenticates the request first, then mirrors each change onto Levelrail accounts and roles. It is tested with a SCIM client in the test suite, not yet against a live Okta or Entra ID tenant (see [What has not been verified](#what-has-not-been-verified)).

SCIM 2.0 lets your identity provider create accounts, keep their profile up to date, put them in groups and deactivate them. Group membership drives roles through the same [group to role mappings](sso-saml.md#group-to-role-mappings) SAML uses. People still sign in with [SAML](sso-saml.md), OAuth or a password.

<InlineToc default-open />

## Set up

1. Open **Settings, Single sign-on**, section **Directory sync (SCIM)**.
2. Copy the **SCIM base URL**: `https://<dashboard>/api/v1/scim/v2`. SCIM only answers over https.
3. Create a token, give it a name like `okta-provisioning`, and copy it. It is shown once; the sign-in library stores only its SHA-256 hash. Levelrail keeps the first six characters and who created it, for the token list.
4. Paste the base URL and the token (HTTP header, bearer) into your identity provider.
5. Pick the **Role for new directory users**. It applies when no group mapping matches.

```bash
levelrail-cli sso scim tokens create okta-provisioning
levelrail-cli sso scim tokens list
levelrail-cli sso scim tokens rotate <token-id>
levelrail-cli sso scim tokens revoke <token-id>
levelrail-cli sso settings set --scim-default-role role_viewer
```

**Rotate** issues a new token with the same name, then revokes the old one. If the old one cannot be revoked, the new one is revoked too, so a failed rotation never leaves two live tokens. Update the IdP right after.

## What the directory can do

| Endpoint | Supported |
| --- | --- |
| `/Users` | list, get, create, `PATCH`, delete. `PUT` answers 405 |
| `/Groups` | list, get, create, `PATCH` members (`add`, `remove`, `replace` with a `value` list), display name and external ID, delete. `PUT` answers 405 |
| `/ServiceProviderConfig`, `/ResourceTypes`, `/Schemas` | get |

Filters: `attribute eq "value"` with `userName`, `externalId` or `emails.value` on users, and `displayName` or `externalId` on groups. Matching is case-insensitive. Paging uses `startIndex` and `count` (at most 200). Bulk, sorting, ETags, `Me`, and filters other than `eq` (`co`, `and`, `or`) are not supported; an unsupported filter answers 400 `invalidFilter`.

`PATCH` on a user applies `active`, `userName`, `externalId`, `displayName`, `name` and `emails`. Other attributes an IdP sends in the same request (`title`, `phoneNumbers`, enterprise extension fields, `emails[type eq "work"].value`) are ignored instead of failing the whole request. Okta's `remove` of `members[value eq "id"]` on a group works. A create answers 201 with a `Location` header, and an unknown or malformed id answers 404.

`/Users` lists the people the directory manages: users it created or adopted, and people who signed in through SAML. An existing Levelrail account is adopted when the directory creates a user with the same email and an `externalId`: the account is bound to that external ID and keeps its history. An account already bound to a different external ID is not adopted; the create answers 409.

## Deprovisioning

Setting `active` to false (Entra ID's string `"False"` is accepted too), deleting the user, or creating a user with `active` false takes effect immediately:

- The account is disabled in the sign-in library, and in the same call all its sessions, API tokens and remembered devices are revoked. A disabled account cannot sign in by any method.
- Levelrail forgets trusted browsers and ends any in-memory session of the account.
- The change is written to the audit log as `scim.user.deactivated`, with the SCIM token as the actor.

A deactivated or deleted user disappears from `/Users` but the account is kept, disabled, so history and ownership stay intact. Setting `active` back to true with a `PATCH` on the same id, or creating the same user again through SCIM (same email and `externalId`), restores it and re-enables the account; the person signs in again. The `PATCH` answer after a deactivation reports `"active": false`.

The last user holding `root` can never be deactivated or deleted through SCIM (the request answers 409 and the directory membership is put back), so directory drift cannot lock every administrator out. Grant root to someone else first.

## Groups and roles

When a group's members change, or a group is deleted, everyone affected gets the role of their highest priority mapped group, or the default directory role when none matches. With no mappings configured, SCIM never changes roles and you manage them by hand. Profile email changes at the IdP do not rename the account here; the account keeps the email it was created with.

## IdP guides

These follow each IdP's documented SCIM setup. They have not been run end to end yet.

### Okta

1. In the app you created for SAML (or a new one), **General, App Settings, Provisioning: SCIM**.
2. **Provisioning, Integration:** SCIM connector base URL = the SCIM base URL, unique identifier field = `userName`, actions: Push New Users, Push Profile Updates, Push Groups. Authentication mode **HTTP Header**, paste the token.
3. **Provisioning, To App:** enable Create Users, Update User Attributes and Deactivate Users.
4. **Push Groups** for the groups you mapped to roles. Okta removes group members with a filtered `PATCH` path, which is rewritten to a plain member removal; check the Okta provisioning log after the first group change.

### Microsoft Entra ID

1. In the enterprise application, **Provisioning, Get started**, mode **Automatic**.
2. **Tenant URL** = the SCIM base URL, **Secret Token** = the token. **Test Connection**, then save.
3. Under **Mappings**, keep the default user and group mappings.
4. Turn **Provisioning Status** on. Entra syncs every 40 minutes; use **Provision on demand** to test one user.

### Keycloak

Keycloak has no built-in SCIM client. Use a SCIM provider extension for Keycloak, or keep Keycloak for SAML sign-in only and turn on account creation on the SAML connection.

## Tested against

A table-driven SCIM 2.0 conformance test, `internal/api/sso_scim_live_test.go` (skipped under `-short`, run with `LEVELRAIL_LIVE_IDP=1`), drives `/api/v1/scim/v2` over HTTP against an in-process control plane with requests shaped like Okta and Entra ID send them. No open source SCIM compliance suite was run, and no live IdP tenant.

Verified: bearer token auth (none, forged, revoked token refused immediately); `ServiceProviderConfig`, `ResourceTypes` and `Schemas`; user create (201, duplicate 409 `uniqueness`, missing `userName` 400, bad JSON 400); get, unknown id 404, `PUT` 405; `eq` filters on `userName`, `externalId` and `emails.value` (case-insensitive) and 400 `invalidFilter` for others; `startIndex` and `count` paging; deactivation by Okta (boolean), Entra (`"False"`) and pathless `PATCH`, with the account disabled, then reactivation by `PATCH`, then delete (204) and 404; groups create, add, remove, replace, rename, filter by `displayName`, delete, `PUT` 405, and role changes from group membership through the mappings; Okta's `members[value eq "id"]` removal.

Not verified: a live Okta or Entra ID tenant (their retry, ordering and error handling), `PUT`, ETag and bulk (not supported), more than 200 results per page, and group `PATCH` with member filters other than `value eq`.

## What has not been verified

- Provisioning from a live Okta or Entra ID tenant. The conformance test sends requests shaped like theirs.
- Behaviour of the IdPs when `PUT` answers 405 (the SCIM clients of Okta and Entra ID default to `PATCH` for updates).

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
