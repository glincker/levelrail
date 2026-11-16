---
page_title: "levelrail_cron_job Resource - levelrail"
subcategory: "Apps"
description: |-
  A scheduled command of one app, the same job a CronJob document describes.
---

# levelrail_cron_job (Resource)

A scheduled command of one app, the same job a CronJob document describes.

## Example Usage

```terraform
resource "levelrail_cron_job" "nightly" {
  app                   = levelrail_app.web.name
  name                  = "nightly"
  schedule              = "0 3 * * *"
  timezone              = "Europe/Berlin"
  command               = ["node", "scripts/nightly.js"]
  concurrency_policy    = "forbid"
  timeout_seconds       = 600
  max_retries           = 2
  retry_backoff_seconds = 30
}
```

## Argument Reference

### Required

- `app` (String) App name that owns the job.
- `name` (String) Job name: lowercase letters, digits, hyphens or underscores.
- `schedule` (String) Cron expression, for example 0 3 * * *.

### Optional

- `catch_up` (String) Missed runs after downtime: skip or run_once. Computed when unset.
- `command` (List of String) Command and arguments to run. Set exactly one of command or http.
- `concurrency_policy` (String) What to do when a run is still going: allow, forbid or replace. Computed when unset.
- `enabled` (Boolean) Whether the job is scheduled. Computed when unset.
- `http` (Object) Call a path on the app instead of running a command.
- `max_retries` (Number) Retries after a failed run. Computed when unset.
- `mode` (String) fresh runs a new container per run, exec runs inside the app container, http is set by the http block. Computed when unset.
- `retry_backoff_seconds` (Number) Seconds between retries. Computed when unset.
- `timeout_seconds` (Number) Seconds before a run is stopped, 0 for the server default. Computed when unset.
- `timezone` (String) IANA timezone of the schedule. The server uses UTC when unset. Computed when unset.

## Attributes Reference

### Read-Only

- `id` (String) Identifier assigned by the provider.

### Nested attributes

- `http.background` (Boolean, Optional) Return at once and let the control plane await the reply. Computed when unset.
- `http.body` (String, Optional) Request body.
- `http.headers` (List of Object, Optional) Request headers. Set a literal value or a secret_ref naming an app env var, never secret text.
- `http.headers.name` (String, Required) Reported by the control plane.
- `http.headers.prefix` (String, Optional) Text before the secret, such as Bearer. Only with secret_ref.
- `http.headers.secret_ref` (String, Optional) Name of the app env var that holds the value.
- `http.headers.value` (String, Optional) Literal header value.
- `http.method` (String, Optional) GET, POST, PUT, PATCH or DELETE. Computed when unset.
- `http.path` (String, Required) Path on the app, starting with a single /.

## Import

Import by `app/job_id`.

```shell
terraform import levelrail_cron_job.example APP_NAME/JOB_ID
```
