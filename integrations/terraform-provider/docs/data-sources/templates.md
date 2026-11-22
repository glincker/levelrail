---
page_title: "levelrail_templates Data Source - levelrail"
subcategory: "Apps"
description: |-
  The service template catalog, optionally narrowed to one category.
---

# levelrail_templates (Data Source)

The service template catalog, optionally narrowed to one category.

## Example Usage

```terraform
data "levelrail_templates" "databases" {
  category = "database"
}
```

## Argument Reference

### Optional

- `category` (String) Keep only templates of this category.

## Attributes Reference

### Read-Only

- `id` (String) Identifier assigned by the provider.
- `templates` (List of Object) The templates of the catalog.

### Nested attributes

- `templates.id` (String, Read-Only) Template id.
- `templates.name` (String, Read-Only) Display name.
- `templates.slogan` (String, Read-Only) One line description.
- `templates.category` (String, Read-Only) Catalog category.
- `templates.installable` (Boolean, Read-Only) True when the template can be installed with one command.
- `templates.requires_configuration` (Boolean, Read-Only) True when values must be supplied before install.
- `templates.verified` (Boolean, Read-Only) True when the template passed the last live smoke run.
