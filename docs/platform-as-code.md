---
title: "Platform as code"
tag: "Experimental"
description: Describe projects, environments, apps, domains, databases and more as YAML files, review the plan, and apply it from the CLI, the dashboard, MCP or CI.
"og:url": "https://docs.levelrail.com/platform-as-code"
---

Write the state you want as YAML, run `levelrail-cli apply`, and the control plane converges to it. This page is the entry point; the details live in the [IaC section](/iac/index).

Everything goes through the ordinary REST API with your own token. If a policy denies you an app, that one item is reported as denied and nothing else is touched by it. There is no separate write path.

<Warning>
**Experimental**

Platform as code is off by default. Set `APP_EXPERIMENTAL=iac` on the control plane, and in the shell where you run the CLI or the MCP server, to enable `apply`, `diff` and `export`. See [Experimental features](/experimental-features).
</Warning>

## Quick start

```bash
# Capture what exists today
levelrail-cli export --project shop -o infra/

# See what applying the files would change, exit 2 if anything would
levelrail-cli apply -f infra/ --dry-run --exit-code

# Apply, tagging what this source creates so prune can find it later
levelrail-cli apply -f infra/ --source infra-repo --yes
```

## Where to read next

| You want to | Page |
| --- | --- |
| Get one app running from a file in five minutes | [IaC quickstart](/iac/index) |
| Copy a working file | [Templates](/iac/templates) |
| See the fields of every kind | [Reference](/iac/reference), with [worked examples](/iac/resources) |
| Understand plan, approve, apply, prune and drift | [How it works](/iac/how-it-works) |
| Run it from CI | [IaC in CI](/iac/ci) |
| Fix an error code | [Errors](/iac/errors) |
| Revisions, rollback, approvals and drift policy | [Stacks and GitOps](/stacks) |

## Secrets

Documents never contain secret values.

- Reference a secret by name: `API_KEY: { secretRef: API_KEY }` or <span v-pre>`API_KEY: ${{ secrets.API_KEY }}`</span>. Secrets are stored per app under the env var name, so the reference must match it.
- A literal value on a key that looks like a secret (`PASSWORD`, `TOKEN`, `API_KEY`, and similar) or a value shaped like a known token is refused with an error. The error names the key and line and never echoes the value.
- To create or update the value at apply time, pass it explicitly from the environment or a file:

```bash
levelrail-cli apply -f infra/ \
  --secret API_KEY=env:PROD_API_KEY \
  --secret web/DB_PASSWORD=file:./db-password.txt
```

Use `NAME` to apply to every app referencing that name, or `app/NAME` for one app. The plan only ever shows that a secret is set, never the value. A secret that is referenced but has no value yet is reported as a warning.

- <span v-pre>`${{ env.NAME }}`</span> placeholders in plain values are filled at apply time from the sources described in [Providing values safely](#providing-values-safely). Export uses them when you pass `--include-env-values=false`.

Export never writes a secret value. Secret backed variables are written as `secretRef`, and a plain variable whose name or value looks like a secret is written as a <span v-pre>`${{ env.NAME }}`</span> placeholder with a warning that also says how to supply it.

## Providing values safely

A resource file can be written by someone else: a teammate, a template, a pull request. So the CLI never fills a <span v-pre>`${{ env.NAME }}`</span> placeholder from your shell unless you said it may. A placeholder is resolved from, in this order:

1. `--var NAME=VALUE` (repeatable).
2. `--var-file PATH` (repeatable): `KEY=VALUE` lines in dotenv format, with `#` comments, quotes and an optional `export` prefix. A line that is not a valid `NAME=VALUE` is an error naming the line. Keep the file readable only by you (`chmod 600`), and out of the repository.
3. `--allow-env NAME[,NAME...]` (repeatable): reads exactly those names from your environment. A trailing `*` allows a prefix, for example `--allow-env 'APP_*'`.

Anything else fails before the CLI contacts the control plane, and the error lists each unresolved name with the three ways to provide it. Nothing from the files is sent while a placeholder is unresolved. Placeholders on full line `#` comments are ignored.

Names that look like credentials are never covered by a wildcard: cloud and CI prefixes (`AWS_*`, `AZURE_*`, `GOOGLE_*`, `GCP_*`, `S3_*`, `SSH_*`, `ACTIONS_*` and similar), `GITHUB_TOKEN`, `GH_TOKEN`, `NPM_TOKEN`, and any name containing `TOKEN`, `SECRET`, `PASSW`, `CREDENTIAL`, `PRIVATE` or `_KEY`. If a file really needs one, name it exactly: `--allow-env AWS_REGION`. A shared file that quietly asks for <span v-pre>`${{ env.AWS_SECRET_ACCESS_KEY }}`</span> therefore fails instead of sending your key.

The control plane never reads its own environment for placeholders. The dashboard and the MCP tools cannot pass values at all, so files applied there must not use <span v-pre>`${{ env.NAME }}`</span>.

```bash
levelrail-cli apply -f infra/ --var REGION=eu-west-1
levelrail-cli apply -f infra/ --var-file ./prod.vars
levelrail-cli apply -f infra/ --allow-env REGION,LOG_LEVEL
```

## Export

```bash
levelrail-cli export                       # everything you can read, to stdout
levelrail-cli export --project shop -o infra/
levelrail-cli export --app web -o -
levelrail-cli export --project shop --include-env-values=false -o infra/
```

Output is stable: documents are ordered by kind then name, keys are sorted, and there are no timestamps or generated ids. Export, apply, export gives identical files, and applying an export to the control plane it came from changes nothing. Database connections are exported as `from` references. Live settings the format cannot express (bind mounts, egress policies, vault env) are listed as warnings on stderr.

## Apply

```text
levelrail-cli apply -f file|dir|- [--dry-run] [--prune] [--yes] [--project P]
                    [--source NAME] [--secret ...] [--var NAME=VALUE]
                    [--var-file PATH] [--allow-env NAME[,NAME...]]
                    [--no-deploy] [--continue-on-error] [--exit-code]
levelrail-cli diff  -f dir        # the same as apply --dry-run --exit-code
```

`-f` may be repeated and accepts a file, a directory (searched recursively for `.yaml` and `.yml`) or `-` for stdin.

### How it runs

Everything is validated first, then planned against live state, then executed item by item. Apply is safe to repeat. The full flow is on [How it works](/iac/how-it-works). The plan carries a hash so an apply only proceeds if live state still matches what was reviewed.

- A plan that contains an error item (a missing project, a read you are not permitted to make) applies nothing unless `--continue-on-error` is set.
- Lists and maps on an App (`domains`, `tags`, `env`, `labels`, secret names) are additive. Live entries the files do not mention are kept and shown as `keep` in the plan. Everything else on a declared resource is authoritative.

### Deploys

Apply saves desired state and the reconciler acts on it. A changed image is a deploy, by the same path as any other image change. When an env var or secret changes on a running app, apply restarts it so the change takes effect. `--no-deploy` skips that restart; the app keeps running with the old values until you restart it. It does not hold back an image change, which the control plane starts on save. An app is only touched when its plan is not empty.

### Prune

`--prune` deletes things that are in the control plane but not in your files, and it is deliberately narrow:

- It requires `--source NAME`. Apps created or updated by an apply with a source carry a `managed-by:NAME` tag.
- Only apps carrying that source's tag are ever deleted. Anything created by hand, or by another source, is never touched.
- On a managed app it also removes load balancers and extra domains, tags, env vars and labels that are absent from the files.
- Projects, environments, databases, pipelines and alert rules are never pruned.
- `--prune` asks for confirmation unless `--yes` is set.

### Exit codes

| Code | Meaning |
| --- | --- |
| 0 | No changes, or the changes were applied |
| 1 | An error: invalid files, a failed or denied item, an unreachable server |
| 2 | Changes are pending (`--dry-run --exit-code`, and `diff`) |

### Drift

`levelrail-cli diff -f infra/` shows how live state differs from the files and exits 2 when it does. Run it on a schedule to catch hand edits.

## Dashboard and MCP

**Settings, Infrastructure as code** lets you paste or upload YAML, see the plan with diffs, and apply with a confirmation. It also has an Export button per project and per app.

MCP has two tools: `plan_apply` (read only, a dry run) and `apply_resources` (changes state, subject to the usual approval and IAM rules).

## Git workflow

There is no sync engine to run. Keep the files in a repository and use the CLI from CI: a pull request shows the plan, merging applies it. Copy the workflow from [IaC in CI](/iac/ci).

## Adopting an existing project

```bash
levelrail-cli export --project shop -o infra/
git add infra && git commit -m "infra: import shop"
levelrail-cli apply -f infra/ --dry-run --exit-code     # exits 0: the files match live state
```

From there, change files and let CI apply them. To let prune clean up later, run one apply with `--source infra-repo` so the apps are tagged as managed by it.

## Driving an install from a repo

This section covers the install-wide parts: values per environment, Node and Setting documents, and install stacks. The sections above cover apps, projects and the other resource kinds.

### Values per environment

```bash
levelrail-cli apply -f infra/ --values values/prod.yaml
levelrail-cli stack plan shop-prod -f infra/stacks/shop-prod --values values/prod.yaml
```

`--values` reads a flat YAML map (`IMAGE_TAG: v1.4.2`) that fills `${{ env.NAME }}` placeholders. It is repeatable, and a later file wins. A placeholder is filled from the first source that has its name, in this order:

1. `--var NAME=VALUE`
2. `--values FILE`
3. `--var-file PATH`
4. `--allow-env NAME[,NAME...]`, read from the environment

Only scalar values are accepted. A key that looks like a credential (a name containing `TOKEN`, `SECRET`, `PASSW`, `CREDENTIAL`, `PRIVATE`, `API_KEY` or `_KEY`) is refused. The error names the key and the file, never the value. Pass such a value with `--secret` instead.

### Existing secrets

```yaml
version: 1
kind: App
metadata: { name: web }
spec:
  service:
    build: { type: image, image: "ghcr.io/acme/web:1.2.3" }
    port: 3000
    env:
      DB_PASSWORD: { secretRef: DB_PASSWORD, existing: true }
```

`existing: true` points at a secret that is already stored for the app. Apply never writes, overwrites or deletes it, even when `--secret` is passed for the same name. The plan fails if the app has no secret with that name. Use it for values a person set in the dashboard. A stack token cannot pass `--secret`, so this is how a CI apply refers to a secret.

### Node documents

```yaml
version: 1
kind: Node
metadata: { name: worker-1 }
spec:
  labels: { zone: a }
  taints:
    - { key: gpu, value: "true", effect: NoSchedule }
```

- The document matches an existing node by its display name. A name that matches no node, or more than one, is a plan error.
- Labels and taints that the document leaves out are not managed. `labels: {}` or `taints: []` clears that set.
- Apply saves them through the node routes, which need a root token. It never creates, deletes or prunes a node. Removing the document leaves the node as it is.
- Nodes join with an enrollment token, not with a document.

### Setting documents

```yaml
version: 1
kind: Setting
metadata: { name: heartbeat.job_max_attempts }
spec:
  value: 5
```

- The name is the runtime setting's registry key. The value goes through the same validator as the API, so a value the dashboard refuses is a plan error.
- A declared setting is authoritative. A value that only comes from the environment or the default is saved, so a later env change does not move it.
- A restart setting warns that it takes effect after a restart. Apply never restarts the control plane.
- A setting that holds a credential is refused in files. Set it from the dashboard or the CLI.
- Removing the document keeps the saved value and only releases it from the stack.
- A change that weakens a security protection (turns one off, shortens a duration or raises a limit) is a plan error that names `allow_weaken`. File applies do not accept `allow_weaken` yet, so a human root operator makes that change in the dashboard or the CLI.

### Install-scoped stacks

An install stack owns only Node and Setting documents and has no project. A project stack that holds either kind gets a warning.

- **Creating it needs a human root session.** The API refuses a token for install scope. Create it with `levelrail-cli stack create NAME --scope install`, run as a human root user. The install scope has no `--project`.
- **Approval is on by default.** Applies wait as pending revisions until another administrator approves them. Change it with `levelrail-cli stack settings NAME --require-approval=false` (root only).
- **Drift policy is `lock` by default**, the same mode described under [Drift](/stacks#drift). Set `--drift warn` to relax it (root only).

### Export as code and the Import YAML preview

```bash
levelrail-cli export --project shop -o infra/
levelrail-cli export --app web -o -
```

Export writes projects and apps as documents. The dashboard has an Export button per project and per app, under Settings, Infrastructure as code.

The app, database and node pages have an **Import YAML** button. It opens a preview of a pasted or uploaded file: the plan and its changes, with nothing applied. Apply stays in the CLI or on the Infrastructure as code page, where the plan is confirmed first.

### Not supported yet

- `levelrail-cli export` has no node option, so Node documents are not written out. The control plane's export API takes one, but the CLI does not expose it yet.
- Database placement is not part of a stack. The database API stores only a node selector, and no stack manages it.
- A file cannot set `allow_weaken`, and a stack token cannot pass `--secret`.
- Plain `apply` takes a token from `APP_API_TOKEN`. The OIDC exchange (`POST /api/v1/ci/stack-oidc-exchange`) returns a stack token for `stack plan` and `stack apply` only.

## Not in scope

Git sync is a recipe, not an engine. Backup schedules, egress policies, node placement, registry credentials and vault env references are not expressible yet and are left as they are.

## Next steps

<CardGroup cols={2}>
<Card title="app.yaml reference" href="/app-spec-reference">

The service fields an App document embeds.

</Card>
<Card title="Pipelines" href="/pipelines">

The pipeline definition format.

</Card>
<Card title="Identity and access" href="/identity-and-access">

Tokens and per resource policies.

</Card>
<Card title="Migrating from other platforms" href="/migrate-to-levelrail">

Import from Coolify, Dokploy or CapRover.

</Card>
</CardGroup>
