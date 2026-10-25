# Web app with Postgres

A web app on a prebuilt image, a Postgres database injected as DATABASE_URL, and a domain.

## What it creates

- a Project and a Stack
- a Postgres 16 database `main`
- an App on port 3000, with `DATABASE_URL` read from the database
- a Domain for the app

## Variables

Set them with `--set key=value` on `stack init`.

- `name`: the stack, project and app name (default `web-app-postgres`).
- `image`: the prebuilt image to run (default `ghcr.io/acme/web:latest`). Apply runs prebuilt images only, so push it from CI before the first apply.
- `domain`: the host to serve (default `NAME.example.com`). Point its DNS at this server.

## Required secrets

Names only, listed in SECRETS.md. Pass each value at apply time, never write it into a file.

## How to apply

```bash
levelrail-cli stack init web-app-postgres --dir infra/stacks/NAME --set name=NAME --set domain=app.example.com
levelrail-cli stack plan NAME -f infra/stacks/NAME
levelrail-cli stack apply NAME -f infra/stacks/NAME --yes --secret SESSION_SECRET=env:SESSION_SECRET
```

Stacks need `APP_EXPERIMENTAL=iac`. See the Stacks docs for approvals, rollback and drift.
