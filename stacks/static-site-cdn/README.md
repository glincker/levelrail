# Static site

A static site served by an nginx image, with a domain. Bake your built files into the image.

## What it creates

- a Project and a Stack
- an App running the image on port 80
- a Domain for the app

## Variables

Set them with `--set key=value` on `stack init`.

- `name`: the stack, project and app name (default `static-site-cdn`).
- `image`: the prebuilt image to run (default `nginx:1.27-alpine`). Apply runs prebuilt images only, so push it from CI before the first apply.
- `domain`: the host to serve (default `NAME.example.com`). Point its DNS at this server.

## Required secrets

Names only, listed in SECRETS.md. Pass each value at apply time, never write it into a file.

## How to apply

```bash
levelrail-cli stack init static-site-cdn --dir infra/stacks/NAME --set name=NAME --set domain=www.example.com
levelrail-cli stack plan NAME -f infra/stacks/NAME
levelrail-cli stack apply NAME -f infra/stacks/NAME --yes
```

Stacks need `APP_EXPERIMENTAL=iac`. See the Stacks docs for approvals, rollback and drift.
