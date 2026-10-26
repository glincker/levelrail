# Preview environment

A small app and database for one pull request. Use a different name per PR so each gets its own stack.

## What it creates

- a Project and a Stack named after the PR
- a Postgres 16 database `main`
- an App with `DATABASE_URL` from the database
- a Domain for the preview host

## Variables

Set them with `--set key=value` on `stack init`.

- `name`: the stack, project and app name (default `preview-env`).
- `image`: the prebuilt image to run (default `ghcr.io/acme/web:pr-1`). Apply runs prebuilt images only, so push it from CI before the first apply.
- `domain`: the preview host (default `NAME.example.com`). Use a name such as `pr-123.preview.example.com`.

## Required secrets

Names only, listed in SECRETS.md. Pass each value at apply time, never write it into a file.

## How to apply

```bash
levelrail-cli stack init preview-env --dir infra/stacks/NAME --set name=NAME --set name=pr-123 --set domain=pr-123.preview.example.com
levelrail-cli stack plan NAME -f infra/stacks/NAME
levelrail-cli stack apply NAME -f infra/stacks/NAME --yes
```

Stacks need `APP_EXPERIMENTAL=iac`. See the Stacks docs for approvals, rollback and drift.
