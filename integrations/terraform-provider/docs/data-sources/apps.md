---
page_title: "levelrail_apps Data Source - levelrail"
subcategory: "Apps"
description: |-
  Every app the token can see.
---

# levelrail_apps (Data Source)

Every app the token can see.

## Example Usage

```terraform
data "levelrail_apps" "all" {}
```

## Argument Reference

This page has no arguments to set.

## Attributes Reference

### Read-Only

- `apps` (List of Object) Every app on the control plane.
- `id` (String) Identifier assigned by the provider.

### Nested attributes

- `apps.domains` (List of String, Read-Only) Reported by the control plane.
- `apps.image` (String, Read-Only) Reported by the control plane.
- `apps.name` (String, Read-Only) Reported by the control plane.
- `apps.port` (Number, Read-Only) Reported by the control plane.
- `apps.project_id` (String, Read-Only) Reported by the control plane.
