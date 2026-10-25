# Agent app with a scheduled run

An app with a nightly scheduled job, a Postgres database and secret references for model and webhook credentials.

## What it creates

- a Project and a Stack
- a Postgres 16 database `main`
- an App on port 8080 with `DATABASE_URL` from the database
- a job `nightly-run` at 03:00 that runs `node scripts/agent-run.js` in the app image

## Variables

Set them with `--set key=value` on `stack init`.

- `name`: the stack, project and app name (default `agent-app`).
- `image`: the prebuilt image to run (default `ghcr.io/acme/agent:latest`). Apply runs prebuilt images only, so push it from CI before the first apply.

## Required secrets

Names only, listed in SECRETS.md. Pass each value at apply time, never write it into a file.

## How to apply

```bash
levelrail-cli stack init agent-app --dir infra/stacks/NAME --set name=NAME
levelrail-cli stack plan NAME -f infra/stacks/NAME
levelrail-cli stack apply NAME -f infra/stacks/NAME --yes --secret MODEL_API_KEY=env:MODEL_API_KEY --secret TOOL_WEBHOOK_SECRET=env:TOOL_WEBHOOK_SECRET
```

Stacks need `APP_EXPERIMENTAL=iac`. See the Stacks docs for approvals, rollback and drift.
