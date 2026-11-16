---
page_title: "levelrail_nodes Data Source - levelrail"
subcategory: "Fleet"
description: |-
  Every node of the fleet. Listing nodes needs a root token.
---

# levelrail_nodes (Data Source)

Every node of the fleet. Listing nodes needs a root token.

## Example Usage

```terraform
data "levelrail_nodes" "all" {}
```

## Argument Reference

This page has no arguments to set.

## Attributes Reference

### Read-Only

- `id` (String) Identifier assigned by the provider.
- `nodes` (List of Object) Every node in the fleet.

### Nested attributes

- `nodes.address` (String, Read-Only) Reported by the control plane.
- `nodes.id` (String, Read-Only) Reported by the control plane.
- `nodes.name` (String, Read-Only) Reported by the control plane.
- `nodes.region` (String, Read-Only) Reported by the control plane.
- `nodes.schedulable` (Boolean, Read-Only) Reported by the control plane.
- `nodes.status` (String, Read-Only) Reported by the control plane.
