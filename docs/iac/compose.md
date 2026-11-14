---
title: "Compose projects as code"
tag: "Beta"
description: Point an App document at an existing Docker Compose file instead of copying it. Plan, apply, import and adopt, with the exact limits strict mode enforces.
---

An App can reference a Docker Compose file in place. The compose file stays the one definition of your services; the App document adds what Levelrail owns: project, environment, tags, domains and the values for `${VAR}`.

## The shape

```yaml
# app.yaml, next to the folder that holds shop/compose.yaml
version: 1
kind: App
metadata:
  name: shop
spec:
  project: shop
  compose:
    path: shop/compose.yaml     # relative to the folder you apply
    profiles: [jobs]            # optional compose profiles to enable
    env:                        # values for ${VAR} in the file, never read from your shell
      SHOP_TITLE: My shop
      DB_PASSWORD: ${{ secrets.DB_PASSWORD }}
```

`spec.compose` and `spec.service` are mutually exclusive. `path` must be relative, inside the folder you apply, with no `..` and no symlink on the way. A working copy lives in `examples/iac/compose-ref/`.

## Turn it on

The feature is beta and off by default. Enable the runtime setting `iac.compose_refs`:

```bash
levelrail-cli settings runtime set iac.compose_refs true
```

Size caps are `iac.compose_max_file_kb`, `iac.compose_max_files` and `iac.compose_max_total_kb`.

## Apply

`levelrail-cli apply -f DIR` reads the documents, finds every `spec.compose.path`, and sends those files as attachments next to the documents. The compose file is never parsed as a resource document. Plan first:

```bash
levelrail-cli apply -f ./myproject --dry-run
levelrail-cli apply -f ./myproject --yes
```

A compose App deploys through the compose deploy path on every apply, so `no_deploy` is refused (`compose_no_deploy_unsupported`). `stack plan` and `stack apply` do not send attachments yet, so use `apply` for compose Apps.

## What strict mode refuses

A referenced file comes from a repository, so it runs under a stricter parser than a file you paste into the dashboard. Every refusal has a stable code (see [errors](/iac/errors)):

| Refused | Code | Why |
| --- | --- | --- |
| `build:` | `compose_unsupported_key` | An attachment has no build context. Build and push an image first. |
| `env_file:` | `compose_unsupported_key` | The CLI would have to upload `.env` files, which hold secrets. Declare values in `spec.compose.env`. |
| `include`, `extends.file`, unknown top level keys | `compose_unsupported_key` | Only the one file you reference is read. |
| bind mounts | `compose_rejected` | A bind mount reaches the host disk. Use a named volume. |
| `privileged`, host network or pid, device mounts, socket mounts, `cap_add` | `compose_rejected` | IaC compose Apps never get host access. |
| literal secrets in `environment`, `labels` or `command` | `compose_secret_literal` | Use `${NAME}` and declare it in `spec.compose.env`. |
| undeclared `${VAR}` | `compose_var_missing` | The host environment is never read. Declare it or give it a default. |

Plan reports every refusal in one pass, with the service and key but never a value.

## Import a compose project

`levelrail-cli import compose-ref --reference [dir]` emits one App per compose file that points at it. The compose file is never copied or rewritten, and `--write DIR` must be the folder that holds the compose files so every `path` stays relative without `..`.

```bash
levelrail-cli import compose-ref ./myproject --reference --stack shop              # dry run
levelrail-cli import compose-ref ./myproject --reference --stack shop --write ./myproject
```

The output is validated with the real loader and the strict parser before it is printed or written. A file strict mode would refuse is not emitted; the command prints the code for each refusal as a note. Variables become `${{ env.NAME }}` placeholders, or `${{ secrets.NAME }}` for names that look like secrets. Values are never read. Existing files are kept without `--force`. The file name must look like `compose.yaml` or `docker-compose.yml`.

## Adopt a running compose app

If the project already runs on Levelrail, make the file match what is deployed so the first plan is empty:

```bash
levelrail-cli stack adopt-from-export shop -f ./myproject
```

It writes the current deployed revision to the path the referencing App names, refuses to overwrite an existing file without `--force`, then runs a plan and tells you whether it is empty. The stored revision is the file as it was deployed, before interpolation, so no env value is written.

## Limits

- External `secretRef` sources in `spec.compose.env` are refused for now; use a literal, a `${{ env.NAME }}` placeholder or a `${{ secrets.NAME }}` reference.
- `env_file`, bind mounts, `build:` and `include` are refused.
- There is no git form (`spec.compose.git`) yet. Reference a file in the folder you apply.
- A compose App is one compose project: there is no per-service selector. Use profiles for subsets.
