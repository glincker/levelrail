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
| `levelrail_app` | Image, port, replicas, strategy, bind address, plain env vars, project | app name |
| `levelrail_domain` | One hostname routed to an app | `app_name/host` |
| `levelrail_database` | A managed database and its project | database name |
| `levelrail_environment_variables` | Variables shared by a project or an environment | `project/<id>` or `environment/<id>` |
| `levelrail_status_page` | The public status page settings: `enabled`, `title`, `description`, `custom_domain` (one per control plane) | any value, for example `status-page` |
| `levelrail_status_component` | One status page row: `kind` (`app`, `domain` or `check`), `target`, `name`, `order` | component ID |

Data sources: `levelrail_apps` lists every app with its image, port, project and domains. `levelrail_nodes` lists the fleet and needs a root token.

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

## What is not covered

- App secrets and vault-backed variables (`secret_env`, `vault_env`): values would land in Terraform state, so use the secrets API or the CLI.
- Volumes, bind mounts, resource limits, health checks, hooks, labels, commands and egress policy on apps.
- Git and build sources, deploys and rollbacks (these are actions, not state).
- Per-domain settings such as TLS certificates, redirects, headers, WAF, caching and auth gates.
- Nodes (read only through the data source), load balancers, alerts, pipelines, notification channels, backups, teams, users, roles and tokens.
- Database users, backup schedules, public access and upgrades.

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
