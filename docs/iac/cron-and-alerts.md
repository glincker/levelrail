# Cron jobs and alert rules in files

Scheduled commands and alert rules of an app can live in your repository next to the app, as their own documents. Plan shows what would change, apply writes it, export reads the live ones back, and prune removes what you dropped, but only on a stack the files own. Both kinds are beta.

## CronJob

The document name is the job name: lowercase letters, digits, hyphens or underscores. The app must exist live or be declared in the same files.

```yaml
version: 1
kind: CronJob
metadata: { name: nightly }
spec:
  app: web
  schedule: "0 3 * * *"
  timezone: Europe/Berlin      # default UTC
  command: [node, scripts/nightly.js]
  timeout: 10m
  retries: 2
  retryBackoff: 30s
  catchUp: run-once            # skip (default) or run-once
  concurrency: forbid          # allow, forbid (default) or replace
  mode: fresh                  # fresh (default) or exec
  enabled: true
```

The other fields match the `service.jobs` list of an App: `catchUpLookback`, `cpu`, `memory` and `warnAfter`.

### HTTP jobs

Instead of a command, a job can call a path on the app itself. Set `http` and leave out `command`, `mode`, `cpu` and `memory`.

```yaml
version: 1
kind: CronJob
metadata: { name: warm-cache }
spec:
  app: web
  schedule: "*/10 * * * *"
  timeout: 30s
  http:
    path: /internal/warm
    method: POST               # GET (default), POST, PUT, PATCH or DELETE
    body: '{"full": false}'
    background: false          # true returns at once and awaits the reply server side
    headers:
      - { name: X-Job, value: warm }
      - { name: Authorization, secretRef: CRON_TOKEN, prefix: "Bearer " }
```

The request goes to the app, never to an arbitrary URL, so the path must start with a single `/`. A header holds either a literal `value` or a `secretRef` naming an env var of the app, and the secret is read at run time, so no secret text lives in the file. `timeout` bounds the call.

Each job has one owner. Declaring the same job name in an App's `service.jobs` and as a CronJob fails the plan with `ownership_conflict`, and so does a job that the app's own `app.yaml` declares. Jobs created in the dashboard or CLI that no file lists are kept, and removed only by prune.

A command can hold a credential. Plan output hides a changed command value that looks like one, and a secret should be read from the app's env at run time, not written into the file.

## AlertRule

```yaml
version: 1
kind: AlertRule
metadata: { name: high-cpu }
spec:
  app: web
  type: threshold
  metric: cpu_percent
  comparator: ">"
  threshold: 90
  for: 5m
  channel: ops
```

A `threshold` rule needs `metric`, a `comparator` (`>`, `<`, `>=` or `<=`) and a `threshold`. Other types, such as `crashloop` or `scheduled_task_failure`, take only the fields that apply to them. The channel is a notification channel by name and must exist.

## Invalid files

A bad document fails before anything is planned, with the file, line and a stable code.

| Code | Meaning |
| --- | --- |
| `cron_schedule_invalid` | The schedule is not a valid cron expression. |
| `cron_job_invalid` | The job name, command, timezone, retries or limits break a rule. |
| `cron_http_invalid` | The http path, method or a header breaks a rule, or `http` is set with a `command`. |
| `alert_rule_invalid` | A threshold rule lacks its metric, comparator or threshold. |
| `ownership_conflict` | The job is declared in two places. |

Unknown fields fail with `schema_unknown_field`. See [Errors](errors.md) for every code.

## Export

Export writes a CronJob document for each job that is not declared by the app's own `app.yaml`, and an AlertRule document for each rule, so a repository can adopt what you built in the dashboard.
