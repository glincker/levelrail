---
page_title: "levelrail_dns_zone Resource - levelrail"
subcategory: "DNS"
description: |-
  A DNS zone at a connected DNS provider. A zone is never adopted by create: one that already exists is refused.
---

# levelrail_dns_zone (Resource)

A DNS zone at a connected DNS provider. A zone is never adopted by create: one that already exists is refused.

## Example Usage

```terraform
resource "levelrail_dns_zone" "main" {
  name = "example.com"
}
```

## Argument Reference

### Required

- `name` (String) Zone name such as example.com.

### Optional

- `dns_provider` (String) Connected DNS provider to create the zone at. Defaults to the only connected provider.
- `force_destroy` (Boolean) Delete the zone even when it still holds records beyond NS and SOA. Defaults to false.

## Attributes Reference

### Read-Only

- `id` (String) Zone name, which is also the import ID.
- `status` (String) Status reported by the provider.
- `name_servers` (List of String) Name servers to delegate the domain to.

## Import

Import by the zone name.

```shell
terraform import levelrail_dns_zone.main example.com
```
