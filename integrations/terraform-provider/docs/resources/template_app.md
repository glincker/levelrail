---
page_title: "levelrail_template_app Resource - levelrail"
subcategory: "Apps"
description: |-
  A catalog template installed by id, the same install the dashboard form and a TemplateApp document run.
---

# levelrail_template_app (Resource)

A catalog template installed by id, the same install the dashboard form and a TemplateApp document run. `env` and `domains` apply at install time only, and any change replaces the app.

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

## Changes and destroy

`env` and `domains` apply at install time only. Any change destroys the app and installs it again onto the surviving named volumes: a delete keeps volumes unless asked otherwise, and regenerated passwords can break stateful apps such as Postgres. A protected app refuses the delete. Recommended:

```terraform
lifecycle {
  prevent_destroy = true
  ignore_changes  = [env, domains]
}
```

Terraform state stores `env` in plaintext. Sensitive only masks plan output, so protect the state.

## Import

Import by app name. The template is read back from the install record. After an import, set `env` and `domains` as installed, or add `ignore_changes`; they cannot be read back.

```shell
terraform import levelrail_template_app.example APP_NAME
```
