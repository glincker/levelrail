---
page_title: "levelrail_cron_jobs Data Source - levelrail"
subcategory: "Apps"
description: |-
  Every scheduled job of one app, whatever created it. Header values and secret references are not listed.
---

# levelrail_cron_jobs (Data Source)

Every scheduled job of one app, whatever created it. Header values and secret references are not listed.

## Example Usage

```terraform
data "levelrail_cron_jobs" "web" {
  app = levelrail_app.web.name
}
```

## Argument Reference

### Required

- `app` (String) Name of the app.

## Attributes Reference

### Read-Only

- `id` (String) Identifier assigned by the provider.
- `jobs` (List of Object) The jobs of the app.

### Nested attributes

- `jobs.command` (List of String, Read-Only) Reported by the control plane.
- `jobs.enabled` (Boolean, Read-Only) Reported by the control plane.
- `jobs.http_method` (String, Read-Only) Method of an http job, empty for a command job.
- `jobs.http_path` (String, Read-Only) Path called by an http job, empty for a command job.
- `jobs.id` (String, Read-Only) Reported by the control plane.
- `jobs.mode` (String, Read-Only) Reported by the control plane.
- `jobs.name` (String, Read-Only) Reported by the control plane.
- `jobs.schedule` (String, Read-Only) Reported by the control plane.
- `jobs.timezone` (String, Read-Only) Reported by the control plane.
