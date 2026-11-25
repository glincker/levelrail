---
page_title: "levelrail_database Resource - levelrail"
subcategory: "Databases"
description: |-
  A managed database. Engine and version cannot change in place because the database holds data.
---

# levelrail_database (Resource)

A managed database. Engine and version cannot change in place because the database holds data.

## Example Usage

```terraform
resource "levelrail_database" "main" {
  name       = "shop-db"
  engine     = "postgres"
  version    = "16"
  project_id = levelrail_project.shop.id

  pooler_enabled         = true
  pooler_pool_size       = 20
  pooler_max_client_conn = 200

  lifecycle {
    prevent_destroy = true
  }
}
```

## Argument Reference

### Required

- `engine` (String) For example postgres or redis.
- `name` (String) Database name. Changing it replaces the database.

### Optional

- `pooler_enabled` (Boolean) Route linked apps through a connection pooler. Toggled in place. Computed when unset.
- `pooler_max_client_conn` (Number) Client connections the pooler accepts. Used when the pooler is enabled. Computed when unset.
- `pooler_pool_size` (Number) Server connections per pool. Used when the pooler is enabled. Computed when unset.
- `project_id` (String) Project the database belongs to. Computed when unset.
- `version` (String) Engine version. Omit or set "latest" to let the control plane pick the newest stable supported version; the concrete version is in resolved_version. Computed when unset.

## Attributes Reference

- `resolved_version` (String) The concrete engine version the database runs.

## Import

Import by the database name.

```shell
terraform import levelrail_database.example DATABASE_NAME
```
