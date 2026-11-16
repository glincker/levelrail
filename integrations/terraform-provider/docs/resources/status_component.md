---
page_title: "levelrail_status_component Resource - levelrail"
subcategory: "Status page"
description: |-
  One row on the public status page, backed by an app, a domain or a check.
---

# levelrail_status_component (Resource)

One row on the public status page, backed by an app, a domain or a check.

## Example Usage

```terraform
resource "levelrail_status_component" "web" {
  kind   = "app"
  target = levelrail_app.web.name
  name   = "Web"
  order  = 1
}
```

## Argument Reference

### Required

- `kind` (String) One of app, domain or check.
- `name` (String) Label shown on the page.
- `target` (String) App name, domain host or check ID, depending on kind.

### Optional

- `order` (Number) Sort position, lowest first. Computed when unset.

## Attributes Reference

### Read-Only

- `id` (String) Identifier assigned by the provider.

## Import

Import by the component ID.

```shell
terraform import levelrail_status_component.example COMPONENT_ID
```
