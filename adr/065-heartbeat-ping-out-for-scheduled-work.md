# ADR 065: Heartbeat ping-out for scheduled work

Status: Proposed

Date: 2026-11-01

## Context

A dead-man's-switch service expects a ping every time a job succeeds and
alerts when the ping stops. That catches the failure that in-box alerts
cannot: the job that silently never ran. Operators run scheduled tasks,
backups and deploys on Levelrail and want to point such a service at them.

What exists today, checked in code:

- The off-host heartbeat (`internal/heartbeat`, documented in
  `docs/heartbeat.md`) pings one URL on a timer while the control plane is
  healthy, with an optional failure URL. It answers "is the server alive", not
  "did the nightly backup run". Its URL is stored encrypted, shown as a host
  only, and scrubbed from errors (`scrub`, `HostOf`). Its client refuses
  private destinations unless `APP_HEARTBEAT_ALLOW_PRIVATE` is set, and
  refuses redirects to another host.
- Event webhooks (`internal/eventhook/eventhook.go`) send a signed JSON POST.
  The main source is `emitAuditEvent` in `internal/api/event_webhooks.go`,
  which fires for successful audited API requests such as `apps.deploy`. The
  one non-request source found is the cleanup center (`cleanup.run`,
  `cleanup.failed`). No scheduled task or backup completion path emits an
  event. The delivery client uses `netguard.NewClientAllowing`, so private
  addresses are refused unless `APP_EXPORT_ALLOW_PRIVATE_NETWORKS` (or
  `APP_NOTIFY_ALLOW_PRIVATE_NETWORKS`) is set.
- The alert channel (`internal/alerting/notify.go`) posts JSON when a rule
  fires and also when it resolves (`Event.Resolved`). It can reach a failure
  URL, but it only runs on alert transitions, so it cannot send a periodic
  success ping.
- Scheduled task runs settle through one place in `internal/scheduledtask`
  (`Runner.record`) with a final status (success, failed, timeout and so on).
  Backups record a succeeded or failed status in the store.

The gap: no per-job success signal leaves the box.

## Decision

Add a small per-job ping-out, built from the pieces above rather than a new
subsystem.

1. Config shape. A ping URL and an optional fail URL are attached to the
   thing being watched:
   - a scheduled task gets `heartbeat_url` and `heartbeat_fail_url` fields;
   - backups get the same pair on the backup schedule (per volume or
     database schedule);
   - deploys use one pair per app, because a deploy has no schedule of its
     own.
   If only `heartbeat_url` is set, the fail URL defaults to the ping URL with
   `/fail` appended, which is the convention these services share. An explicit
   fail URL wins. A start ping is out of scope for the first version.
2. Which events ping. Success of a scheduled task run, of a backup run and of
   a deploy sends the ping URL. Failed, timed-out or interrupted runs send the
   fail URL. Runs skipped for concurrency or replaced by a newer run send
   nothing, so the service's own grace period decides.
3. Request. GET by default with no body, POST as an option, matching
   `APP_HEARTBEAT_METHOD` semantics. A POST body carries only a status word
   and the run id, no hostnames and no app names, as the platform heartbeat
   already does.
4. Egress and SSRF. Reuse the guarded client from `internal/heartbeat`
   (`NewClient`) and `ValidateURL`: http or https only, no credentials before
   the host, private, loopback and link-local destinations refused at dial time
   so a DNS name cannot smuggle one in, same-host redirects only, no proxy.
   Private destinations stay behind the existing opt-in
   (`APP_HEARTBEAT_ALLOW_PRIVATE`), so there is one switch for all off-host
   pings and no new one.
5. Timeouts and retries. The per-request timeout reads
   `APP_HEARTBEAT_TIMEOUT`. Pings never block or fail the job: they run after
   the run is recorded, in a bounded background queue. A failed ping retries a
   few times with exponential backoff and jitter, with the attempt count and
   delays read from new env variables (`APP_HEARTBEAT_JOB_MAX_ATTEMPTS`,
   `APP_HEARTBEAT_JOB_BACKOFF_BASE`, `APP_HEARTBEAT_JOB_BACKOFF_MAX`); no
   defaults are hardcoded in logic. A 4xx answer is not retried. After the last
   attempt the failure is recorded on the run as a ping error, visible in the
   run history, and the job result is unchanged.
6. Secrets. A ping URL usually embeds a token, so it is treated as a secret.
   It is stored through the secrets manager like the platform heartbeat URL,
   never in the task or schedule row. The API and UI return only the host and
   a configured flag. Errors pass through the existing `scrub` so net/http
   does not echo the URL, logs carry the host only, audit entries record that
   the field changed and never its value, and exports and backups of
   definitions omit it.
7. Out of scope. No generic outbound hook language, no inbound heartbeat
   changes, no new alert rule type. Event webhooks and the alert channel stay
   as they are; operators who want a JSON notification on failure keep using
   them.

Why not extend event webhooks or the alert channel: both are one-way
notifications of change, signed and JSON-only, and neither sees job success.
Making them emit periodic success would change their meaning for every
existing subscriber. A dedicated, tiny sender that reuses the heartbeat guard
keeps one egress policy and one secret-handling path.

## Consequences

- One more outbound path, but it reuses the existing guarded client, URL
  validation, secret storage and scrubbing, so the SSRF and logging rules do
  not fork.
- Ping delivery is best effort. A lost success ping makes the external service
  alert, which is the intended failure mode for a dead-man's-switch.
- Per-job URLs add secret rows. Rotation is replacing the value.
- Docs gain a section in `docs/heartbeat.md` and rows in
  `docs/environment-variables.md`.

## Tests needed

- URL validation: scheme, credentials, private host refused, opt-in allows it,
  errors never contain the URL.
- Fail URL derivation: default `/fail` suffix, explicit override, query string
  kept intact.
- Event mapping per kind: success pings, failure sends the fail URL, skipped
  and replaced runs send nothing.
- Retry policy with an injected clock: backoff, jitter bounds, 4xx not
  retried, ping error recorded and job status untouched.
- Secret handling: API responses and logs hold the host only; a captured log
  buffer never contains the token.
- Redirect to another host refused.

## Slices

1. Core sender: guarded client reuse, URL validation, fail URL derivation,
   retry queue, env config. Effort: 0.5 day.
2. Secret storage and API fields with masked reads, audit and export rules.
   Effort: 0.5 day.
3. Scheduled task wiring at the run settle point, plus CLI flags. Effort: 0.5
   day.
4. Backup schedule wiring and per-app deploy wiring. Effort: 1 day.
5. Dashboard fields and run-history ping status. Effort: 0.5 day.
6. Docs and tests above. Effort: 0.5 day.
