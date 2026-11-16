---
page_title: "levelrail_domain Resource - levelrail"
subcategory: "Apps"
description: |-
  One hostname routed to an app.
---

# levelrail_domain (Resource)

One hostname routed to an app.

## Example Usage

```terraform
resource "levelrail_domain" "web" {
  app  = levelrail_app.web.name
  host = "shop.example.com"
}
```

## Argument Reference

### Required

- `app` (String) Name of the app.
- `host` (String) Hostname routed to the app.

## Attributes Reference

### Read-Only

- `id` (String) Identifier assigned by the provider.

## Import

Import by `app_name/host`.

```shell
terraform import levelrail_domain.example APP_NAME/HOST
```
