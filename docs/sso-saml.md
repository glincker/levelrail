---
title: "SAML single sign-on"
description: "Sign in through Okta, Microsoft Entra ID, Keycloak or any SAML 2.0 identity provider, map directory groups to roles, require SSO and keep a break-glass path."
"og:url": "https://docs.levelrail.com/sso-saml"
---

> **Beta, off by default.** SAML sign-in and [SCIM provisioning](scim.md) sit behind the `saml-scim` beta feature: turn it on under **Settings, Beta features** or with `APP_EXPERIMENTAL=saml-scim`. While off, the routes answer `404`, the settings page and the sign-in button are hidden, and a "require single sign-on" setting is not applied. The service provider is built on the sign-in library; Levelrail adds a stricter response check, account policy and role mapping around it. It has been run against Keycloak 26.0.8 and SimpleSAMLphp 1.15 in a local lab (see [Tested against](#tested-against)). Okta and Microsoft Entra ID have not been tested (see [What has not been verified](#what-has-not-been-verified)).

Connect one or more SAML 2.0 identity providers (IdPs). For OpenID Connect and OAuth providers see [OAuth and OIDC single sign-on](sso-oidc.md), which has the same group, role and account controls. Each enabled connection adds a button to the sign-in page. Directory groups can decide each person's role, and you can require single sign-on for everyone except root users.

<InlineToc default-open />

## SCIM provisioning at a glance

SCIM 2.0 provisioning shares the `saml-scim` toggle and the group to role mappings below. Open **Settings, Single sign-on, Directory sync (SCIM)**, copy the base URL `https://<dashboard>/api/v1/scim/v2`, create a token (shown once) and paste both into your IdP. It supports `/Users` and `/Groups` (list, get, create, `PATCH`, delete), deactivation that revokes sessions at once, and the discovery endpoints. Bulk, sorting, ETags and `PUT` are not supported. Full steps, IdP guides and limits: [SCIM provisioning](scim.md).

```bash
levelrail-cli sso scim tokens create okta-provisioning
```

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
levelrail-cli sso saml test saml_abc
levelrail-cli sso saml remove saml_abc
```

`add` and `remove` are the same commands as `create` and `delete`.

## Test the connection

Once a connection is complete and enabled, **Test sign-in** in its sheet (or `levelrail-cli sso saml test <id>`, which prints the address to open in a browser signed in as root) runs the real sign-in at your identity provider for you. It ends on the settings page with a result panel: the email, groups and attributes the assertion carried, the role they would earn, whether the account would be linked, created or refused, and a typed fix for each problem. A test sign-in creates no account, role or session, and its result is shown once, to the root user who started it.

| Result | What it means | Fix |
| --- | --- | --- |
| `signature_invalid` | The assertion is not signed by the certificate on file, or only the outer response is signed | Import the IdP metadata again, and sign the assertion |
| `audience_mismatch` | The IdP addressed another entity ID | Set the audience to the entity ID shown in the sheet |
| `clock_skew` | The assertion is outside its validity window (three minutes of skew are allowed) | Fix time sync (NTP) on this server or at the IdP |
| `request_mismatch` | The response answers another request or destination | Start the test again, and check the ACS URL at the IdP |
| `response_invalid` | The response could not be read or verified for another reason | Check the IdP logs and import its metadata again |
| no groups received | The groups attribute name does not match what the IdP sends | Set the attribute name, or add a group statement at the IdP |
| domain not allowed | The email domain is not claimed by the connection | One click adds the domain |
| not provisioned | No account exists and account creation is off | One click turns account creation on |

The failure codes are classified from the verifier's reason, and only `signature_invalid` is exercised against a real IdP (a tampered response). The other typed failures are covered by the classifier's unit test, not by a live run.

## What is checked on every sign-in

Sign-in is SP-initiated only: it always starts from the button on the sign-in page. Every response passes two checks, and the browser lands back on the sign-in page with a short error (and the reason goes to the control plane log and the audit log) if either fails.

Levelrail's check, before the library sees the response:

- `InResponseTo` names the AuthnRequest this browser started, and the assertion carries a bearer confirmation for that same request. Each request ID is single use, expires after ten minutes, and is bound to the browser by a secure cookie. This is what refuses IdP-initiated, forwarded and replayed responses. The library on its own accepts IdP-initiated responses (no `InResponseTo`) and checks `InResponseTo` only when one is present.
- The assertion names this connection's entity ID as its audience. An assertion without an audience restriction is refused.
- The response carries a valid XML signature from the stored IdP certificate, the issuer is the configured IdP entity ID, `Destination` and the bearer `Recipient` are this connection's ACS URL, `NotBefore` and `NotOnOrAfter` hold (three minutes of clock skew), and the response and assertion are no older than 90 seconds.

The library's check, which then links the identity and issues the session:

- The same signature, issuer, audience and time checks, plus: the assertion itself must be signed. A response that is signed only on the outer element is refused.

In-flight sign-ins are held in memory, so a control plane restart in the middle of one means starting again from the button.

Encrypted assertions are not supported. The sign-in library looks for the assertion signature in the raw response, which an encrypted assertion hides, so the response is refused with `saml assertion not signed` and no account or session is left behind. Turn assertion encryption off at the IdP.

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
levelrail-cli sso mappings set --map Engineering=role_operator:10 --map everyone=role_viewer
levelrail-cli sso mappings list
```

The highest priority group the person belongs to decides the role. Group names match case-insensitively. A person in no mapped group gets the connection's default role. With **Apply the mapped role on every sign-in** (`--sync-roles`), the role is re-applied at each sign-in, so a group change at the IdP takes effect at the next one. With sync on and no default role, a person in no mapped group is refused. A mapping cannot name a role that holds root: only a person with root can grant it, so an assertion never can. A user who already holds root is never changed by role sync, so directory drift cannot lock the instance out. The same mappings drive [SCIM provisioning](scim.md) groups and [OAuth and OIDC sign-in](sso-oidc.md).

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

The Okta and Entra ID steps follow each IdP's documented SAML setup and have not been run against a live tenant. The Keycloak steps were run end to end (see [Tested against](#tested-against)).

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

## Tested against

Last run on 2026-10-20 against real identity providers in local Docker containers (the SSO lab, `test/sso-lab/`), driven by a scripted HTTP client with a cookie jar (no browser): SP-initiated login, the IdP's own login form, and the signed response posted back to the ACS of an in-process control plane. The tests are `internal/api/sso_saml_live_test.go` and `internal/api/sso_saml_test_flow_live_test.go`, skipped under `-short` and run only with `LEVELRAIL_LIVE_IDP=1`.

To reproduce it on your machine (Docker required, lab credentials only):

```bash
scripts/sso-lab.sh up          # starts both IdPs, waits until healthy, prints metadata URLs and test users
export LEVELRAIL_LIVE_IDP=1
CGO_ENABLED=0 go test ./internal/api -run 'TestLive' -count=1 -v
scripts/sso-lab.sh down
```

The same lab is a quick way to try the setup flow by hand: import `https://localhost:18443/realms/lab/protocol/saml/descriptor` into a connection (the lab certificate is self-signed, so paste the metadata XML instead of using the URL) and register the connection's service provider values at the lab Keycloak.

| IdP | Version | Verified |
| --- | --- | --- |
| Keycloak | 26.0.8, `start-dev`, https | Everything in the list below, plus Keycloak importing Levelrail's SP metadata as a client |
| SimpleSAMLphp | 1.15 (image `kristophjunge/test-saml-idp:1.15`, amd64), https | Everything in the list below |

Verified against both:

- Metadata: the IdP descriptor parses into entity ID, sign-in URL and signing certificate.
- Sign-in with a signed assertion creates the account with the right email, picks up the group attribute and applies the mapped role (with role sync on). A second sign-in links the same account.
- Account creation off refuses an unknown person, and signs in an invited one.
- A tampered response, a replayed response (same flow and a fresh flow), a response with no flow cookie and a response issued for another connection are all refused, each for its own reason.
- An assertion that is not signed is refused when only the outer response is signed (the library gate) and when nothing is signed, and no account is left behind.
- Keycloak only: **Test sign-in** returns the received email and groups, the mapped role and an `account would be created` verdict without creating anything; a domain the connection does not claim comes back refused with the add-domain fix; a tampered response comes back as `signature_invalid`; the sign-in page's domain lookup (`/api/v1/auth/login-options?sso_domain=`) returns the connection for a claimed domain and nothing for an unclaimed one.
- Keycloak only: an IdP-initiated response is refused, with and without a flow cookie. Encrypted assertions are refused cleanly (see above).

Not verified live, covered only by the canned-response unit tests: a wrong audience, expired or not-yet-valid assertions (the three minute skew cannot be waited out), an IdP certificate rotation, and metadata import from an https URL (the IdPs ran with self-signed certificates and the fetch refuses private addresses).

## What has not been verified

- Sign-in against Okta or Microsoft Entra ID, which have not been tested at all: no tenant was available. The Okta and Entra ID guides above follow each vendor's documentation.
- Sign-in against a live Okta or Entra ID tenant. Their attribute names, NameID formats and group claims are covered by the attribute settings above, not by a run.
- Encrypted assertions are refused, see above.
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
| `GET` | `/api/v1/sso/saml/connections/{id}/test/start`, `/test/result` | `root`, needs a dashboard session |
| `GET`, `PUT` | `/api/v1/sso/group-mappings` | `root` |
| `GET`, `PUT` | `/api/v1/sso/settings` | `root` |

The sign-in library's own SAML and organization routes are not exposed; only the routes above reach it.

| Variable | Default | Purpose |
| --- | --- | --- |
| `APP_SAML_RATE_PER_MINUTE` | `30` | SAML sign-in starts and responses accepted per client IP per minute |
