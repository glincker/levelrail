---
page_title: "levelrail_app_secret Resource - levelrail"
subcategory: "Apps"
description: |-
  A secret value stored for one app. The API never returns values, so drift in the value itself cannot be detected; only a missing key is.
---

# levelrail_app_secret (Resource)

A secret value stored for one app. The API never returns values, so drift in the value itself cannot be detected; only a missing key is.

## Example Usage

```terraform
resource "levelrail_app_secret" "session" {
  app   = levelrail_app.web.name
  key   = "SESSION_KEY"
  value = var.session_key
}
```

## Argument Reference

### Required

- `app` (String) Name of the app.
- `key` (String) Secret key name.
- `value` (String) Secret value. Stored in state, so protect the state. Sensitive.

## Attributes Reference

### Read-Only

- `id` (String) Identifier assigned by the provider.

## Import

Import by `app/key`. The API never reveals the value, so set `value` in the configuration.

```shell
terraform import levelrail_app_secret.example APP_NAME/KEY
```
