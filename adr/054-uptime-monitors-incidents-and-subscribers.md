# ADR 054: Uptime monitors, incidents from checks, and status page subscribers

Status: Accepted (slices 1 and 2 partly built, flag default off)

Date: 2026-10-29

## Implementation status

Behind `APP_MONITORS_ENABLED` (default off, see `docs/monitors.md`).

Built:

- Monitor model with `http` and `tcp` kinds, the control plane
  scheduler, batched result flush, hourly rollups and pruning
  (`internal/monitor`, migration `3060_monitors.sql`).
- `GET/POST /api/v1/monitors`, `DELETE /api/v1/monitors/{id}` and
  `GET /api/v1/monitors/{id}/results`.
- The confirmation state machine (`internal/monitor/state.go`: up,
  suspect, down, recovering) with `APP_MONITOR_CONFIRM_FAILURES` and
  `APP_MONITOR_RECOVERY_SUCCESSES`.
- A `monitor` status page component kind that follows the confirmed
  monitor state (`internal/statuspage/service.go`). One failed sample
  stays operational.

Not built yet:

- The `monitor_down` alert rule.
- Internal incidents with ack and publish, and the incidents and
  monitors dashboard pages.
- Alert and manual component sources, migration of `check` components,
  tls_cert, dns and heartbeat kinds, subscribers, deploy maintenance
  windows and node vantage points (phases 3 to 7).

The status page itself (single sampler, `app`, `domain` and `check`
components) is unchanged and stable; ADRs 055 (remote MCP) and 056
(advisory feed) do not cover monitors or the status page and stay
Proposed.

## Context

The question: can a Levelrail operator watch their public endpoints,
cron jobs and certificates, have a confirmed failure open an incident,
and tell their own users through the status page, without running a
second monitoring or status page product next to the platform? Today the
pieces exist but are not joined. Verified on `release/v0.8.11` by reading
the code:

1. **The status page has one sampler with one interval.**
   `internal/statuspage/service.go` runs `Sample` on a single ticker
   (`Run`, `:100`) driven by `Config.SampleInterval` (default 1 minute,
   `:47`, set from `APP_STATUS_PAGE_SAMPLE_INTERVAL`), with one global
   `CheckTimeout` (default 8 s, `:49`) and a parallelism of 8. `probe`
   (`:175`) knows three component kinds: `app` (reconcile conditions),
   `domain` (the DNS domain check) and `check`. `httpCheck` (`:220`) is a
   bare `GET` with no headers, body, expected status or keyword: 5xx or
   unreachable is an outage, 4xx is degraded, anything else is
   operational. It uses `kit/netguard`, and a blocked private address
   reports unknown (`:226`). There is no retry, no confirmation and no
   per component interval; one failed sample is one red sample.
2. **Storage keeps only daily counters.** Migration
   `internal/store/migrations/0142_status_page.sql` creates
   `status_page_settings` (a single row, `CHECK (id = 1)`),
   `status_components` (`kind`, `target`, `display_name`) and
   `status_component_daily` with `ok_samples`, `degraded_samples` and
   `down_samples` per UTC day. There is no latency, no per sample record
   and nothing finer than a day. `Sample` prunes days older than
   `UptimeDays` (90).
3. **Incidents are manual.** Migration `0143_status_incidents.sql`
   creates `status_incidents` (kind `incident` or `maintenance`, a
   public `status`, `impact`, a JSON list of `component_ids`) and
   `status_incident_updates`. Only an operator creates them (CLI and
   REST, `docs/status-page.md`). Nothing in alerting or the sampler
   writes an incident. `docs/status-page.md` states the limits plainly:
   no subscriber notifications (RSS is the push channel), measured from
   this control plane only, one page per control plane.
4. **Alerting is rich and already does the hard noise work.**
   `internal/alerting` has 18 notification channel kinds (`NotifyKind`
   constants in `rules.go`, counted in `docs/feature-status.md`), silences
   (`silence.go`), maintenance windows (`maintenance.go`, with
   `MaintenanceWindow.Covers`), and `NoiseControl` (`noise.go`) which
   applies debounce streaks, a flap tracker (`FlapThreshold`,
   `FlapWindow`), silences, inhibition, grouping and rate limits before
   delivery (`deliver`, `noise.go:286`). SLO burn rules live in `slo.go`
   and `slo_burn.go`, and the alert inbox in `inbox*.go`. I found no
   acknowledged state on a firing: a grep for `Acknowledge` in
   `internal/alerting` returns nothing outside tests.
5. **Certificates and domains are evaluated as rules, for platform
   managed resources.** `cert_expiry.go` reads certificates from a
   `CertSource` (the platform's own store) with a 14 day warning window
   and a renewal stalled threshold. `domain_health.go` evaluates the DNS
   state of domains attached to apps, with a per rule throttle. Neither
   dials an arbitrary external host.
6. **Heartbeat is outbound only.** `internal/heartbeat` pings an
   operator supplied URL while the control plane is healthy
   (`APP_HEARTBEAT_INTERVAL`, default 60 s, floor 5 s in `config.go`),
   with an optional fail URL, URLs kept in the secrets store, scrubbed
   from errors (`scrub`, `service.go:232`) and validated by
   `ValidateURL` against private addresses. It is a dead man switch for
   the platform itself. There is no inbound ping endpoint for a user's
   cron job.
7. **Scheduled maintenance already feeds the status page.**
   `docs/scheduled-maintenance.md`: a window shows a dashboard banner,
   sends one lead time notice to every enabled channel, lists itself on
   the public page, can freeze automatic deploys, and with `all` scope
   marks every component as under maintenance. Domain maintenance mode
   with an end time creates a window.
8. **Probing from nodes is plausible.** Nodes run an agent that the
   control plane reaches over a reverse dialed gRPC connection with mTLS
   (`docs/architecture.md`, `internal/agent/agentpb`). No probe RPC
   exists today.
9. **Signing and MCP limits.** Outbound webhooks are HMAC signed in
   `internal/webhook/webhook.go`. `docs/mcp-tool-surface.md` (generated)
   shows per toolset ceilings of 4000 estimated tokens and a ratchet on
   the total; the alerts toolset is already at 10 tools.
10. **ADR 051** proposes a read only incident analysis agent over alerts
    and deploys. A machine opened incident with a clean timeline is the
    input that agent wants.

So the gap is specific: there is no monitor resource with real
assertions, no result history finer than a day, no confirmation logic
on probes, no path from a confirmed probe failure to an incident, no
inbound heartbeat for user jobs, and no way to notify the people who
read the status page.

## Decision (recommended)

Add a first class **Monitor** resource and a small monitor engine in a
new `internal/monitor` package. Monitor results feed the existing
alerting pipeline as a new rule source, so silences, maintenance,
grouping, inhibition, flap control and the 18 channels are reused, not
rebuilt. A confirmed monitor alert may open an internal incident; a
human (or an explicit per monitor auto publish setting) decides whether
it becomes public. Add status page subscribers with double opt in email
and signed webhooks. Probe from the control plane first; nodes as probe
locations come later.

### 1. Monitor resource and engine

A monitor has: `id`, `name`, `kind`, `interval`, `timeout`,
`confirm_failures` (N), `recovery_successes` (M), `enabled`, `labels`,
`project` scope, and a kind specific `spec`.

| Kind | Spec | Up when |
| --- | --- | --- |
| `http` | URL, method (`GET`, `HEAD`, `POST`, `PUT`), headers (values may reference secrets), optional body, follow redirects (bool, max hops from env), expected status set (default 200 to 399), keyword assertions (`contains`, `not_contains`, on the first `APP_MONITOR_BODY_MAX_BYTES` bytes), optional max latency | every assertion holds |
| `tcp` | host, port | connect completes within the timeout |
| `tls_cert` | host, port, SNI | handshake succeeds, chain verifies, and NotAfter is beyond the warning window (default reuses the 14 day value from `cert_expiry.go`, overridable per monitor) |
| `dns` | name, record type (`A`, `AAAA`, `CNAME`, `MX`, `TXT`), optional expected values, optional resolver | the query answers and, if set, matches |
| `heartbeat` | period, grace | a ping arrived within period plus grace |

Engine rules:

- **Scheduling.** One scheduler goroutine keeps monitors in a min heap
  by next due time with a per monitor jitter (up to 10 percent of the
  interval) so monitors created together do not fire together. A worker
  pool of `APP_MONITOR_WORKERS` runs probes. A monitor never has two
  probes in flight; a slow probe delays its own next run, it does not
  stack.
- **Interval floors from env.** `APP_MONITOR_MIN_INTERVAL_HTTP`,
  `_TCP`, `_DNS` (suggested default 30 s), `APP_MONITOR_MIN_INTERVAL_TLS`
  (suggested 1 h, certificates do not change by the minute) and
  `APP_MONITOR_MIN_HEARTBEAT_PERIOD` (suggested 1 m). The API rejects a
  lower value with a message naming the env var. `APP_MONITOR_MAX_COUNT`
  caps monitors per control plane.
- **Timeouts.** Per monitor timeout, capped by `APP_MONITOR_MAX_TIMEOUT`
  and always below the interval.
- **SSRF guard reuse.** All dials go through a `kit/netguard` client or
  dialer control, as `httpCheck` does today, so the check happens on the
  resolved address at connect time, including after redirects and for
  DNS rebinding. A blocked address is a configuration error shown to the
  operator, not an outage. Private targets need the same explicit env
  opt in netguard already honors. The `dns` kind with a custom resolver
  is also dialed through the guard.
- **Inbound heartbeat.** `POST` or `GET` on
  `/public/monitors/heartbeat/{token}` (and `/{token}/fail` for an
  explicit failure, `/{token}/start` to measure run time). The token is
  32 random bytes, stored only as a hash, shown once, and rotatable with
  an optional overlap period during which both tokens work. The route is
  on the public allowlist, rate limited per token and per IP, and
  returns the same response for unknown and known tokens. The engine
  evaluates heartbeats on its tick; there is nothing to dial.
- **Existing `check` components** become a thin migration target: a
  `check` component can be converted to an `http` monitor with the same
  URL and the old status mapping. The old sampler path keeps working
  until slice 3 switches it.

### 2. Result storage

SQLite is the store, so the design keeps rows small, writes batched,
and long range reads on rollups only.

```sql
-- raw, short retention (APP_MONITOR_RAW_RETENTION, suggested 48h)
CREATE TABLE monitor_results (
    monitor_id  TEXT    NOT NULL,
    at          INTEGER NOT NULL,  -- unix seconds
    location    TEXT    NOT NULL DEFAULT 'cp',
    ok          INTEGER NOT NULL,  -- 0 or 1
    latency_ms  INTEGER,
    code        INTEGER,           -- HTTP status, DNS rcode, or NULL
    reason      TEXT,              -- short failure class, never a body or header value
    PRIMARY KEY (monitor_id, at, location)
) WITHOUT ROWID;

-- 1 minute rollups (APP_MONITOR_MINUTE_RETENTION, suggested 7d)
CREATE TABLE monitor_rollup_1m (
    monitor_id TEXT NOT NULL, bucket INTEGER NOT NULL,
    total INTEGER NOT NULL, failed INTEGER NOT NULL,
    lat_sum INTEGER NOT NULL, lat_max INTEGER NOT NULL,
    PRIMARY KEY (monitor_id, bucket)
) WITHOUT ROWID;

-- 1 hour rollups (APP_MONITOR_HOUR_RETENTION, suggested 400d)
CREATE TABLE monitor_rollup_1h (same shape as monitor_rollup_1m);

-- open and closed confirmed outages, the source for incidents and SLA
CREATE TABLE monitor_downtimes (
    id TEXT PRIMARY KEY, monitor_id TEXT NOT NULL,
    started_at INTEGER NOT NULL, ended_at INTEGER,
    reason TEXT, alert_fingerprint TEXT, incident_id TEXT
);
```

- Rollups are updated in the same transaction as the batch of raw
  inserts (an upsert per touched bucket), so no rollup job can fall
  behind. One write transaction per scheduler flush
  (`APP_MONITOR_FLUSH_INTERVAL`, suggested 5 s), not per probe, keeps
  the single SQLite writer free for the rest of the platform.
- Charts up to 24 h may read 1 minute rollups; anything longer and the
  90 day strip read only `monitor_rollup_1h`. Raw rows are for the
  "last results" table and debugging. Latency percentiles are not
  stored; the chart shows average and max. A p95 would need a histogram
  column and is left open.
- `status_component_daily` stays for `app` and `domain` components.
  Monitor backed components compute daily bars from hourly rollups.
- **Sizing (estimate, not measured).** A raw row is about 60 to 80 bytes
  including the key. 100 monitors at 30 s is 288k rows a day, about
  20 MB a day, so 48 h is about 40 to 50 MB. 1 minute rollups: 144k rows
  a day, 7 d is about 1M rows, about 60 MB. Hourly rollups for 400 days:
  about 960k rows, about 55 MB. Total under 200 MB for 100 monitors at
  the floor. Pruning runs in bounded batches (`DELETE ... LIMIT` loop)
  so it never holds the write lock for long.

### 3. Confirmation, recovery, alert, incident

Per monitor state machine, held in memory and persisted on change:

`up` to `suspect` on the first failure; `suspect` to `down` after N
consecutive failures (`confirm_failures`, default from
`APP_MONITOR_CONFIRM_FAILURES`, suggested 2); a success in `suspect`
returns to `up`. `down` to `recovering` on a success; `recovering` to
`up` after M consecutive successes (`recovery_successes`, suggested 2);
a failure in `recovering` returns to `down` without a new notification.
On `suspect` the engine may schedule one early retry after
`APP_MONITOR_RETRY_DELAY` instead of waiting a full interval, which cuts
detection time for long intervals.

How a failure travels:

1. **Monitor to alert.** A new alerting rule kind `monitor_down` with
   the monitor id as resource. The engine reports the confirmed state;
   alerting fires and resolves through `NoiseControl`, so flap control,
   silences, maintenance windows, grouping and inhibition apply
   unchanged. Monitors do not get their own flap logic; N and M are
   confirmation, flap suppression stays where it already lives.
2. **Alert to incident.** A monitor may set `incident: off | internal |
   auto_publish`. `internal` (the default) opens an incident record with
   state `triggered` and links the alert fingerprint and the downtime
   row. Alert grouping decides folding: several monitors down in the
   same group attach to one open incident.
3. **Internal state is separate from the public phase.** Add columns to
   `status_incidents`: `internal_state` (`triggered`, `acknowledged`,
   `resolved`), `source` (`manual`, `monitor`), `published` (0 or 1),
   `published_at`. The existing `status` column stays the public phase
   (`investigating` to `resolved`). An internal incident is invisible on
   the public page, JSON, RSS and to subscribers until someone publishes
   it with public text. `auto_publish` posts a fixed, operator written
   template, never probe output. Acknowledging is new: it stops
   escalation repeats and records who acknowledged, but does not resolve.
   When the monitor recovers, the internal incident resolves; a
   published incident gets a suggested "resolved" update the operator
   confirms, unless `auto_publish` is on.

### 4. Component state sources

A status component gets a `source`: `monitor` (one or more monitor ids,
worst state wins, or a quorum when monitors have several locations),
`alert` (state follows firing alerts matching a label selector, for
things that are not endpoints such as a queue backlog SLO), or `manual`
(operator set). The current `app`, `domain` and `check` kinds remain and
map to these internally. Incident impact keeps overriding on top, as
`combine` does today (`service.go:402`). Unknown never counts as down.

### 5. Maintenance around deploys

A deploy may create a short maintenance window scoped to the app, from
deploy start until the rollout is healthy plus
`APP_MONITOR_DEPLOY_GRACE` (suggested 2 m), capped at
`APP_MONITOR_DEPLOY_WINDOW_MAX` (suggested 30 m). It reuses
`MaintenanceWindow`, so `Covers` suppresses paging for that app's
monitors through the existing pipeline. Results are still recorded and
marked `in_maintenance`, so uptime math can exclude them by policy. The
window is closed by the deploy finishing, by the cap, or by a failed
deploy (which ends it immediately so a broken rollout pages). It is
off by default per app, not published to the status page, and does not
freeze deploys. Zero downtime deploys should not need it; it is for apps
that restart in place.

### 6. Subscribers

- **Email.** `POST /public/status/subscribe` with an address and
  optional component filter. A confirmation mail with a signed, expiring
  token (`APP_STATUS_SUBSCRIBE_CONFIRM_TTL`); nothing else is sent until
  confirmed. Every mail carries a one click unsubscribe link and the
  `List-Unsubscribe` and `List-Unsubscribe-Post` headers. Sent through
  the existing mail stack (`internal/email`, `internal/mailtemplates`).
- **Webhook.** Subscribe with a URL; at subscribe time the platform
  sends a challenge to the URL (through netguard) and the endpoint must
  echo it, which proves control and blocks using the page to aim traffic
  at a third party. Deliveries are HMAC signed with a per subscriber
  secret, reusing the signing in `internal/webhook`, with a timestamp to
  bound replay.
- **Folding.** Only published incident events notify: publish, each
  public update, resolve, and scheduled maintenance start and end.
  Updates within `APP_STATUS_NOTIFY_FOLD_WINDOW` (suggested 5 m) to the
  same incident fold into one message. A subscriber gets at most
  `APP_STATUS_NOTIFY_MAX_PER_HOUR` messages.
- **Retries.** Exponential backoff with jitter up to
  `APP_STATUS_NOTIFY_MAX_ATTEMPTS`; then the delivery is dropped and
  counted. A webhook failing for `APP_STATUS_WEBHOOK_DISABLE_AFTER`
  consecutive events is disabled and the operator sees it.
- **Bounces.** Hard bounces and complaints mark the address suppressed
  if the mail transport reports them; otherwise repeated send failures
  suppress it. Whether the current mail stack exposes bounce events is
  not verified (see below).
- **Abuse limits.** Subscribe is rate limited per IP and per address
  (`APP_STATUS_SUBSCRIBE_RPM`), unconfirmed signups expire and are
  deleted, total subscribers are capped (`APP_STATUS_SUBSCRIBERS_MAX`),
  responses do not reveal whether an address is already subscribed.
- **GDPR delete.** Unsubscribe deletes the row, it does not soft delete.
  An operator API deletes by address and exports the list. Delivery logs
  keep a hash of the address, not the address.

### 7. Vantage points

Phase one probes from the control plane only, labelled `cp`, and the
docs say so. Phase two adds nodes as probe locations through a new
`Probe` RPC on the existing reverse dialed agent connection: the control
plane sends the monitor spec, the node runs it with its own netguard
policy and returns the result. The monitor picks locations and a quorum
policy: `any` (down if any location confirms), `majority`, or `all`.
State is evaluated per location first, then the quorum decides the
monitor state, which separates "our network" from "their endpoint". A
location whose agent is offline is excluded from the quorum, not counted
as down. Nodes in one provider region are one location; region labels
come from node labels. **Browser flows (scripted clicks, page loads with
a real browser) are out of scope.**

### 8. API, CLI, MCP, Terraform, audit and IAM

- REST under `/api/v1/monitors` (CRUD, `results`, `rollups`,
  `downtimes`, `pause`, `rotate-token`) and
  `/api/v1/status-page/subscribers`, plus `incidents/{id}/ack` and
  `incidents/{id}/publish`.
- CLI: `levelrail-cli monitors create|list|get|update|delete|pause|results|rotate-token`
  and `status-page subscribers list|delete|export`.
- MCP: the budget is tight, so add at most two tools to the alerts
  toolset: `list_monitors` (read, includes current state) and
  `get_monitor` (read, recent results and downtimes). No mutating
  monitor tools in phase one, and publishing stays off MCP, as today.
  The tool surface doc is regenerated, not edited.
- Terraform: a `levelrail_monitor` resource and a
  `levelrail_status_component` source field, mapped one to one to the
  REST shapes.
- IAM: a new `monitors` resource with `read` and `write`;
  `status-page` keeps incidents and subscribers. Publishing needs
  `write` on `status-page` even when the incident was opened by a
  monitor. Every create, update, delete, rotate, ack and publish writes
  an audit event. Heartbeat tokens and header secrets never appear in
  audit payloads or API reads.

## Non-goals

Browser flows, synthetic transactions, multi page status sites, a
status page per project, paid SMS delivery, public latency graphs, and
long term raw result archives.

## Alternatives rejected

- **Extend the status page sampler in place.** It has one interval, no
  state per component beyond a string, and writes daily counters. Adding
  assertions there keeps monitoring tied to the page being enabled and
  mixes public rendering with probing.
- **Separate notification and noise logic for monitors.** Alerting
  already has flap control, silences, maintenance, grouping and 18
  channels. A second copy would drift.
- **Store every result forever, compute charts on read.** On SQLite this
  grows without bound and makes the 90 day strip a full scan.
- **Auto publish by default.** A probe can be wrong (our network, a DNS
  blip). Public text should be a decision; default is internal.
- **Probe from nodes first.** It needs a new RPC and quorum logic; the
  control plane probe answers most single region installs today.
- **Single opt in email.** Lets anyone enroll a stranger's address and
  makes the platform a spam relay.

## Security

- SSRF: every dial goes through netguard at connect time, including
  redirects, custom DNS resolvers, webhook subscribe challenges and
  deliveries. Private targets need the existing env opt in.
- Secrets in monitor headers are secret references, resolved at probe
  time, never returned or logged; errors are scrubbed like
  `heartbeat.scrub`.
- Raw results store a short failure class, never bodies or header
  values, so a keyword check cannot leak page content into storage.
- Public surface: heartbeat ping and subscribe routes are on the public
  allowlist with reasons, rate limited, and constant shaped responses.
  The public payload whitelist and its privacy test are extended, not
  bypassed; monitor targets never leave the control plane, the same rule
  as component targets.
- Heartbeat tokens are hashed at rest and rotatable.
- Subscriber webhooks are proven at subscribe time and signed.

## Failure modes

- **Control plane down.** No probes run, so nothing is recorded; gaps
  show as "no data", not as uptime. The outbound heartbeat in
  `internal/heartbeat` remains the signal for this case.
- **Control plane network partition.** Every external monitor fails at
  once. Mitigation: a canary check against a well known stable target
  (configurable) and inhibition rules so a canary failure suppresses
  monitor pages; node vantage points in phase two fix it properly.
- **SQLite write pressure.** Batched flush; if a flush fails, results
  are kept in a bounded buffer and dropped oldest first with a counter.
- **Scheduler overrun.** If due probes exceed workers, monitors run
  late, never in parallel with themselves; a lag metric alerts.
- **Clock jumps.** Scheduling uses monotonic time; buckets use wall time
  and a jump can leave one partial bucket.
- **Restart.** State is persisted on transition; on restart a `down`
  monitor stays `down` until recovery, so no duplicate page.
- **Mail outage.** Subscriber mail retries then drops; incidents still
  publish.

## What cannot be verified locally

- Real false positive rates across networks, and whether N=2 and M=2 are
  sensible defaults.
- SQLite behavior at the sizing above on small VPS disks; the numbers
  are estimates, not benchmarks.
- Bounce and complaint feedback: I did not confirm that the mail stack
  in `internal/email` exposes bounce events. If not, suppression falls
  back to send errors only.
- Deliverability of confirmation mail and `List-Unsubscribe` handling by
  receiving providers.
- Probe RPC cost on the agent connection and node side netguard policy.
- Whether alert grouping keys fold monitors the way operators expect.
- That no firing acknowledged state exists anywhere else; my grep in
  `internal/alerting` found none.

## Testing plan

- Unit: state machine table tests (N, M, early retry, recovery
  regression), each kind against `httptest` and local TCP, TLS and DNS
  test servers, netguard blocking for redirect and rebinding cases,
  rollup upsert math, pruning in batches.
- Store: migration test, retention and bucket boundaries.
- Alerting: `monitor_down` rule through `NoiseControl` with a silence
  and a maintenance window, and a deploy window that ends on failure.
- Incident: internal incident never appears in public view, JSON, RSS
  (extend the privacy test), publish requires `write`.
- Subscribers: unconfirmed gets nothing, unsubscribe deletes, fold
  window, retry cap, webhook challenge failure rejects subscribe.
- One focused test per slice; no broad guard tests that couple
  features.

## Phases

1. Monitor model, http and tcp kinds, control plane engine, raw results
   and rollups, CLI and REST.
2. Confirmation state machine, `monitor_down` alert rule, internal
   incidents with ack and publish.
3. Component sources (monitor, alert, manual), status page reads
   rollups, `check` components migrated.
4. tls_cert, dns and inbound heartbeat kinds with token rotation.
5. Subscribers: email double opt in, webhooks, folding, retries, delete.
6. Deploy maintenance windows.
7. Node vantage points and quorum.
8. Dashboard for each phase, Terraform resource, two MCP read tools.

## First thin slice (under 2 days)

Behind `APP_MONITORS_ENABLED`: new `internal/monitor` package with the
`Monitor` struct, the `http` (method, headers, expected status,
`contains` keyword) and `tcp` kinds, the heap scheduler with env
interval floors and timeouts through netguard, migration for `monitors`,
`monitor_results` and `monitor_rollup_1h`, batched flush and batched
pruning, `GET/POST/DELETE /api/v1/monitors` plus `results`, and
`levelrail-cli monitors create|list|results`. No alerts, no incidents,
no status page change. Exit: a monitor runs at its interval, results and
hourly rollups land, a private address is refused.

## Slices and file ownership

| Slice | Owns | Touches (shared, sequence) |
| --- | --- | --- |
| 1 engine | `internal/monitor/*`, new migration in `internal/store/migrations/`, `internal/store/monitors.go`, `internal/api/monitors.go`, `cmd/levelrail-cli/monitors.go` | `internal/api/router.go`, generated OpenAPI, `cmd/levelrail` wiring and env parsing |
| 2 alert + incident | `internal/alerting/monitor_down.go`, `internal/api/status_incident_ack.go` | `internal/alerting/rules.go`, `internal/store` incidents file, new migration altering `status_incidents` |
| 3 components | `internal/statuspage/sources.go` | `internal/statuspage/service.go`, `view.go`, privacy test, `docs/status-page.md` |
| 4 kinds | `internal/monitor/tls.go`, `dns.go`, `heartbeat.go`, `internal/api/monitor_heartbeat.go` | public allowlist and authorization matrix test |
| 5 subscribers | `internal/statuspage/subscribers.go`, `internal/api/status_subscribers.go`, new migration | `internal/webhook`, `internal/mailtemplates` |
| 6 deploy windows | `internal/monitor/deploywindow.go` | deploy pipeline hook, `internal/alerting/maintenance.go` |
| 7 vantage | agent probe handler | `internal/agent/agentpb` proto and generated code |
| 8 surfaces | dashboard routes, `internal/mcptools/tools_monitors.go`, Terraform provider resource | generated route tree, MCP surface doc |

Slices 2, 3 and 5 all touch `status_incidents` or the status page view;
run them in order. Migration versions are picked at push time from
`scripts/check-migration-versions.sh`, per `AGENTS.md`.

Env settings introduced (all with defaults, none hardcoded):
`APP_MONITORS_ENABLED`, `APP_MONITOR_WORKERS`,
`APP_MONITOR_MIN_INTERVAL_HTTP`, `APP_MONITOR_MIN_INTERVAL_TCP`,
`APP_MONITOR_MIN_INTERVAL_DNS`, `APP_MONITOR_MIN_INTERVAL_TLS`,
`APP_MONITOR_MIN_HEARTBEAT_PERIOD`, `APP_MONITOR_MAX_TIMEOUT`,
`APP_MONITOR_MAX_COUNT`, `APP_MONITOR_BODY_MAX_BYTES`,
`APP_MONITOR_CONFIRM_FAILURES`, `APP_MONITOR_RETRY_DELAY`,
`APP_MONITOR_FLUSH_INTERVAL`, `APP_MONITOR_RAW_RETENTION`,
`APP_MONITOR_MINUTE_RETENTION`, `APP_MONITOR_HOUR_RETENTION`,
`APP_MONITOR_DEPLOY_GRACE`, `APP_MONITOR_DEPLOY_WINDOW_MAX`,
`APP_STATUS_SUBSCRIBE_CONFIRM_TTL`, `APP_STATUS_SUBSCRIBE_RPM`,
`APP_STATUS_SUBSCRIBERS_MAX`, `APP_STATUS_NOTIFY_FOLD_WINDOW`,
`APP_STATUS_NOTIFY_MAX_PER_HOUR`, `APP_STATUS_NOTIFY_MAX_ATTEMPTS`,
`APP_STATUS_WEBHOOK_DISABLE_AFTER`.

## Consequences

- Operators get endpoint, port, certificate, DNS and cron monitoring and
  a page that notifies its readers, inside the platform they run.
- The alerting pipeline gains its first high frequency rule source; its
  rate limits and grouping will see more traffic than today.
- The control plane does more outbound network work and more SQLite
  writes; small installs must keep defaults conservative.
- `status_incidents` gains internal fields, so every public read path
  must filter on `published`. The privacy test is the guard.
- Uptime numbers change meaning for converted `check` components
  (confirmed downtime instead of raw failed samples); the docs must say
  so at the switch.

## Open questions

1. Should uptime exclude maintenance and deploy windows by default, or
   show them and let the operator choose?
2. Is acknowledged an alerting concept (on the firing) or only an
   incident concept? This ADR puts it on the incident.
3. Do we need latency percentiles (a histogram column) in phase one, or
   are average and max enough?
4. Does the mail stack report bounces and complaints, and if not, is
   send error suppression acceptable?
5. Should monitors be project scoped from day one or control plane wide
   like the status page?
6. What is the right default quorum once nodes probe: `majority`, or
   `any` for small clusters with two nodes?
