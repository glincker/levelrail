---
page_title: "levelrail_template_app Resource - levelrail"
subcategory: "Apps"
description: |-
  A catalog template installed by id, the same install the dashboard form and a TemplateApp document run.
---

# levelrail_template_app (Resource)

A catalog template installed by id, the same install the dashboard form and a TemplateApp document run. Every input is fixed at install time, so changing one replaces the app.

## Example Usage

```terraform
resource "levelrail_template_app" "archive" {
  template = "archivebox"
  name     = "archive"
  env      = { ADMIN_USERNAME = "admin" }
  domains  = { archivebox = "archive.example.com" }
}
```

## Argument Reference

### Required

- `template` (String) Catalog template id.

### Optional

- `name` (String) App name. Defaults to the template id.
- `env` (Map of String, Sensitive) Values for the template install form. Keys must be settings the template declares.
- `domains` (Map of String) Host per service the template offers a domain for.

## Attributes Reference

### Read-Only

- `id` (String) Identifier assigned by the provider, the app name.

## Import

Import by app name.

```shell
terraform import levelrail_template_app.example APP_NAME
```
