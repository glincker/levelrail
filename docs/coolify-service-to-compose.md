---
title: "Move a Coolify one-click service to a Compose app"
sidebarTitle: "Service to Compose"
description: "How migrate coolify stages a Coolify one-click service as a Compose file with its magic variables translated, what it leaves for you to fix, and a manual path when you need one."
"og:url": "https://docs.levelrail.com/coolify-service-to-compose"
---

A Coolify one-click service is a Compose file with `SERVICE_` magic variables. `migrate coolify` finds these services on the Coolify instance, translates the magic variables, and stages each one as a Compose file on your machine. It starts nothing. You review the staged file, then deploy it with one command.

For the rest of a Coolify move, see [Migrate from Coolify](/migrate-from-coolify).

## What the migration does

For each one-click service it discovers, `migrate coolify` does four things:

1. Translates the `SERVICE_` tokens in the Compose body into literal values, using the table below.
2. Writes the result to `<stage-dir>/<name>/compose.yaml`, with owner-only permissions (directory `0700`, file `0600`), because the file holds generated secrets.
3. Lists what it could not translate or that needs a review, in the migration report.
4. Prints the deploy command for the staged file.

It does this in file mode (the default) and with `--apply`. It does not stage anything in `--dry-run` or `--verify`, which write nothing. Nothing is deployed in any mode.

Staging only covers one-click services. A Coolify application that uses the `dockercompose` build pack is a different case, covered at the end of this page.

### The flags

| Flag | What it does |
| --- | --- |
| `--stage-dir` | Directory for the staged services. Default `./migrated/compose`. |
| `--domain-suffix` | Parent domain for the hosts of staged services. Default `localhost`, so nothing routes publicly until you change it. |
| `--out-dir` | Directory for the app files of the other migrated apps. File mode only. |

A host from `--domain-suffix` is built from the magic variable's key. For example, with `--domain-suffix apps.example.com`, the key `APP` becomes `app.apps.example.com`. Set the suffix to a domain you own before you deploy a staged service that should be reachable.

## Step 1: Stage the services

```bash
levelrail-cli migrate coolify --url https://coolify.example.com --token "$COOLIFY_TOKEN" \
  --stage-dir ./migrated/compose --domain-suffix apps.example.com
```

Use a Coolify token that can read services. The migration reads the services with that token, and it warns on stderr if the read is not permitted, then carries on with the applications only.

The report ends with the staged services. For each one it shows the staged path, the note that the file holds generated secrets, and the deploy command:

```text
  - plausible (from "plausible")
      staged: ./migrated/compose/plausible/compose.yaml (holds generated secrets, do not commit)
      deploy: levelrail-cli compose deploy plausible -f ./migrated/compose/plausible/compose.yaml
```

The deploy line is printed with the word `levelrail`. In these docs the CLI is `levelrail-cli`, so type the command with that name.

Keep the staged files out of git. They contain the generated passwords.

## Step 2: Review the staged file

Open `compose.yaml` and check the items the report lists under the service. The migration lists each of these and does not change it unless the table says so:

- **Unsupported magic variables.** A `SERVICE_` token with no Levelrail equivalent stays in the file. The staged file is marked as not applied until you fix it, so the deploy refuses it.
- **Coolify-only keys.** Keys such as `exclude_from_hc`, `x-coolify…` and `coolify.*` labels are listed as ignored. They stay in the file, so remove them if they cause trouble.
- **Bind mounts to host paths.** A path such as `./data` or `/srv/data` is a path on the Coolify server. Copy its data over and switch to a named volume.
- **Masked secrets.** If Coolify returned a masked value for an inline secret, the file contains the mask. Replace it with the real value.
- **Required variables with no default.** A `${NAME}` reference with no default is listed. Put the value in the app's environment before you deploy.

## Step 3: Deploy the staged file

```bash
levelrail-cli compose preview -f ./migrated/compose/plausible/compose.yaml
levelrail-cli compose deploy plausible -f ./migrated/compose/plausible/compose.yaml
levelrail-cli compose status plausible
levelrail-cli compose logs plausible --service app --tail 100
```

Read the preview before the deploy. It shows the services and whether anything is still unresolved. Whether `compose deploy` creates the app or needs it to exist first is pending run on a real host.

If a new release misbehaves, list the revisions and roll back:

```bash
levelrail-cli compose revisions plausible
levelrail-cli compose rollback plausible --revision <n>
```

## Magic variable table

The table is the current behavior of `internal/compose/magicvars.go` and the translation in `TranslateMagicVars`. The same name in one file always gets the same value, so an app and its database can share a password.

| Token in the Coolify file | Result in the staged file | Default |
| --- | --- | --- |
| `SERVICE_FQDN_KEY` | The host `key.<domain-suffix>`. A trailing `_PORT` on the key is removed first. | Suffix `localhost` |
| `SERVICE_URL_KEY` | `https://` plus the same host | Suffix `localhost` |
| `SERVICE_PASSWORD_KEY` | A random alphanumeric password | 32 characters |
| `SERVICE_PASSWORD_64_KEY` | The same, with the length in the name | 64 characters |
| `SERVICE_USER_KEY` | A random lowercase user name | 16 characters |
| `SERVICE_HEX_KEY` | A random hex string | 64 characters |
| `SERVICE_BASE64_KEY` or `SERVICE_REALBASE64_KEY` | A random URL-safe base64 string | 32 characters |
| `SERVICE_BASE64BYTES_KEY` | Standard base64 of random bytes | 32 bytes |
| Any other `SERVICE_` token with a `:-default` | The default | As written |
| Any other `SERVICE_` token | Left in place and listed as unsupported | None |

A token can be written as `$SERVICE_NAME` or `${SERVICE_NAME}`. A variable declared on its own line, as `- SERVICE_NAME` in a list or `SERVICE_NAME:` in a map, becomes `NAME=value` or `NAME: "value"`.

Here is a small example of the translation with `--domain-suffix apps.example.com`. The values are shown as placeholders because each one is generated:

```yaml
services:
  app:
    image: ghcr.io/example/app:1.0
    environment:
      APP_URL: "https://app.apps.example.com"
      DB_PASSWORD: "<generated, 64 characters>"
  db:
    image: postgres:16
    environment:
      POSTGRES_PASSWORD: "<the same generated value>"
```

## Manual path

Use this when the staged file is not what you want, or when you have a Compose file from elsewhere.

1. Copy the Docker Compose content of the service from Coolify and save it as `compose.yaml`.
2. Replace each `SERVICE_` token using the table above. Use a real domain for the `FQDN` and `URL` tokens.
3. Run `levelrail-cli compose preview -f compose.yaml`, then `levelrail-cli compose deploy <app> -f compose.yaml`.

The manual path does not check the file for Coolify-only keys or bind mounts. Check those yourself, as in Step 2.

## Things to know

- **Data is not copied.** A database in the Compose file starts empty unless you restore a backup into it. See [Back up Postgres to S3 and test the restore](/tutorials/back-up-postgres-to-s3).
- **Keep the Coolify service** stopped rather than deleted until the new app is verified.
- **Compose applications with the `dockercompose` build pack are not translated.** A Coolify application that uses that build pack is multi-service, and the migration marks it as blocked. Stage it with the manual path above.
