---
page_title: "levelrail_status_page Resource - levelrail"
subcategory: "Status page"
description: |-
  The public status page settings. There is one page per control plane.
---

# levelrail_status_page (Resource)

The public status page settings. There is one page per control plane.

## Example Usage

```terraform
resource "levelrail_status_page" "main" {
  enabled       = true
  title         = "Acme status"
  description   = "Live health of Acme services."
  custom_domain = "status.example.com"
}
```

## Argument Reference

### Optional

- `custom_domain` (String) Plain hostname that serves the page, such as status.example.com. Computed when unset.
- `description` (String) Short text under the title. Computed when unset.
- `enabled` (Boolean) Serve the public page. Computed when unset.
- `title` (String) Page title. Computed when unset.

## Attributes Reference

### Read-Only

- `id` (String) Identifier assigned by the provider.

## Import

There is only one status page, so any value works as the ID.

```shell
terraform import levelrail_status_page.example status-page
```
