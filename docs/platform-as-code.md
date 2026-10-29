---
title: "Platform as code"
tag: "Experimental"
description: Describe projects, environments, apps, domains, databases and more as YAML files, review the plan, and apply it from the CLI, the dashboard, MCP or CI.
"og:url": "https://docs.levelrail.com/platform-as-code"
---

Write the state you want as YAML, run `levelrail-cli apply`, and the control plane converges to it. The same files can be exported from a live control plane, reviewed in a pull request, and applied from CI.

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

## Document format

A file holds one or more YAML documents separated by `---`. Every document has the same envelope:

```yaml
version: 1              # schema version, the only accepted value
kind: App               # Project, Environment, Tag, Database, App, Domain, LoadBalancer, Pipeline, AlertRule
metadata:
  name: web             # unique per kind (see each kind below)
  labels: {}            # free form, for your own tooling; not stored
spec: {}                # kind specific
```

The [JSON Schema](https://github.com/glincker/levelrail/blob/main/docs/schemas/resources.schema.json) is also served at `GET /api/v1/apply/schema`. Point your editor at it for completion. Errors are reported with file and line: `infra/web.yaml:14: additional properties 'prot' not allowed (spec.service.prot)`.

An App's `spec.service` is the app.yaml service definition, validated by the same schema. The document schema embeds the app.yaml definitions unchanged, with two additions: an env value may be `{ secretRef: NAME }`, or `{ from: { database: NAME, field: url } }` to read a value from a Database (see [Database references](#database-references)).

### Project

```yaml
version: 1
kind: Project
metadata: { name: shop }
spec:
  env: { REGION: eu }     # shared, non secret variables
```

### Environment

Environments belong to a project. Identity is project plus name.

```yaml
version: 1
kind: Environment
metadata: { name: production }
spec:
  project: shop
  protected: true
  env: { LOG_LEVEL: info }
```

### Tag

```yaml
version: 1
kind: Tag
metadata: { name: frontend }
```

### Database

```yaml
version: 1
kind: Database
metadata: { name: main-db }
spec:
  engine: postgres
  version: "16"
  project: shop
```

Engine and version cannot change through apply: databases hold data, so a mismatch is reported as an error and you recreate it deliberately. Backups, resources and public access are still managed with their own commands.

### App

```yaml
version: 1
kind: App
metadata: { name: web }
spec:
  project: shop
  environment: production
  tags: [frontend]
  service:
    build: { type: image, image: "ghcr.io/acme/web:1.2.3" }
    port: 3000
    domains: [app.example.com]
    replicas: 2
    strategy: blue-green
    resources: { memory: 512Mi, cpu: 0.5 }
    health:
      readiness: { path: /healthz, interval: 5s, timeout: 2s }
    env:
      LOG_LEVEL: info
      API_KEY: { secretRef: API_KEY }
      DB_PASSWORD: ${{ secrets.DB_PASSWORD }}
      DATABASE_URL: { from: { database: main-db, field: url } }
    volumes:
      - name: data
        path: /data
    jobs:
      - name: nightly-report
        schedule: "0 3 * * *"
        command: [node, scripts/report.js]
        timeout: 10m
```

Apply manages apps that run a prebuilt image (`build.type: image`). Git builds are configured with a git source, not here. `port`, `health.readiness` and `health.liveness` are applied as written. Not supported yet, and rejected with a clear error: host-path volumes, egress policies, GPU requests, registry credentials and `vault` env references. A live app that uses them keeps them when you apply; the plan warns.

#### Database references

`{ from: { database: NAME, field: FIELD } }` points an env var at a Database's connection details. `field` is one of `url` (the default), `host`, `port`, `username`, `password` or `database`. The app.yaml string form `{ from: main-db.url }` is accepted too.

The value is never read by apply. Apply records the reference (the same connection the dashboard's Connect button makes), and the control plane fills in the value when it creates the container. So the files, the plan, the plan hash and stack revisions only ever hold `main-db.url`, never the password or the URL. The database must exist or be declared in the same files; the plan shows the reference as `connections.DATABASE_URL: main-db.url` and lists the database under `dependsOn`, and databases are always applied before apps. If the live app also has a literal or secret value under the same name, the plan warns so you can remove it.

#### Volumes

`service.volumes` lists named volumes (`name`, `path`). Apply matches them by name through `PUT /api/v1/apps/{name}/volumes`: a new name is attached, a changed path is updated, and the plan shows `volumes.data: /data`. Host-path bind mounts are rejected, so the files can never mount a host directory. Volumes that the files do not list are kept, and detached only by prune on an app the source manages. Detaching never deletes the Docker volume or its data; deleting data is a separate, explicit action. Mounts are fixed when the container is created, so a volume change restarts the app unless `--no-deploy` is set, and the app shows a pending config change until then. An app document without a `volumes` key never touches the app's volumes.

#### Published ports and restart

`service.published_ports` binds ports straight on the host beside the routed `port`, for UDP services such as DNS or WireGuard. Each entry is `container_port`, `host_port` and `protocol` (`tcp`, the default, or `udp`), the same shape as app.yaml and the API. A port out of range, a protocol other than tcp or udp, or the same host port and protocol twice is rejected before the plan. Apply runs the same host port admission as `POST /api/v1/apps`: a port another app already publishes, a reserved port, or a service placed on a remote node fails the apply with the API's own message, and nothing is saved for that app.

`service.port` may be left out when the service has at least one `published_ports` entry (a UDP-only DNS or VPN app). A service with `domains` still needs a `port`, since the proxy routes HTTP to it, and a service with neither a `port` nor `published_ports` is rejected.

`service.restart` takes `no`, `on-failure` or `on-failure:N` for a service that runs to completion (a migration or seed job), and `always` or `unless-stopped`. The plan shows changes as `publishedPorts` and `restart`, an out-of-band change to either is drift, and revisions and rollbacks keep both. An app document without `published_ports` or `restart` leaves the live values alone; `published_ports: []` clears the ports.

```yaml
version: 1
kind: App
metadata: {name: dns}
spec:
  service:
    build: {type: image, image: "ghcr.io/acme/dns:1.0.0"}
    port: 8080
    published_ports:
      - {container_port: 53, host_port: 53, protocol: udp}
      - {container_port: 53, host_port: 53, protocol: tcp}
```

#### Rollout policy

`service.rollout` sets `max_surge`, `max_unavailable` and `min_available` (each an integer such as `2` or a percent such as `25%`). Apply saves it through `PUT /api/v1/apps/{name}/rollout`, the same policy a spec deploy and `levelrail-cli rollout status` read, and export prints it back. The plan shows `rollout.min_available: 1 -> 2`. Fields left out inherit the `APP_ROLLOUT_*` env defaults. An app document without a `rollout` key never touches the saved policy; `DELETE /api/v1/apps/{name}/rollout` returns an app to the defaults.

#### Scheduled jobs

`service.jobs` is the app.yaml jobs list. Apply matches jobs by name: a new name is created, a changed one is updated in place (its run history is kept). Jobs created in the dashboard or CLI that the files do not list are kept, and removed only by prune on an app the source manages. A job that the app's own app.yaml declares is left alone with a warning, so each job has one owner. An app document without a `jobs` key never touches the app's jobs.

### Domain

Adds a hostname to an app. It is additive with the App's own `domains`, and a hostname can belong to one app only.

```yaml
version: 1
kind: Domain
metadata: { name: www.example.com }
spec: { app: web }
```

### LoadBalancer

`metadata.name` is the app. The spec is the app.yaml `loadbalancer` block.

```yaml
version: 1
kind: LoadBalancer
metadata: { name: web }
spec: { algorithm: least_conn }
```

### Pipeline

```yaml
version: 1
kind: Pipeline
metadata: { name: ci }
spec:
  app: web
  enabled: true
  yaml: |
    version: 1
    jobs:
      test:
        image: golang:1.22
        steps:
          - run: go test ./...
```

The pipeline definition is validated with the same checks as `pipelines validate`, and issues point at the line inside your file. A pipeline synced from the repository cannot be changed here.

### AlertRule

```yaml
version: 1
kind: AlertRule
metadata: { name: high-cpu }
spec:
  app: web
  type: threshold
  metric: cpu_percent
  comparator: ">"
  threshold: 90
  for: 5m
  channel: ops          # a notification channel, by name
```

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

<Steps>
<Step title="Validate everything first">

Every document is checked against the schema and the semantic rules. If any document has an issue, nothing is planned and nothing is applied.

</Step>
<Step title="Plan">

Live state is read through the API with your permissions, and a plan is computed: create, update (with a field level diff), delete (only with `--prune`) or no change. Env values are never shown, only `(hidden)`. Items are ordered by dependency: project, environment, tag, database, app, domain, [load balancer](/load-balancing), pipeline, alert rule.

</Step>
<Step title="Apply">

Each item is executed through the existing endpoint. Apply is safe to repeat and to resume: a re-run recomputes the plan and only does what is still missing.

</Step>
</Steps>

A few rules to know:

- Validation and planning are all or nothing. Execution is per item: by default the first failure stops the run and later items are reported as skipped. With `--continue-on-error` the rest proceed.
- A plan that contains an error item (a missing project, a read you are not permitted to make) applies nothing unless `--continue-on-error` is set.
- Per resource permissions apply to each item. A denied item is reported as `denied` with its key.
- The plan carries a hash. The CLI, dashboard and MCP pass it back so an apply only proceeds if live state still matches what was reviewed.
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

There is no sync engine to run. Keep the files in a repository and use the CLI from CI. A pull request shows the plan, and merging applies it.

```yaml
# .github/workflows/infra.yml
name: infra
on:
  pull_request:
    paths: ["infra/**"]
  push:
    branches: [main]
    paths: ["infra/**"]

jobs:
  plan:
    if: github.event_name == 'pull_request'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install the CLI
        run: ./ci/install-levelrail-cli.sh   # download levelrail-cli from GitHub Releases
      - name: Plan
        env:
          APP_API_URL: ${{ secrets.LEVELRAIL_API_URL }}
          APP_API_TOKEN: ${{ secrets.LEVELRAIL_READ_TOKEN }}
          REGION: ${{ vars.REGION }}
        run: levelrail-cli apply -f infra/ --source infra-repo --allow-env REGION --dry-run --exit-code || [ $? -eq 2 ]

  apply:
    if: github.event_name == 'push'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install the CLI
        run: ./ci/install-levelrail-cli.sh
      - name: Apply
        env:
          APP_API_URL: ${{ secrets.LEVELRAIL_API_URL }}
          APP_API_TOKEN: ${{ secrets.LEVELRAIL_WRITE_TOKEN }}
          PROD_API_KEY: ${{ secrets.PROD_API_KEY }}
          REGION: ${{ vars.REGION }}
        run: levelrail-cli apply -f infra/ --source infra-repo --yes --secret API_KEY=env:PROD_API_KEY --allow-env REGION
```

Pass placeholder values with `--var` or name them with `--allow-env`; the runner's own credentials (`GITHUB_TOKEN`, `ACTIONS_*`, cloud keys) are never read unless you name them exactly. The plan job needs the same `--var` or `--allow-env` flags as the apply job.

Planning needs only a read token. Use a separate token with write ability for the apply job. The install step is yours to supply: download the `levelrail-cli` release binary for the runner, the way the bundled deploy action does. See [GitHub Actions](/github-actions) for token setup and that action.

For numbered revisions, rollback, approvals and drift checks on the same files, see [Stacks and GitOps](/stacks). It is experimental and behind `APP_EXPERIMENTAL=iac`.

## Examples

### A single app

```yaml
version: 1
kind: App
metadata: { name: docs }
spec:
  service:
    build: { type: image, image: "ghcr.io/acme/docs:2.0.0" }
    port: 8080
    domains: [docs.example.com]
```

### A multi environment project with a database and domains

```yaml
version: 1
kind: Project
metadata: { name: shop }
---
version: 1
kind: Environment
metadata: { name: staging }
spec: { project: shop }
---
version: 1
kind: Environment
metadata: { name: production }
spec: { project: shop, protected: true }
---
version: 1
kind: Database
metadata: { name: shop-db }
spec: { engine: postgres, version: "16", project: shop }
---
version: 1
kind: App
metadata: { name: shop-web-staging }
spec:
  project: shop
  environment: staging
  service:
    build: { type: image, image: "ghcr.io/acme/shop:1.9.0-rc1" }
    port: 3000
    domains: [staging.shop.example.com]
    env:
      DATABASE_URL: { from: { database: shop-db } }
---
version: 1
kind: App
metadata: { name: shop-web }
spec:
  project: shop
  environment: production
  service:
    build: { type: image, image: "ghcr.io/acme/shop:1.8.2" }
    port: 3000
    replicas: 2
    domains: [shop.example.com]
    env:
      DATABASE_URL: { from: { database: shop-db, field: url } }
---
version: 1
kind: Domain
metadata: { name: www.shop.example.com }
spec: { app: shop-web }
```

### Adopting an existing project

```bash
levelrail-cli export --project shop -o infra/
git add infra && git commit -m "infra: import shop"
levelrail-cli apply -f infra/ --dry-run --exit-code     # exits 0: the files match live state
```

From there, change files and let CI apply them. To let prune clean up later, run one apply with `--source infra-repo` so the apps are tagged as managed by it.

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
