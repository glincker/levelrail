---
title: "Import Heroku, Fly, DigitalOcean, Portainer and more"
sidebarTitle: "Importers"
description: "Turn a fly.toml, DigitalOcean app spec, Render Blueprint, Netlify, Railway or Portainer file, or a Heroku account, into reviewable Levelrail documents with a dry run."
"og:url": "https://docs.levelrail.com/iac/importers"
---

Offline importers turn an existing deployment definition into reviewable Levelrail documents (App kinds inside a Stack). They are dry runs: nothing is written unless you pass `--write DIR`, and existing files are kept without `--force`. Supported files: `deploy.yml`, Portainer stacks, `railway.json` or `railway.toml`, `fly.toml`, `.do/app.yaml`, `render.yaml`, `netlify.toml` and a Cloud Run service YAML. A Heroku account is read through its API instead (see below).

```sh
levelrail-cli import deploy-yml config/deploy.yml                 # print the documents
levelrail-cli import deploy-yml config/deploy.yml --write ./iac   # save them
levelrail-cli import portainer stack.json --stack blog --write ./iac
levelrail-cli import railway railway.toml --image-repo registry.example.com/team --write ./iac
levelrail-cli import fly fly.toml --write ./iac
levelrail-cli import do-app .do/app.yaml --write ./iac
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

## Fly.io

```sh
levelrail-cli import fly fly.toml --write ./iac
```

Only the file is read. Fly secrets live on the platform, so none are listed and no value is fetched.

| Source field | Result |
| --- | --- |
| `app` | App name |
| `build.image` | App image |
| `env` | App env |
| `http_service.internal_port`, `services` | App port (one HTTP port per App) |
| `http_service` health checks, `checks` | readiness path |
| `http_service.min_machines_running` | replicas, when set |
| `vm` (first table) | CPU and memory |
| `deploy.release_command` | pre-deploy hook (dropped if it carries an inline credential) |

`build.dockerfile` and builder settings are noted for the build you create from the repository. Regions, mounts, statics, processes and any unknown top level key are listed in the notes with what to do by hand. If the file names no image, push one and pass `--image-repo`.

## DigitalOcean App Platform

```sh
levelrail-cli import do-app .do/app.yaml --write ./iac
```

Export the spec with `doctl apps spec get APP_ID > .do/app.yaml`. `SECRET` values are never read.

| Source | Result |
| --- | --- |
| `services` | one App each, with the spec domains |
| `workers` | one App each, not exposed over HTTP |
| `jobs` of kind `PRE_DEPLOY` or `POST_DEPLOY` | pre-deploy or post-deploy hook |
| `databases` (PG, MYSQL, REDIS, VALKEY) | one Database each, with the spec version |

Other job kinds (no schedule is recorded), static sites, instance sizes and databases of another engine are listed in the notes. An app that builds from source needs an image: build and push one, then set it or use `--image-repo`.

## Render Blueprint

```sh
levelrail-cli import render render.yaml --write ./iac
```

Reads services, databases and cron jobs from a `render.yaml`. Variable values are not read. To read a live Render account instead, use `levelrail-cli import platform render`. See [Migrating from Render](/migrating-from-render).

## Netlify and Cloud Run

`levelrail-cli import netlify netlify.toml` turns the build command, publish directory, redirects and headers into a static site App. `levelrail-cli import cloud-run service.yaml` reads the output of `gcloud run services describe NAME --format export`. Full guides: [Migrating from Netlify](/migrating-from-netlify) and [Migrating from Cloud Run](/migrating-from-cloud-run).

## Heroku

Heroku is read through its Platform API, GET requests only, rather than from a file.

```sh
export APP_IMPORT_SOURCE_TOKEN=YOUR_HEROKU_API_TOKEN
levelrail-cli import platform heroku --dry-run
levelrail-cli import platform heroku --emit stack --out ./iac
```

You can also pass the token with `--token-stdin`. `--dry-run` needs a logged-in control plane, `--emit stack` does not.

| Heroku | Result |
| --- | --- |
| app | an App, with config vars as env (secret-looking and add-on variables become secrets) |
| custom domains | App domains (the `herokuapp.com` domain is skipped) |
| web formation | replicas of the App |
| other process types (worker, clock) | one App each, from the same repository |
| `heroku-postgresql`, `heroku-redis` add-ons | an empty Postgres or Redis Database |

Heroku keeps no repository for the app, so import the repository with `levelrail-cli import REPO_URL` and attach it. Database data is not migrated: dump and restore it, see [Managing databases](/managing-databases). Add-ons with no managed equivalent are listed as warnings. `import platform` also reads Vercel and Render accounts, see [Migrating from Vercel](/migrating-from-vercel).

The size limit for the input file is `LEVELRAIL_IMPORT_FILE_MAX_BYTES`.
