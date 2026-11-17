---
title: "Contribute to the template registry"
description: How to add a template folder to registry/templates, the rules the validator enforces, and how CI smoke-tests your change.
"og:url": "https://docs.levelrail.com/registry-contributing"
---

The registry is a folder of templates in the repository. Each template is one folder, and a pull request that adds or changes a folder is checked by CI. For the older manifest route see [Add a template](/add-a-template).

## Add a template folder

Create `registry/templates/<id>/` with two files:

```text
registry/templates/<id>/template.yaml
registry/templates/<id>/compose.yaml
```

`<id>` is lowercase kebab case (`my-app`) and must equal the `id:` field. `manifest.compose.yaml` is optional and only needed when the install form uses `${KEY}` prompts.

`template.yaml` needs `name`, `slogan`, `category` and `documentation_url`. `compose.yaml` is the Compose file that runs the app.

## What the validator checks

- The folder name is lowercase kebab case and equals `id`.
- `name`, `slogan`, `category`, `documentation_url` and the compose file are present.
- `category` is one the catalog already uses.
- The compose file parses and passes the same checks the product uses.
- Every image has a version tag or a digest. `latest` and a missing tag are errors.
- Every service has a `healthcheck`.
- No `privileged: true`, no `network_mode: host`, no mount of `/`.
- The Docker socket is never mounted. Levelrail refuses it at deploy time, so a template that needs it cannot be listed.
- Published ports are container-only, for example `"3000"`, not `"3000:3000"`.
- `icon`, when set, is a lowercase slug matching `^[a-z0-9-]+$`. It is not checked against the icon library yet.

## Run it locally

```bash
go run ./scripts/validate-registry                           # legacy summary, never fails
go run ./scripts/validate-registry -strict                   # every folder, fails on any problem
git diff --name-only origin/main > /tmp/changed.txt
go run ./scripts/validate-registry -changed /tmp/changed.txt # only the folders you touched
```

Add `-json` for machine output. Each problem prints as `registry/templates/<id>: message`, and the exit code is 1 when there is any.

Folders that predate these rules are not enforced. Touching one means it must pass.

## How the smoke test runs

For each changed folder CI renders the template through the Levelrail engine (`go run ./scripts/registry-render -dir registry/templates/<id>`), which fills magic variables like `$SERVICE_PASSWORD_32_X` and declares named volumes, then runs `docker compose up --wait --wait-timeout 240` on the result and `down -v`. The run is capped at 8 folders per change. A template whose images have no `linux/arm64` build gets a warning, not a failure. Reproduce locally by redirecting the render output to a file and passing it to `docker compose -f`.

A template listed in `registry/legacy.txt`, or already present at the base commit of the change, is legacy: its smoke failure is a warning. A template new in the change must pass. An engine rejection in the render step is always a failure.
