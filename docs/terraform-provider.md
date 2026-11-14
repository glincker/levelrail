---
title: "Terraform and OpenTofu provider"
description: "Manage projects, environments, apps, domains, databases and shared env variables from Terraform or OpenTofu, with import support and what is not covered."
"og:url": "https://docs.levelrail.com/terraform-provider"
---

The provider drives the same REST API as the dashboard and CLI, with your own token, so every policy and approval rule still applies. It lives in its own Go module, `integrations/terraform-provider/`, and never ships inside the control plane binary.

<Warning>
**Not published yet.** The provider is built from source for now. Publishing to the Terraform and OpenTofu registries is a later step, so there is no `terraform init` download yet.
</Warning>

## Where the name lives

Everything that carries the product name is in one file, `integrations/terraform-provider/internal/provider/brand.go`: the resource prefix, the registry address, and the environment variable names. A rename is that one edit plus the module path in `integrations/terraform-provider/go.mod`. Resource type names are built from the prefix at runtime, so no other Go file repeats it.

## Configure

```hcl
terraform {
  required_providers {
    levelrail = {
      source = "registry.terraform.io/glincker/levelrail"
    }
  }
}

provider "levelrail" {
  url   = "https://deploy.example.com"
  token = var.api_token
}
```

`url` and `token` fall back to the `APP_API_URL` and `APP_API_TOKEN` environment variables, the same ones the CLI reads. Prefer the variables over writing the token into a file.

## Resources

| Resource | Manages | Import ID |
| --- | --- | --- |
| `levelrail_project` | A project | project ID |
| `levelrail_environment` | An environment of a project, including `protected` | `project_id/environment_name` |
| `levelrail_app` | Image, port, replicas, strategy, bind address, plain env vars, project, command, labels, memory and CPU limits, restart policy, readiness and liveness paths, published ports | app name |
| `levelrail_domain` | One hostname routed to an app | `app_name/host` |
| `levelrail_database` | A managed database, its project and its connection pooler (`pooler_enabled`, `pooler_pool_size`, `pooler_max_client_conn`) | database name |
| `levelrail_environment_variables` | Variables shared by a project or an environment | `project/<id>` or `environment/<id>` |
| `levelrail_status_page` | The public status page settings: `enabled`, `title`, `description`, `custom_domain` (one per control plane) | any value, for example `status-page` |
| `levelrail_app_secret` | One secret value for an app (write only: the API never returns values, so only a missing key shows as drift; the value is kept in state) | `app/key` |
| `levelrail_alert_rule` | An alert rule on one app (threshold or crash loop) | `app/rule_id` |
| `levelrail_status_component` | One status page row: `kind` (`app`, `domain` or `check`), `target`, `name`, `order` | component ID |
| `levelrail_setting` | One runtime setting (`key`, `value`); destroy resets it to the default | setting key |
| `levelrail_node_labels` | Labels and taints on an existing node, by name | node name or ID |

Data sources: `levelrail_apps` lists every app with its image, port, project and domains. `levelrail_nodes` lists the fleet and needs a root token. `levelrail_app_secrets` lists the secret key names, lock and age of one app, never values.

## Coverage

Gap analysis of what each resource exposes against what the REST API and the IaC kinds support. "Not covered" means the API has it but it is not stable in the IaC schema yet, or it has no stable route.

| Resource or data source | Exposed | Not covered |
| --- | --- | --- |
| `levelrail_project` | name | shared env (use `levelrail_environment_variables`) |
| `levelrail_environment` | project, name, protected | environment env (use `levelrail_environment_variables`) |
| `levelrail_app` | image, port, replicas, strategy, bind address, env, project, command, labels, memory and CPU limits, restart policy, readiness and liveness paths, published ports | volumes, bind mounts, hooks, exec and TCP probes and probe timings, egress, GPU, tolerations, affinity, tags, database env references, files |
| `levelrail_domain` | app, host | TLS, redirects, headers, WAF, caching, auth gates (not in the IaC Domain schema yet) |
| `levelrail_database` | engine, version, project, pooler settings | backup schedules, placement, public access (not in the IaC Database schema yet), users |
| `levelrail_environment_variables` | keys at a project or environment scope | vault-backed variables |
| `levelrail_app_secret` | one secret value | bulk import |
| `levelrail_alert_rule` | threshold and crash loop rules on an app | notification channel management |
| `levelrail_status_page` | enabled, title, description, custom domain | incidents |
| `levelrail_status_component` | kind, target, name, order | none |
| `levelrail_setting` | key and value of any setting that is not marked sensitive | sensitive settings (credentials), which are refused so they never reach state |
| `levelrail_node_labels` | labels and taints of an existing node | creating or deleting nodes, drains, region, workloads |
| Data `levelrail_apps`, `levelrail_nodes`, `levelrail_app_secrets` | listings | filters |

Every resource supports `terraform import`, treats a 404 on read as drift (removed from state), and reads the live object after each write. Lists and maps on `levelrail_app` (command, labels, published ports, env) are managed only when set, so leaving one unset never wipes what the dashboard configured; a resource that is imported shows them as unset until you add them to the configuration.

## Complete example

```hcl
resource "levelrail_project" "shop" {
  name = "shop"
}

resource "levelrail_environment" "staging" {
  project_id = levelrail_project.shop.id
  name       = "staging"
  protected  = false
}

resource "levelrail_environment_variables" "staging" {
  scope    = "environment"
  scope_id = levelrail_environment.staging.id
  variables = {
    LOG_LEVEL = "debug"
  }
}

resource "levelrail_database" "main" {
  name       = "shop-db"
  engine     = "postgres"
  version    = "16"
  project_id = levelrail_project.shop.id
}

resource "levelrail_app" "web" {
  name       = "shop-web"
  image      = "ghcr.io/example/shop:1.4.2"
  port       = 3000
  replicas   = 2
  project_id = levelrail_project.shop.id
  env = {
    NODE_ENV = "production"
  }
}

resource "levelrail_domain" "web" {
  app  = levelrail_app.web.name
  host = "shop.example.com"
}

data "levelrail_nodes" "all" {}

output "node_count" {
  value = length(data.levelrail_nodes.all.nodes)
}
```

## Behavior worth knowing

- `levelrail_app` and `levelrail_domain` both write the app's domain list. Manage hostnames only with `levelrail_domain`, never through the app resource, which does not expose them.
- `levelrail_app.env` is sensitive and, when set, replaces the live plain variables. Leave it unset to manage variables elsewhere.
- `levelrail_environment_variables` only manages the keys you list. Other variables at that scope are left alone, and deleting the resource removes only its own keys.
- Database engine and version never change in place. A change forces a replace, which destroys data, so use `prevent_destroy` on databases you care about.
- Deleting a project follows the API's own guard and is refused while it still holds apps.

- `levelrail_status_page` is a singleton. Destroying it turns the page off and clears its fields. See `examples/terraform-provider/status-page.tf`.
- `levelrail_status_component` is matched by ID, so a component deleted outside Terraform is planned for re-creation. Status incidents are not managed.

## Settings, node labels and tokens

- `levelrail_setting` writes through the same validation as the CLI. A change that weakens a security protection is refused until you confirm it in the CLI or dashboard, and settings marked sensitive are refused outright, so a credential never lands in state. Destroying the resource resets the key to its default. Writing settings needs a root token.
- `levelrail_node_labels` never creates or deletes a node. It owns only the label keys and the taints (key plus effect) you list: others set from the CLI are left alone and never show as drift, and destroy removes only what the resource set. Importing by node name adopts every label and taint the node has.
- There is no token resource. The API only lets an interactive session create or revoke tokens, never a bearer token, so a provider holding a token cannot mint another one. Create tokens in the dashboard or CLI and pass them in as variables.

## Retries

Requests are retried with exponential backoff and jitter on 429, 5xx and connection resets, honoring `Retry-After`. Only safe requests are retried: GET, HEAD, PUT and DELETE, and POST only when it carries an `Idempotency-Key`. A 4xx other than 429 is never retried. Tune it with `max_retries` (default 4, `0` turns it off) and `retry_wait_max_seconds` (default 30, which also caps a long `Retry-After`) in the provider block.

## What is not covered

- Vault-backed variables (`vault_env`) and bulk secret import. Single secret values can be set with `levelrail_app_secret`, but they land in Terraform state, so protect the state.
- Volumes, bind mounts, hooks, exec and TCP probes, egress policy, GPU requests and placement on apps.
- Git and build sources, deploys and rollbacks (these are actions, not state).
- Per-domain settings such as TLS certificates, redirects, headers, WAF, caching and auth gates.
- Creating or removing nodes (the `levelrail_nodes` data source reads them), load balancers, pipelines, notification channels, backups, teams, users, roles and tokens.
- Database users, backup schedules, placement, public access and upgrades.

For several of these, [Platform as code](platform-as-code.md) covers them through the CLI.

## Acceptance tests

They need a `terraform` or `tofu` binary and a live control plane, and are off unless `TF_ACC=1`. The recipe in `scripts/smoke.sh` starts a dev mode control plane with no Docker (`SMOKE_KEEP=1` leaves it running and prints the URL):

```bash
SMOKE_KEEP=1 scripts/smoke.sh -- nodes list
cd integrations/terraform-provider
TF_ACC=1 APP_API_URL=http://127.0.0.1:<port> APP_API_TOKEN=dev-root-token \
  go test ./internal/provider -count=1
```

The database resource has no acceptance test because creating a database needs a Docker host.
