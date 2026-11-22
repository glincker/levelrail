---
page_title: "levelrail_latest_deploy Data Source - levelrail"
subcategory: "Apps"
description: |-
  The newest deploy of one app, optionally the newest one with a given status.
---

# levelrail_latest_deploy (Data Source)

The newest deploy of one app, optionally the newest one with a given status.

## Example Usage

```terraform
data "levelrail_latest_deploy" "web" {
  app    = "shop-web"
  status = "succeeded"
}
```

## Argument Reference

### Required

- `app` (String) Name of the app.

### Optional

- `status` (String) Keep only deploys with this status, for example succeeded. The status of the deploy found is returned here.

## Attributes Reference

### Read-Only

- `id` (String) Identifier assigned by the provider.
- `deploy_id` (String) Id of the deploy, empty when the app has none.
- `image` (String) Image the deploy ran.
- `commit_sha` (String) Git commit of a source deploy, empty otherwise.
- `started_at` (String) When the deploy started, RFC 3339.
- `finished_at` (String) When the deploy finished, empty while it runs.
- `found` (Boolean) False when the app has no deploy matching the filter.
