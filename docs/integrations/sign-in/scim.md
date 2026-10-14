---
title: "Use SCIM 2.0 with Levelrail: user provisioning"
sidebarTitle: "SCIM 2.0"
description: "Let Okta, Microsoft Entra ID or any SCIM 2.0 client create, update and deactivate Levelrail accounts and groups with a bearer token."
---

Let Okta, Microsoft Entra ID or any SCIM 2.0 client create, update and deactivate Levelrail accounts and groups with a bearer token.

This page covers how to provision and deprovision users and groups from your identity provider.

## What you get

- Users and groups created and updated from the directory, driving roles through the same mappings as SAML.
- Immediate session and API token revocation when a user is deactivated.
- Audit entries such as scim.user.deactivated with the SCIM token as actor.

## Set up

1. Prepare the credential. HTTP bearer token. Only its SHA-256 hash is stored and it is shown once.
2. In the dashboard, open **Settings > Single sign-on** (`/settings/sso`), then open the Directory sync (SCIM) section.
3. Open the sign-in page in a private window and confirm the new button completes a login.

From the CLI:

```bash
levelrail-cli sso scim tokens create okta-provisioning
levelrail-cli sso settings set --scim-default-role role_viewer
levelrail-cli sso scim tokens rotate <token-id>
```

## Permissions and scopes

| Needed | Why |
| --- | --- |
| Root | Creating and rotating SCIM tokens needs root. |
| Endpoints | /Users and /Groups support list, get, create, replace, PATCH and delete. ServiceProviderConfig and ResourceTypes are readable. |

## Endpoint and format

SCIM base URL: `https://<dashboard>/api/v1/scim/v2`. SCIM only answers over https.

## Security notes

- Client secrets and signing keys are encrypted under the control plane master key and never returned by the API.
- Changing sign-in settings needs a root session or token, and refusals are written to the audit log.
- Nothing you enter here is shown again after you save it.

## Limits and caveats

- Interim implementation behind the saml-scim beta feature, off by default.
- Bulk, sorting and ETags are not supported. Paging allows at most 200 items per page.
- The last root user cannot be deactivated or deleted through SCIM; the request answers 409.
- Profile email changes at the IdP do not rename the account.

## Troubleshooting

**The IdP gets 404 from the SCIM base URL.** The saml-scim beta feature is off. Turn it on under Settings, Beta features or with APP_EXPERIMENTAL=saml-scim.

**A deprovision request answers 409.** The target is the last root user. Grant root to someone else first.

**Roles do not change when group membership changes.** With no group to role mappings configured SCIM never changes roles. Add mappings under Single sign-on.

## FAQ

**Do I need SAML as well?**

No. People can sign in with SAML, OAuth or a password. SCIM only manages accounts and groups.

**What happens to a deleted user?**

The account is kept but disabled, so history and ownership stay intact. Creating the same email again restores it.

## Related

- [SAML 2.0](/integrations/sign-in/saml): Sign-in providers
- [Generic OpenID Connect](/integrations/sign-in/oidc): Sign-in providers
- [All sign-in providers](/integrations/sign-in)
- [SCIM 2.0 documentation](https://www.rfc-editor.org/rfc/rfc7644), maintained by the vendor
