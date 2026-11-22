---
page_title: "levelrail_dns_record Resource - levelrail"
subcategory: "DNS"
description: |-
  All values of one DNS record name and type in a zone (a record set).
---

# levelrail_dns_record (Resource)

All values of one DNS record name and type in a zone (a record set).

## Example Usage

```terraform
resource "levelrail_dns_record" "www" {
  zone   = levelrail_dns_zone.main.name
  name   = "www"
  type   = "A"
  ttl    = 300
  values = ["203.0.113.10"]
}
```

## Argument Reference

### Required

- `zone` (String) Zone name the record lives in.
- `name` (String) Record name relative to the zone, or @ for the apex.
- `type` (String) Record type such as A, AAAA, CNAME, TXT or MX.
- `values` (Set of String) Record values in zone file syntax, except TXT which holds the raw text.

### Optional

- `dns_provider` (String) Connected DNS provider holding the zone. Defaults to the only connected provider.
- `ttl` (Number) Time to live in seconds. Defaults to the control plane default.

## Attributes Reference

### Read-Only

- `id` (String) zone/name/type, which is also the import ID.

## Import

Import by `zone/name/type`, with `@` as the name of the apex.

```shell
terraform import levelrail_dns_record.www example.com/www/A
```
