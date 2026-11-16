---
page_title: "levelrail_app_secrets Data Source - levelrail"
subcategory: "Apps"
description: |-
  Secret key metadata of one app (names, lock and age). Values are never exposed.
---

# levelrail_app_secrets (Data Source)

Secret key metadata of one app (names, lock and age). Values are never exposed.

## Example Usage

```terraform
data "levelrail_app_secrets" "web" {
  app = levelrail_app.web.name
}
```

## Argument Reference

### Required

- `app` (String) Name of the app.

## Attributes Reference

### Read-Only

- `id` (String) Identifier assigned by the provider.
- `secrets` (List of Object) Secret key names with lock state and age, never values.

### Nested attributes

- `secrets.build_time` (Boolean, Read-Only) Reported by the control plane.
- `secrets.key` (String, Read-Only) Reported by the control plane.
- `secrets.locked` (Boolean, Read-Only) Reported by the control plane.
- `secrets.stale` (Boolean, Read-Only) Reported by the control plane.
- `secrets.updated_at` (String, Read-Only) Reported by the control plane.
