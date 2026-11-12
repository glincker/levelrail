# App with a worker and Redis

A web app and a background worker that share a Redis queue.

## What it creates

- a Project and a Stack
- a Redis 7.4 database `queue`
- an App and an App `NAME-worker`, both reading `REDIS_URL` from the database

## Variables

Set them with `--set key=value` on `stack init`.

- `name`: the stack, project and app name (default `worker-queue`).
- `image`: the prebuilt image to run (default `ghcr.io/acme/app:latest`). Apply runs prebuilt images only, so push it from CI before the first apply.
- The worker runs the same image with `WORKER=1`. Edit its image in `apps.yaml` if you build it separately.

## Required secrets

Names only, listed in SECRETS.md. Pass each value at apply time, never write it into a file.

## How to apply

```bash
levelrail-cli stack init worker-queue --dir infra/stacks/NAME --set name=NAME
levelrail-cli stack plan NAME -f infra/stacks/NAME
levelrail-cli stack apply NAME -f infra/stacks/NAME --yes --secret API_TOKEN=env:API_TOKEN
```

Stacks need `APP_EXPERIMENTAL=iac`. See the Stacks docs for approvals, rollback and drift.
