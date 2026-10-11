---
description: How app environment variables and secrets are masked on every read surface, how a confirmed reveal works, the reveal policies, and what this protects against.
---

# Environment variables and secrets

An app's Environment tab is one editor for every variable the app reads at start: plain values, encrypted secrets, and Vault references. Values that look like credentials are masked everywhere they are read, and reading one back needs a fresh confirmation that is written to the audit log.

## What counts as sensitive

The control plane classifies every plain variable when it is read, not when it is saved, so older values are covered too.

| Rule | Examples |
| --- | --- |
| The name contains a credential word | `API_KEY`, `API_SECRET`, `GITHUB_TOKEN`, `DB_PASSWORD`, `SMTP_PASSWD`, `PRIVATE_KEY_PATH`, `GOOGLE_APPLICATION_CREDENTIALS`, `SENTRY_DSN`, `DATABASE_URL`, `BASIC_AUTH`, `JWT_SIGNING_KEY`, `QUARTER_ENCRYPTION_KEY_V1` |
| The value looks like key material | a PEM block, a JWT, a URL with a password in it (`redis://user:pass@host`), a long hex or mixed case base64 string, a known token prefix such as `sk_live_`, `ghp_` or `glpat-` |
| Browser public prefixes stay visible | `NEXT_PUBLIC_*`, `VITE_*`, `REACT_APP_*`, `PUBLIC_*`, `NUXT_PUBLIC_*`, `EXPO_PUBLIC_*`, `GATSBY_*`: these are compiled into the browser bundle, so they are public anyway |
| Ordinary settings stay visible | `PORT`, `NODE_ENV`, `NEXT_PUBLIC_API_URL`, UUIDs, commit SHAs in `*_SHA` or `*_COMMIT` variables |

You can override the automatic choice per variable from the row menu: **Mark as sensitive**, **Show value to readers**, or **Use automatic detection**. Marking a value sensitive takes effect at once. Making a sensitive value visible again is treated like a reveal, so it asks you to confirm it's you.

Secrets (saved with **Type: Secret**) are always sensitive. Their values are envelope encrypted at rest (see [Security overview](security.md)).

## What is masked

A sensitive value is never returned by a read. In its place the API returns `••••••••` plus metadata:

```json
{
  "env": { "PORT": "3000", "QUARTER_ENCRYPTION_KEY_V1": "••••••••" },
  "env_meta": {
    "QUARTER_ENCRYPTION_KEY_V1": {
      "sensitive": true,
      "set": true,
      "preview": "ubah...fmc=",
      "value_redacted": true,
      "reason": "name"
    }
  }
}
```

The preview shows at most four characters from each end, and nothing for values shorter than sixteen characters.

Masked surfaces: `GET /api/v1/apps/{name}`, the app list, the responses of create, update and clone, organization, project and environment variables, preview and branch overrides, environment and deploy comparisons, the compose preview, the masked export, the audit log and app events (keys only, never values), every MCP tool, and CLI output. The deploy pipeline, the reconciler and the app importer still read the real values on the server.

### Sending a masked value back

A client that reads an app, changes one field and writes the whole app back sends `••••••••` for the values it never saw. The server keeps the stored value for every existing key that comes back masked. A masked value for a key that has nothing stored is rejected with `400`, so a pasted file can never write the placeholder as a real value. The same rule applies to organization, project and environment variables.

## Revealing a value

Reveal is a separate, audited action:

| Request | Who may call it |
| --- | --- |
| `POST /api/v1/auth/step-up` | a signed-in dashboard session only |
| `POST /api/v1/apps/{name}/env/reveal` with `{"key": "API_SECRET"}` | `read:sensitive`, plus a step-up |
| `POST /api/v1/apps/{name}/secrets/{key}/reveal` | `read:sensitive`, plus a step-up |
| `POST /api/v1/apps/{name}/env/export` with `{"format": "dotenv"}` or `"json"` | `read:sensitive`, plus an export step-up |
| `GET /api/v1/apps/{name}/env/export?values=masked` or `values=none` | `read`, no step-up, values masked or dropped |

Step-up proves it is still you at the keyboard:

- **Password**: your current password. When the account has an authenticator app enabled, the authenticator code (or a recovery code) is required as well.
- **Passkey**: a passkey assertion for the signed-in account (`POST /api/v1/auth/step-up/passkey/begin`, then `.../finish`).

A successful step-up issues a grant that is bound to the browser session and to one action on one app (`reveal`, `export` or `manage` for `web`, for example). It lasts five minutes by default (`APP_SECRET_STEP_UP_TTL`, at most 15 minutes) and is useless from any other session, for any other app, or for any other action. Failed attempts are rate limited per account and per client address and lock out with growing back-off. Every reveal and export writes an audit row with the actor, app, key and method, never the value.

In the dashboard a revealed value shows for 30 seconds (`APP_SECRET_REVEAL_HIDE_AFTER`), hides when the tab is hidden, and is gone when you leave the page. Copy is only offered after a reveal. All export responses carry `Cache-Control: no-store`.

### API tokens

API tokens cannot use step-up grants. A token can reveal only when all of these hold:

1. it carries the `secrets:reveal` ability (only grantable from a signed-in session that holds `read:sensitive`; `root` does not imply it),
2. it also holds `read:sensitive`,
3. the server runs with `APP_SECRET_REVEAL_TOKENS=on` (off by default),
4. the request includes the token owner's password, plus the authenticator code when the owner has one, as `step_up`.

The CLI asks for the password and code at the prompt and never reads them from flags or the environment:

```sh
levelrail-cli apps env get web API_SECRET              # prints ••••••••
levelrail-cli apps env get web API_SECRET --reveal     # prompts, then prints the value
levelrail-cli apps env export web --reveal --out .env  # prompts, writes every value
levelrail-cli apps env export web                      # sensitive keys written empty
```

## Reveal policy

`APP_SECRET_REVEAL` sets the platform policy:

| Value | Behaviour |
| --- | --- |
| `step_up` (default) | reveal and full export need a step-up |
| `disabled` | values are write-only forever; the dashboard explains why |
| `allow` | no step-up for holders of `read:sensitive`; for single admin labs only |

An unknown value turns reveal off and the doctor reports it. `allow` raises a warning in `levelrail-cli doctor` (`secret_reveal_policy`) and a high severity row in the [Security center](security-center.md) (`secret_reveal_allow`).

An app owner can also make one app's secrets write-only from **Protection** on the Environment tab (`PUT /api/v1/apps/{name}/env/settings` with `{"secrets_write_only": true}`). That blocks every reveal and full export for the app. Turning it back off needs a step-up.

## The editor

- **Table**: Key, Value, Type (Plain, Secret, Vault) and Scope (own value, or inherited from the organization, project or environment, with an **Override** action).
- **Add variable** suggests Secret when the name looks sensitive.
- **Convert to secret** moves an existing plain value into encrypted storage on the server. The value never passes through the browser. The running app keeps the old variable until you restart or redeploy.
- **Edit as .env** switches the same rows to text. Masked lines keep what is stored, deleting a line removes the variable, and Vault rows stay in the table.
- **Import .env file** merges a file into the table; a masked or empty value for a masked key keeps the stored value.
- **Review changes** lists added, changed and removed keys before anything is saved, with sensitive values hidden.
- After a save, a **Redeploy needed** banner offers a restart.

## Threat model

This protects against:

- **Shoulder surfing and shared screens**: values are masked by default, a reveal hides itself after 30 seconds and when the tab is hidden.
- **Screen recordings and screenshots** of the Environment tab, the deploy comparison or a support session.
- **A leaked read token or a read-only teammate**: `read` and `read:sensitive` tokens never see values on any read surface.
- **A stolen browser session**: a reveal still needs the password (and authenticator code) or a passkey, and every attempt is audited and rate limited.
- **Browser history and caches**: reveal and export responses are `POST` or `no-store`, and values never appear in URLs.
- **Accidental overwrite from a pasted file**: masked placeholders keep stored values.

It does not protect against:

- an attacker with root on the host, who can read the database, the master key in memory, or the running container's environment,
- an attacker who holds a root API token and the account password,
- a user with `write` who changes the app's image or command to print its own environment: deploying code is equivalent to reading the values that code receives.
