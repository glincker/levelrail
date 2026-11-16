---
page_title: "levelrail_environment_variables Resource - levelrail"
subcategory: "Projects"
description: |-
  Variables shared by every app of a project or an environment. Only the keys listed here are managed.
---

# levelrail_environment_variables (Resource)

Variables shared by every app of a project or an environment. Only the keys listed here are managed.

## Example Usage

```terraform
resource "levelrail_environment_variables" "shop" {
  scope    = "project"
  scope_id = levelrail_project.shop.id

  variables = {
    LOG_LEVEL = "info"
  }
}
```

## Argument Reference

### Required

- `scope` (String) project or environment.
- `scope_id` (String) ID of the project or environment.
- `variables` (Map of String) Map of variable names to values managed by this resource. Sensitive.

## Attributes Reference

### Read-Only

- `id` (String) Identifier assigned by the provider.

## Import

Import by `scope/scope_id`, where scope is `project` or `environment`.

```shell
terraform import levelrail_environment_variables.example project/PROJECT_ID
```
