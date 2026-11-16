---
page_title: "levelrail_node_labels Resource - levelrail"
subcategory: "Fleet"
description: |-
  Labels and taints on an existing node. The node is never created or deleted here, and labels or taints set by other means are left alone. Destroying removes only what this resource set. Needs a root token.
---

# levelrail_node_labels (Resource)

Labels and taints on an existing node. The node is never created or deleted here, and labels or taints set by other means are left alone. Destroying removes only what this resource set. Needs a root token.

## Example Usage

```terraform
# Labels and taints on a node that already exists. Other labels stay untouched.
resource "levelrail_node_labels" "edge" {
  node = "edge-1"

  labels = {
    zone = "eu-west"
    tier = "edge"
  }

  taints = [
    { key = "dedicated", value = "gpu", effect = "NoSchedule" },
  ]
}
```

## Argument Reference

### Required

- `node` (String) Name (or ID) of an existing node. Changing it replaces the resource.

### Optional

- `labels` (Map of String) Labels this resource owns on the node.
- `taints` (List of Object) Taints this resource owns on the node, identified by key and effect.

## Attributes Reference

### Read-Only

- `id` (String) The node ID.

### Nested attributes

- `taints.effect` (String, Required) NoSchedule, PreferNoSchedule or NoExecute.
- `taints.key` (String, Required) Taint key.
- `taints.value` (String, Optional) Taint value, empty when unset.

## Import

Import by the node name or ID. Every live label and taint is adopted as owned.

```shell
terraform import levelrail_node_labels.example NODE_NAME
```
