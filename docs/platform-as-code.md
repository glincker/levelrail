---
description: Describe projects, environments, apps, domains, databases and more as YAML files, review the plan, and apply it from the CLI, the dashboard, MCP or CI.
---

# Platform as code

Write the state you want as YAML, run `levelrail-cli apply`, and the control plane converges to it. The same files can be exported from a live control plane, reviewed in a pull request, and applied from CI.

Everything goes through the ordinary REST API with your own token. If a policy denies you an app, that one item is reported as denied and nothing else is touched by it. There is no separate write path.

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

An App's `spec.service` is the app.yaml service definition, validated by the same schema. The document schema embeds the app.yaml definitions unchanged, with one addition: an env value may be `{ secretRef: NAME }`.

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
```

Apply manages apps that run a prebuilt image (`build.type: image`). Git builds are configured with a git source, not here. Not supported yet, and rejected with a clear error: volumes, egress policies, GPU requests, registry credentials, `from` and `vault` env references. A live app that uses them keeps them when you apply; the plan warns.

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

- Reference a secret by name: `API_KEY: { secretRef: API_KEY }` or `API_KEY: ${{ secrets.API_KEY }}`. Secrets are stored per app under the env var name, so the reference must match it.
- A literal value on a key that looks like a secret (`PASSWORD`, `TOKEN`, `API_KEY`, and similar) or a value shaped like a known token is refused with an error. The error names the key and line and never echoes the value.
- To create or update the value at apply time, pass it explicitly from the environment or a file:

```bash
levelrail-cli apply -f infra/ \
  --secret API_KEY=env:PROD_API_KEY \
  --secret web/DB_PASSWORD=file:./db-password.txt
```

Use `NAME` to apply to every app referencing that name, or `app/NAME` for one app. The plan only ever shows that a secret is set, never the value. A secret that is referenced but has no value yet is reported as a warning.

- `${{ env.NAME }}` placeholders in plain values are filled from your environment when you run the CLI (or with `--var NAME=value`). Export uses them when you pass `--include-env-values=false`.

Export never writes a secret value. Secret backed variables are written as `secretRef`, and a plain variable whose name or value looks like a secret is written as a `${{ env.NAME }}` placeholder with a warning.

## Export

```bash
levelrail-cli export                       # everything you can read, to stdout
levelrail-cli export --project shop -o infra/
levelrail-cli export --app web -o -
levelrail-cli export --project shop --include-env-values=false -o infra/
```

Output is stable: documents are ordered by kind then name, keys are sorted, and there are no timestamps or generated ids. Export, apply, export gives identical files, and applying an export to the control plane it came from changes nothing. Live settings the format cannot express (volumes, egress policies, vault env) are listed as warnings on stderr.

## Apply

```text
levelrail-cli apply -f file|dir|- [--dry-run] [--prune] [--yes] [--project P]
                    [--source NAME] [--secret ...] [--var ...]
                    [--no-deploy] [--continue-on-error] [--exit-code]
levelrail-cli diff  -f dir        # the same as apply --dry-run --exit-code
```

`-f` may be repeated and accepts a file, a directory (searched recursively for `.yaml` and `.yml`) or `-` for stdin.

### How it runs

1. **Validate everything first.** Every document is checked against the schema and the semantic rules. If any document has an issue, nothing is planned and nothing is applied.
2. **Plan.** Live state is read through the API with your permissions, and a plan is computed: create, update (with a field level diff), delete (only with `--prune`) or no change. Env values are never shown, only `(hidden)`. Items are ordered by dependency: project, environment, tag, database, app, domain, load balancer, pipeline, alert rule.
3. **Apply.** Each item is executed through the existing endpoint. Apply is safe to repeat and to resume: a re-run recomputes the plan and only does what is still missing.

Semantics worth knowing:

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
        run: levelrail-cli apply -f infra/ --source infra-repo --dry-run --exit-code || [ $? -eq 2 ]

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
        run: levelrail-cli apply -f infra/ --source infra-repo --yes --secret API_KEY=env:PROD_API_KEY
```

Planning needs only a read token. Use a separate token with write ability for the apply job. The install step is yours to supply: download the `levelrail-cli` release binary for the runner, the way the bundled deploy action does. See [GitHub Actions](github-actions.md) for token setup and that action.

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
      DATABASE_PASSWORD: { secretRef: DATABASE_PASSWORD }
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
      DATABASE_PASSWORD: { secretRef: DATABASE_PASSWORD }
---
version: 1
kind: Domain
metadata: { name: www.shop.example.com }
spec: { app: shop-web }
```

### Migrating an existing project

```bash
levelrail-cli export --project shop -o infra/
git add infra && git commit -m "infra: import shop"
levelrail-cli apply -f infra/ --dry-run --exit-code     # exits 0: the files match live state
```

From there, change files and let CI apply them. To let prune clean up later, run one apply with `--source infra-repo` so the apps are tagged as managed by it.

## Not in scope

Git sync is a recipe, not an engine. Backup schedules, volumes, egress policies, node placement, registry credentials and vault or database env references are not expressible yet and are left as they are.

## See also

- [App spec reference](app-spec-reference.md) for the service fields
- [Pipelines](pipelines.md) for the pipeline definition format
- [Identity and access](identity-and-access.md) for tokens and per resource policies
- [Migrating from Coolify, Dokploy and CapRover](migrating-from-coolify-dokploy-and-caprover.md)
