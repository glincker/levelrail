---
page_title: "levelrail_projects Data Source - levelrail"
subcategory: "Projects"
description: |-
  Every project the token can see.
---

# levelrail_projects (Data Source)

Every project the token can see.

## Example Usage

```terraform
data "levelrail_projects" "all" {}
```

## Argument Reference

This page has no arguments to set.

## Attributes Reference

### Read-Only

- `id` (String) Identifier assigned by the provider.
- `projects` (List of Object) Every project on the control plane.

### Nested attributes

- `projects.id` (String, Read-Only) Project id.
- `projects.name` (String, Read-Only) Project name.
