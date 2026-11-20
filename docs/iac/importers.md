---
title: "Import from a deploy.yml or a Portainer stack"
sidebarTitle: "Importers"
description: "Turn an existing deploy.yml or Portainer stack file into reviewable Levelrail App documents with an offline dry run, and save them with --write."
"og:url": "https://docs.levelrail.com/iac/importers"
---

Offline importers turn an existing deployment definition into reviewable Levelrail documents (App kinds inside a Stack). Both are dry runs: nothing is written unless you pass `--write DIR`, and existing files are kept without `--force`.

```sh
levelrail-cli import deploy-yml config/deploy.yml                 # print the documents
levelrail-cli import deploy-yml config/deploy.yml --write ./iac   # save them
levelrail-cli import portainer stack.json --stack blog --write ./iac
levelrail-cli import railway railway.toml --image-repo registry.example.com/team --write ./iac
```

Then run `levelrail-cli stack plan NAME -f ./iac` and apply as for any stack.

## Secrets are references

Values are never read into the output. Every secret becomes a name listed in `SECRETS.md`, and the App document refers to it as a secret reference. Clear environment values that look like secrets (by name or by value) are turned into references too. Fill them at apply time with `--secret NAME=env:CI_SECRET`.

## deploy.yml

| Source field | Result |
| --- | --- |
| `service`, `image` | the main App (both required) |
| `servers.<role>.cmd` | one extra App per role with a command |
| `proxy.host`, `proxy.hosts` | App domains |
| `proxy.app_port`, `proxy.healthcheck.path` | App port and readiness path |
| `env.clear` | App env, with secret-looking values turned into references |
| `env.secret` | secret names |
| `accessories.<name>` | one App each (image, port, env, cmd) |

Fields with no equivalent (`registry`, `ssh`, `builder`, `boot`, `volumes`, server hosts and others) are listed in the notes. Any field the importer does not know is an error that names it, so nothing is dropped silently. A file with ERB tags (`<% %>`) is refused: render it first.

## Portainer

The input is a stack compose file or the JSON export of a stack (`Name`, `StackFileContent`, `Env`). Image services become Apps. Environment values and the export `Env` values are never read, only names. Services with a build context are skipped with a note, and volumes, command overrides and `env_file` entries are listed in the notes.

## Railway config

The input is a `railway.json` or `railway.toml`. It produces one App from the `deploy` section, with no image: build and push one, then pass `--image-repo` or set it in the App document.

| Source field | Result |
| --- | --- |
| `deploy.startCommand` | App command |
| `deploy.healthcheckPath` | App readiness path |
| `deploy.numReplicas` | App replicas |
| `deploy.preDeployCommand` | pre-deploy hook |
| `deploy.cronSchedule` | a scheduled job running the start command (UTC) |
| `variables` (names only) | secret names, values are never read |

`build.builder`, `build.buildCommand`, `build.dockerfilePath`, `deploy.restartPolicyType`, `deploy.healthcheckTimeout` and any other field have no direct equivalent in an image-only App. They are listed in the notes with what to set by hand. A command that carries an inline credential is not kept.

The size limit for the input file is `LEVELRAIL_IMPORT_FILE_MAX_BYTES`.
