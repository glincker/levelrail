---
page_title: "levelrail_project Resource - levelrail"
subcategory: "Projects"
description: |-
  A project groups apps, databases and environments.
---

# levelrail_project (Resource)

A project groups apps, databases and environments.

## Example Usage

```terraform
resource "levelrail_project" "shop" {
  name = "shop"
}
```

## Argument Reference

### Required

- `name` (String) Project name. Changing it replaces the project.

## Attributes Reference

### Read-Only

- `id` (String) Identifier assigned by the provider.

## Import

Import by the project ID.

```shell
terraform import levelrail_project.example PROJECT_ID
```
