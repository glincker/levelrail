---
page_title: "levelrail_setting Resource - levelrail"
subcategory: "Fleet"
description: |-
  One runtime setting. Destroying it resets the setting to its default. Settings marked sensitive are refused: manage them from the CLI so a credential never lands in state.
---

# levelrail_setting (Resource)

One runtime setting. Destroying it resets the setting to its default. Settings marked sensitive are refused: manage them from the CLI so a credential never lands in state.

## Example Usage

```terraform
# Keep the last 10 inline sources; destroying this resets the setting to its default.
resource "levelrail_setting" "inline_keep" {
  key   = "deploy.inline_source_keep"
  value = "10"
}
```

## Argument Reference

### Required

- `key` (String) Setting key, for example `deploy.inline_source_keep`. Only keys the control plane lists are accepted. Changing it replaces the resource.
- `value` (String) Value as text: `true`, `42`, `15m`, plain text, or a JSON array for a list setting.

## Attributes Reference

### Read-Only

- `id` (String) The setting key.

## Import

Import by the setting key.

```shell
terraform import levelrail_setting.example SETTING_KEY
```
