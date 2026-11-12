---
title: "Cloud connections (preview)"
description: Store named cloud account connections on your own control plane, reference credentials from your own vault, and run read-only health checks. Preview, off by default.
"og:url": "https://docs.levelrail.com/cloud-connections"
---

Cloud connections are a preview. They are off by default and the API, CLI and any future screen refuse every request until you turn on the runtime setting `cloud.connections_enabled`.

A connection is a named record of one cloud account on your control plane. It holds non-secret settings (region, account or tenancy ID, compartment, role ARN, subscription ID) and a reference to a credential. It never holds the credential. Everything stays on your control plane: nothing is sent to a hosted service.

## What works today

| Part | State |
| --- | --- |
| Store, create, update, delete, list | Works. Names are lowercase `[a-z0-9_-]`. Providers: `oracle`, `aws`, `azure`, `gcp`, `hetzner`, `digitalocean`. |
| Validation | Works. Required settings per provider and the allowed auth methods are checked when you save. |
| Credential references | Works as references only. `secret_ref` and `role_assumption` take a `secret://CONNECTION/path#key` reference resolved through your [secret connections](/vault-secret-references). `static` takes `NAMESPACE/KEY` naming a value in the built in encrypted store. `oidc_federation` takes none. |
| Live health check (`test`) | Not implemented for any provider in this build. The check returns the class `unsupported`. The code path that resolves the credential and calls a provider is in place and covered by tests with a fake provider. |
| Creating or changing cloud resources | Not part of this feature. Nothing here provisions or edits anything in your account. |

The real read-only checks for Oracle, AWS and Azure need each cloud's client library. Adding those dependencies is a separate decision, so the providers are registered as not implemented until then.

## Turn it on

```sh
levelrail-cli settings runtime set cloud.connections_enabled true
```

The environment variable `APP_CLOUD_CONNECTIONS_ENABLED` only seeds the default. A stored value wins and applies without a restart.

## Use it

```sh
levelrail-cli cloud connection create oci-test --provider oracle --auth secret_ref \
  --region us-ashburn-1 --account-id ocid1.tenancy.example --compartment ocid1.compartment.example \
  --credential-ref 'secret://vault/oci/api-key#pem'
levelrail-cli cloud connection list
levelrail-cli cloud connection test oci-test
levelrail-cli cloud connection update oci-test --team TEAM_ID
levelrail-cli cloud connection delete oci-test
```

Listing needs the read ability. Create, update, delete and test need root. Every change and test writes an audit entry holding the connection name, provider, team and the outcome class. Credential material and credential references are not written to the audit log.

## Health classes

`test` returns one class and a fixed short reason. Provider output is never passed through.

| Class | Meaning |
| --- | --- |
| `ok` | The read-only check passed. |
| `unsupported` | This build has no live check for the provider. |
| `config_invalid` | The stored settings no longer validate. |
| `credential_unresolved` | The reference could not be resolved. |
| `auth_failed` | The provider rejected the credential. |
| `unreachable` | The provider endpoint could not be reached. |
| `unknown` | Any other failure. |

## API

`GET`, `POST` on `/api/v1/settings/cloud-connections`, `PUT`, `DELETE` on `/api/v1/settings/cloud-connections/{name}` and `POST` on `/api/v1/settings/cloud-connections/{name}/test`. While the setting is off every route answers 403 with a message naming it.
