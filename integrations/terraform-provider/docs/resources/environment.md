---
page_title: "levelrail_environment Resource - levelrail"
subcategory: "Projects"
description: |-
  An environment (for example staging) inside a project.
---

# levelrail_environment (Resource)

An environment (for example staging) inside a project.

## Example Usage

```terraform
resource "levelrail_environment" "staging" {
  project_id = levelrail_project.shop.id
  name       = "staging"
  protected  = false
}
```

## Argument Reference

### Required

- `name` (String) Name of the resource.
- `project_id` (String) ID of the owning project.

### Optional

- `protected` (Boolean) Require approval for changes to this environment. Computed when unset.

## Attributes Reference

### Read-Only

- `id` (String) Identifier assigned by the provider.

## Import

Import by `project_id/environment_name`.

```shell
terraform import levelrail_environment.example PROJECT_ID/ENVIRONMENT_NAME
```
