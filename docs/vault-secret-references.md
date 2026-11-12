---
title: "Vault and OpenBao secret references"
description: Read an app's secret from an external Vault at deploy time, which backends work, and what happens when a reference does not resolve.
"og:url": "https://docs.levelrail.com/vault-secret-references"
---

An env var can point at a secret in an external Vault instead of holding a value in Levelrail. Levelrail stores only the reference (a path and a field). The value is read just before the container is created and is not saved.

## Which backends are supported

| Backend | Status | Notes |
| --- | --- | --- |
| Built in encrypted store | Supported | The default. Use `{ secret: true }` or the **Secret** type. See [Environment variables and secrets](/environment-and-secrets). |
| HashiCorp Vault | Supported | KV v2 engine, token or AppRole login. |
| OpenBao | Not tested | OpenBao serves the same KV v2 HTTP API, so the same settings should work. Levelrail has no OpenBao specific test for app references. Try it on a non production app first. |
| Doppler | Beta | Read only. Service token, one project and config per provider. See [Doppler and Infisical](#doppler-and-infisical). |
| Infisical | Beta | Read only. Access token, one workspace and environment per provider. Cloud or self-hosted. |
| 1Password | Beta | Read only, through a 1Password Connect server you run. Connect token, `op://` references. See [1Password Connect](#1password-connect). |

The Vault Transit engine (Vault or OpenBao) can also wrap the master key. That is a separate feature, set with the `APP_VAULT_*` variables in [Environment variables](/environment-variables). It does not enable app references.

## Connect Vault once

The connection is per control plane, not per app.

```bash
levelrail-cli vault set --address https://vault.internal:8200 --auth-method token --vault-token TOKEN
levelrail-cli vault set --address https://vault.internal:8200 --auth-method approle --role-id ID --secret-id SECRET_ID
levelrail-cli vault get
levelrail-cli vault disconnect
```

Other flags on `vault set`:

| Flag | Default | Meaning |
| --- | --- | --- |
| `--namespace` | none | Vault namespace, if you use them. |
| `--mount-path` | `secret` | Mount path of the KV v2 engine. |
| `--enabled=false` | `true` | Turn references off without deleting the stored credential. |

The address is required the first time. The credential is stored encrypted, is write-only, and is never shown by `vault get`. The same settings are under **Settings, Vault** and at `PUT /api/v1/settings/vault`.

Rotate the stored credential without downtime. The old one keeps working for the grace window (`APP_SECRET_ROTATION_GRACE_HOURS`, 24 hours by default):

```bash
levelrail-cli vault rotate-credential --credential-file ./new-token
```

## Reference syntax

In `app.yaml`:

```yaml
env:
  API_KEY: { vault: { path: myapp/config, key: api_key } }
```

`path` is the secret path inside the KV v2 mount. Do not include the mount name or `data/`. `key` is the field in that secret. The field must be a string. A variable can use only one of `from`, `secret` and `vault`.

On the command line, when you create an app from an image or a git build:

```bash
levelrail-cli apps create ... --vault-secret API_KEY=myapp/config#api_key
```

The format is `KEY=path#field`. It splits at the last `#`, so a path may contain `#`. The flag is repeatable.

For an existing app:

```bash
levelrail-cli apps vault-env set web API_KEY --path myapp/config --key api_key
levelrail-cli apps vault-env clear web API_KEY
```

The API is `PUT` and `DELETE /api/v1/apps/{name}/vault-env/{key}`.

## When values are read

Levelrail does no caching. Every time the reconciler creates a container for the app, it logs in to Vault and reads the secret again. A change in Vault shows up in the app the next time its container is created, not before. To pick up a new value, redeploy the app.

Because each read is a fresh login, Vault must be reachable from the control plane at every deploy.

## Troubleshooting

An unresolved reference fails the deploy. The container is never started with the variable empty or missing. The error names the variable, the path and the field.

| Error text | Cause | Fix |
| --- | --- | --- |
| `vault is not configured on this control plane` | A reference exists but no Vault connection was saved. | Run `levelrail-cli vault set`. |
| `vault is not enabled on this control plane` | The connection exists but was disabled. | Run `levelrail-cli vault set --enabled=true`. |
| `resolve vault credential` | The stored credential cannot be read, for example after a master key problem. | Run `vault set` again with the credential. |
| `approle login` | Wrong role ID or secret ID, or the role is not allowed to log in. | Check the role and secret ID in Vault. |
| `read secret "PATH"` | Vault refused the read: bad token, missing policy, wrong mount or namespace, or Vault unreachable. | Check the token policy allows read on the path, then `--mount-path` and `--namespace`. |
| `secret "PATH" not found` | Nothing exists at that path. | Check the path has no mount prefix and no `data/`. |
| `vault address is link-local, metadata or private` | The Vault address resolves to a loopback or private address and an admin turned `vault.allow_private_address` off (it is on by default). Link-local and metadata addresses are always refused. | For a Vault or OpenBao on the same host or LAN, run `levelrail-cli settings runtime set vault.allow_private_address true`. Connections saved earlier keep working on private addresses after an upgrade unless the setting was turned off. |
| `has no field "KEY"` | The secret exists but not that field. | Check the field name in Vault. |
| `is not a string` | The field holds a number, object or list. | Store the value as a string. |

Check the connection before you deploy:

```bash
levelrail-cli vault get
vault kv get -mount=secret myapp/config
```

The second command uses the Vault CLI with the same token, to confirm the path and field exist.

## Doppler and Infisical

Doppler and Infisical are read only providers. Levelrail never writes to them. Connect one or more providers by name:

```bash
levelrail-cli secrets provider add prod --type doppler --project web --environment prd --provider-token-file ./doppler-token
levelrail-cli secrets provider add infra --type infisical --project WORKSPACE_ID --environment prod --provider-token-file -
levelrail-cli secrets provider add lab --type infisical --project WORKSPACE_ID --environment dev --base-url https://infisical.internal --provider-token-file ./token
levelrail-cli secrets provider list
levelrail-cli secrets provider test prod
levelrail-cli secrets provider remove prod
```

`--type vault` on `add` and `remove` uses the Vault settings above. The token is stored encrypted, is write-only, and is never returned by the API or the CLI. Saving a new token keeps the old one working for the rotation grace window. Adding and removing providers and the connection test need the root or admin role and are audited. The API is `GET /api/v1/settings/secret-providers`, `PUT` and `DELETE /api/v1/settings/secret-providers/{name}`, and `POST /api/v1/settings/secret-providers/{name}/test`.

In the dashboard, a 1Password provider row shows the Connect server address (scheme, host and port only) and the default vault. The token is never shown.

To use a value, store a reference as an app secret:

```bash
levelrail-cli apps secrets set web API_KEY doppler://prod/API_KEY
levelrail-cli apps secrets set web STRIPE_KEY infisical://infra/billing/STRIPE_KEY
```

The Infisical form is `infisical://PROVIDER/[folder/...]/NAME`. Without a folder the provider's `--path` is used, `/` by default.

The value is read when the container is created and kept only in memory for the cache time. It is masked in build and deploy logs. A deploy fails, and never starts with an empty value, when the provider is missing, rejects the token, does not answer in time, does not have the secret, or returns an empty value. The error names the variable and the reference.

Network rules: Levelrail calls only `api.doppler.com` or `app.infisical.com`, or the `--base-url` an admin set for self-hosted Infisical. Private and loopback addresses are refused unless they come from that explicit base URL. Link-local addresses such as cloud metadata are always refused. A redirect to another host or scheme is refused.

Provider references (`doppler://`, `infisical://`, and `op://` values that name a configured provider) are set by root, the same tier that manages providers. A provider `path` on Infisical is a limit: a reference may only point at that path or below it. Changing a provider's `base_url` or project needs a new token.

| Runtime setting | Default | Meaning |
| --- | --- | --- |
| `secrets.provider_timeout_seconds` | 10 | Time limit for one provider read. |
| `secrets.provider_cache_ttl_seconds` | 60 | Seconds a read value is reused. 0 reads on every container start. |
| `secrets.provider_max_response_kib` | 1024 | Largest provider response read. |
| `secrets.provider_max_rps` | 20 | Most requests per second to one provider connection. A read over the limit fails with a rate limit reason and is not queued. |

### Infisical machine identity

Instead of a long-lived access token, an Infisical provider can use a machine identity with universal auth. Give the two values separately: `--client-id ID --client-secret-file PATH|-` on the CLI, or the Client ID and Client secret fields in the dashboard form. They cannot be combined with an access token. The server stores them as `infisical-identity:CLIENT_ID:CLIENT_SECRET` (you can also pass that string as the provider token), encrypted like any token. The client ID is shown again when you edit the provider (the client secret never is); changing the ID needs the client secret entered again, and saving with neither changed keeps the stored credential.

Levelrail logs in when it first needs to read, keeps the short-lived access token in memory (one per provider), and logs in again lazily once about 90 percent of its lifetime has passed. There is no background refresh. A rejected login fails the read closed, and neither the client secret nor the access token appears in errors or logs. Replacing the credential triggers a new login on the next read.

Status: Beta. Tested against a local stand-in server only, not yet against a real Infisical instance.

## 1Password Connect

1Password is a read only provider that talks to a Connect server you host. It is set up in Settings > Secret providers, through the API or the CLI. The Connect server URL is required, the default vault (`--project`) is optional through the API and CLI, but the dashboard form requires it. No environment is used. Connect it with the Connect server URL and an access token:

```bash
levelrail-cli secrets provider add vaults --type onepassword --base-url https://connect.internal:8080 --project Prod --provider-token-file -
levelrail-cli apps secrets set web STRIPE_KEY op://vaults/Stripe/api_key
levelrail-cli apps secrets set web OTHER_KEY op://vaults/Staging/Stripe/api_key
```

The form is `op://PROVIDER/[VAULT/]ITEM/FIELD`. Without a vault, the provider's `--project` is the vault. When the provider sets `--project`, a reference naming any other vault is refused. A value starting with `op://` whose first part is not a configured 1Password provider stays a literal, so apps that resolve `op://` themselves keep working. Vault and item can be a name or a Connect ID, and the field is matched by its label or ID. Names with quotes or backslashes are refused. A name that matches no item or more than one item fails the read.

`--base-url` is required and its address may be private, because the Connect server is yours. Link-local addresses are always refused and a redirect to another host is refused. The same rules, fail closed behavior and runtime settings as above apply. Each read makes up to three requests (vault, item, item detail) and is cached for the cache time.

Not verified against a real Connect server yet: the tests use a local fake of the Connect API.

## Named connection references

An env var can also point at a value in a named secret connection with one reference form:

```text
secret://<connection>/<path>#<key>
secret://<connection>/<path>#<key>?version=3
```

In app.yaml and stack files the same reference is an object, so it needs no quoting:

```yaml
env:
  DATABASE_URL: secret://prod-vault/apps/web#database_url
  API_KEY: { secret: { conn: prod-vault, path: apps/web, key: api_key } }
```

`{ secret: true }` still means a value in the built in encrypted store, and `{ vault: { path, key } }` still reads the single Vault connection. Both keep working unchanged. `secret://vault/<path>#<key>` names the same Vault connection.

Rules:

- Levelrail stores the reference text only. The value is read just before the container is created and is never saved in the database, revisions, logs, events or API responses.
- If a reference cannot be read (denied, missing, or the connection is down), the deploy is blocked. The error names the reference, never the value.
- Setting or changing a reference needs the root ability, and an audit entry records the reference text.

From the CLI:

```bash
levelrail-cli env set web --secret API_KEY=secret://prod-vault/apps/web#api_key
```

The value of `--secret` must be a `secret://` reference. A literal value is refused so it never lands in shell history. In stack files (see [Platform as code](/platform-as-code)) the same string or the `secret` object works in `env`, and `secretRef: { connection, path, key }` is accepted too. The plan shows the reference text.
