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
| 1Password | Not supported yet | There is no 1Password backend. |

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
| `has no field "KEY"` | The secret exists but not that field. | Check the field name in Vault. |
| `is not a string` | The field holds a number, object or list. | Store the value as a string. |

Check the connection before you deploy:

```bash
levelrail-cli vault get
vault kv get -mount=secret myapp/config
```

The second command uses the Vault CLI with the same token, to confirm the path and field exist.
