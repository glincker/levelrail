---
title: "Cleanup center"
tag: "Beta"
description: Keep control plane data bounded with per-data-type retention, a scheduled cleanup with preview mode, floors for security data, and an exportable run history.
"og:url": "https://docs.levelrail.com/cleanup-center"
---

The cleanup center is one place to see what the control plane stores, how long each kind of data is kept, and what a cleanup would remove. Every data type is a target with a retention policy, a mode, a usage figure and a run history. Open it from Settings, Cleanup.

Docker objects (stopped containers, images, build cache, orphaned volumes, old deploy attempts) are shown there too, but they are still managed by the [automatic cleanup](/janitor) policy.

## How it works

- **Retention** is a runtime setting per target, with presets of 30, 60, 90, 180 and 365 days, or 0 to keep forever. Change it in the cleanup page or with `PUT /api/v1/settings/runtime/{policy_key}`. A saved value beats the environment variable, which beats the default.
- **Mode** is `off`, `preview` or `on`, stored as the runtime setting `cleanup.<id>.mode`. `preview` runs the scheduled cleanup as a dry run: it records how many rows it would delete and deletes nothing.
- **Origin** decides the default mode. A target that replaces an older sweeper (`existing`) defaults to `on`, so nothing changes for current installs. A target for data that was never bounded (`new`) defaults to `preview` until an administrator enables it.
- **Schedule.** One scheduler checks every target each `cleanup.interval` (default 6 hours, minimum 15 minutes), spaces the targets a few seconds apart, and waits 5 minutes after boot before the first pass. `cleanup.enabled` turns the schedule off; manual cleanups still work.
- **Safety.** Deletes run in time slices (one day of rows at a time), a run stops at a row cap and reports `more_remaining`, a target never runs twice at once, a run never deletes rows younger than its window, and a cancelled run stops between slices. A dry run never writes. Reclaiming disk space (VACUUM) is a separate action, never a side effect.

## Targets

| Target id | Data | Policy setting | Default | Env override | Floor |
| --- | --- | --- | --- | --- | --- |
| `audit.log` | Audit log | `retention.audit_log_days` | 90 days | `APP_AUDIT_LOG_RETENTION_DAYS` | 90 days |
| (same target) | Sign-in, token and policy audit entries | `retention.audit_security_days` | 365 days | `APP_AUDIT_LOG_SECURITY_RETENTION_DAYS` | 365 days |
| `messaging.email_log` | Email delivery log | `retention.email_log_days` | 30 days | `APP_EMAIL_LOG_RETENTION_DAYS` | none |
| `messaging.webhook_deliveries` | Inbound git webhook deliveries | `retention.webhook_delivery_days` | 30 days | `APP_WEBHOOK_DELIVERY_RETENTION_DAYS` | none |
| `messaging.event_webhook_deliveries` | Finished outbound event webhook deliveries | `retention.event_webhook_days` | 30 days | `APP_EVENT_WEBHOOK_RETENTION` | none |
| `observability.alert_history` | Alert history | `retention.alert_history_days` | 30 days | `APP_ALERT_HISTORY_RETENTION` | none |
| `auth.api_tokens` | Revoked and expired API tokens | `retention.revoked_token_days`, `retention.expired_token_days` | 90 days each | `APP_RETENTION_REVOKED_TOKEN_DAYS`, `APP_RETENTION_EXPIRED_TOKEN_DAYS` | none |
| `auth.expired_credentials` | Expired sessions, device login codes and magic links, counted from their expiry | `retention.auth_expired_days` | 30 days | `APP_RETENTION_AUTH_EXPIRED_DAYS` | none |
| `data.ai_history` | AI assistant chats idle past the window (with their messages) and AI usage metering | `retention.ai_history_days` | 90 days | `APP_RETENTION_AI_HISTORY_DAYS` | none |
| `data.restore_history` | Finished restores, point-in-time restores and backup drills | `retention.restore_history_days` | 90 days | `APP_RETENTION_RESTORE_HISTORY_DAYS` | none |
| `deploys.probe_attempts` | Per-attempt readiness probe detail | `retention.probe_attempt_days` | 90 days | `APP_RETENTION_PROBE_ATTEMPT_DAYS` | none |
| `deploys.app_history` | App lifecycle events, scheduled deploy history, last canary outcomes | `retention.app_history_days` | 90 days | `APP_RETENTION_APP_HISTORY_DAYS` | none |
| `observability.usage_cost` | Daily cost showback snapshots | `retention.usage_cost_days` | 90 days | `APP_RETENTION_USAGE_COST_DAYS` | none |
| `observability.status_incidents` | Resolved incidents and completed maintenance | `retention.status_incident_days` | 365 days | `APP_RETENTION_STATUS_INCIDENT_DAYS` | none |
| `docker.containers`, `docker.images`, `docker.build_cache`, `docker.volumes`, `deploys.attempts` | Janitor stages, read only | the janitor policy | see [automatic cleanup](/janitor) | none | none |

The seven rows from `auth.expired_credentials` down are `new` targets: they start in `preview`, so nothing is deleted until an administrator switches the target to `on`. They only remove finished or expired data: running restores and drills, live sessions, open or scheduled status incidents and chats with recent messages are always kept. Upgrade history is append-only by design and has no target, and finished pipeline runs are already pruned by the pipeline engine (newest 100 per pipeline, `APP_PIPELINE_KEEP_RUNS`).

The janitor stages only report. Their usage comes from the last recorded janitor pass, their mode follows the janitor policy (off, preview while it is in dry run, on), and the page links to Settings, Cleanup, Disk for changes.

### Environment overrides

Each older environment variable is now the env override of the same runtime setting, with the old default, so nothing changes until someone saves a value in the dashboard.

- Day counts (`APP_EMAIL_LOG_RETENTION_DAYS` and similar) accept any whole number of days, not only the presets. `0` keeps the data forever, except for the two audit variables, where `0` is ignored and the default applies (it always meant that for audit).
- `APP_EVENT_WEBHOOK_RETENTION` and `APP_ALERT_HISTORY_RETENTION` stay Go durations (`720h`) and also accept `30d`. They are converted to whole days, rounded up, so a window never shrinks (`36h` becomes 2 days).
- Other settings: `APP_CLEANUP_INTERVAL`, `APP_CLEANUP_ENABLED`.
- Limits, read from the environment only: `APP_CLEANUP_MAX_ROWS` (rows per run, default 100000), `APP_CLEANUP_SLICE` (time span of one delete, default 24h), `APP_CLEANUP_STAGGER` (default 5s), `APP_CLEANUP_STARTUP_DELAY` (default 5m), `APP_CLEANUP_HISTORY_DAYS` (default 365), `APP_CLEANUP_USAGE_TTL` (default 30s), `APP_CLEANUP_EXPORT_MAX` (CSV rows, default 10000), `APP_TOKEN_CLEANUP_BATCH` (token delete batch), `APP_CLEANUP_DELETE_BATCH` (rows per statement for the history targets, default 1000).

## Floors and weakening

- A target with a floor refuses a retention shorter than the floor. The API answers 400 with a field error that names the policy key, for example `retention.audit_log_days`. 0 (keep forever) is always allowed.
- Lowering the retention of a destructive target (the audit settings) needs `confirm_weakening: true` in the settings request, like other security settings. The change is audited by the usual `settings_change` row.
- The audit sweeper and the scheduled run raise a window below the floor (for example from an old environment variable) to the floor. The floors are fixed in code.
- Security audit entries never outlive their rule the other way either: their window is never shorter than the general window.

## Audit, events and alerts

- Every run, scheduled or manual, including dry runs, writes an audit row with action `cleanup_run`. The detail holds counts only: target, days, dry_run, rows, bytes, trigger and error. It never holds row contents.
- Each run emits `cleanup.run`, or `cleanup.failed` with the error, through the [event webhooks](/event-webhooks).
- A failed run sends one notice per target per day to the enabled notification channels.
- Run history lives in the `cleanup_runs` table and removes itself after 365 days.

## API

| Method and path | Ability | Purpose |
| --- | --- | --- |
| `GET /api/v1/cleanup` | Read | Targets with usage, days, mode, last and next run, plus scheduler state |
| `POST /api/v1/cleanup/{id}/preview` | Root | Rows and bytes a run would remove. Body `{"days": n}` is optional |
| `POST /api/v1/cleanup/{id}/run` | Root | Clean now. Body `{"days": n, "dry_run": bool}` is optional |
| `PUT /api/v1/cleanup/{id}/mode` | Root | Body `{"mode": "off\|preview\|on"}`; returns the updated target |
| `GET /api/v1/cleanup/runs` | Read | Paged history. Filters `target`, `since`, `until`, `dry_run`, `status` (`ok` or `failed`), `limit`, `cursor`; `format=csv` exports |

Preview and run answer `{"rows", "bytes", "dry_run", "more_remaining", "error"}` (`dry_run` is true for a preview or a dry run); `error` is an empty string on success, and a failure of the run itself is reported there with status 200 and recorded in history. The first-run review (`GET /api/v1/settings/runtime/onboarding`) adds an informational item with key `cleanup.preview_mode` whose value is the number of targets still in preview, janitor stages in dry run included. Each target in `GET /api/v1/cleanup` carries `source` (`default`, `env` or `saved`) for where its retention days came from, and `usage.bytes` is `null` when the store cannot measure size (API tokens).

## Add a target

A new target is one file and one registration line, about 30 lines for a table with a timestamp column.

1. Add the retention setting next to the others in `internal/runtimesettings/retention.go`: a key, `KindInt`, `AllowedInts: RetentionPresets`, the existing env var as `Env` (with `EnvConvert` if it is a duration) and `CheckValue: MinDaysCheck(floor)` if it needs a floor. A security-relevant setting also sets `Risk: RiskWeakensSecurity`.
2. In the store, add a count of rows older than a cutoff and use the existing delete function. For a table with a plain timestamp column, add it to `cleanupTables` in `internal/store/cleanup_stats.go` and the built-in helpers measure and count it.
3. Build a `cutoffTarget` in `internal/cleanup/builtin.go` with `Meta` (id, category, titles, policy setting, `Origin`, floor), a measure function, a count function and a delete function, then add it to `RegisterBuiltins`. `Origin: OriginNew` starts it in preview.
4. Add English strings under `cleanup:targets.<id>` in the dashboard locale file.
5. Test that `Preview` equals the rows `Run` removes, using the pattern in `internal/cleanup/builtin_test.go`.

A target that cannot use a cutoff (it deletes by count or by status) implements `Target` directly: `Measure`, `Preview(days)` and `Run(days, opts)`, honouring `opts.DryRun`, `opts.MaxRows` and the context.

## Not covered yet

Registry blob garbage collection, remote node stores, log size caps and scheduled VACUUM are not part of the cleanup center. Security audit tables, the auth library audit events and upgrade history are not covered: their floors and append-only rules stay as they are.
