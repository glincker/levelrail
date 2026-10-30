# ADR 061: Internal incidents from monitors and alerts, with draft then publish

Status: Proposed

Date: 2026-10-29

Builds on ADR 054 (uptime monitors, incidents from checks, subscribers),
section 3 "Alert to incident" and section 8 "API, CLI, MCP, Terraform,
audit and IAM". ADR 054 named the idea; this ADR fixes the exact shape,
the hook points in today's code, and the slice order.

## Context

Verified on `release/v0.8.12` by reading the code, not only the docs:

1. **The monitor state machine confirms, per monitor.**
   `internal/monitor/state.go` `Tracker.Observe` moves a monitor through
   `up`, `suspect`, `down`, `recovering` and calls the `Notifier` only on
   a confirmed `down` or a confirmed `up` (`Transition{Monitor, State,
   Reason}`). Counts come from `APP_MONITOR_CONFIRM_FAILURES` and
   `APP_MONITOR_RECOVERY_SUCCESSES`. The tracker is in memory: a fresh
   process starts every monitor in `up`, and an `up` sample in `up`
   fires nothing.
2. **The `monitor_down` rule is one platform wide rule, not one per
   monitor.** `internal/alerting/monitor_down.go` `EvaluateMonitorDown`
   fires while `MonitorsDown()` is non empty and joins every down monitor
   into one message. `MonitorDownTracker` is also in memory ("a restart
   starts clear"). So the alert engine's firing and resolved edges say
   "at least one monitor is down", not which one. A second monitor going
   down while the rule already fires produces no new alert edge.
3. **The wiring lives in boot.** `cmd/levelrail/boot_monitors.go`
   `startMonitors` builds the `Notify` callback that records into
   `MonitorDownTracker` and logs. It returns early unless
   `APP_MONITORS_ENABLED` is true (`monitor.ConfigFromEnv`).
4. **Silences and maintenance are reusable as plain functions.**
   `internal/alerting/noise.go` `FindMute(silences, windows,
   AlertContext, now)` reports whether an active silence or enabled
   maintenance window covers an alert context. `NoiseControl` applies it
   inside dispatch.
5. **Incidents are operator authored and always public.** Migration
   `internal/store/migrations/0143_status_incidents.sql` creates
   `status_incidents` (`kind`, `title`, public `status`, `impact`,
   `component_ids` JSON, times) and `status_incident_updates`
   (`incident_id`, `status`, `body`, `created_at`). `SaveStatusIncident`
   in `internal/store/status_page.go` is an upsert. Every stored incident
   is read by the public page: `internal/statuspage/service.go` lists up
   to 500 incidents for the snapshot and folds `impact` into component
   state (`combine`), and `/public/status/incidents/{id}` reads by id.
   There is no visibility flag anywhere.
6. **Other readers of the same table.** `internal/api/mobile_overview.go`
   (operator view), `internal/mcptools/tools_alert_noise.go`
   `list_status_incidents` (via REST), `levelrail-cli status-page`,
   `internal/store/cleanup_retention.go` (prunes `resolved` and
   `completed` by `updated_at`) and `internal/demo/lint.go`.
7. **Authz and audit.** Status page routes in
   `internal/api/routes_status_page.go` use
   `requireAbilityForResource(AbilityRead|AbilityWrite,
   statusPageResource, ...)`; monitor routes use `monitorsResource`.
   Mutating routes are audited by `recordAudit` (`internal/api/audit.go`)
   through `RouteAudited`.
8. **Latest migration on the branch is `3060_monitors.sql`.**

The gap: a confirmed outage pages through `monitor_down`, but nothing
records it as an incident, nobody can acknowledge it, and the only way to
tell users is to retype it as a public incident by hand. Putting an
internal incident in today's table would leak it to the public page on
the first read, because every row is public.

## Decision

**Recommendation: open internal incidents from the per monitor confirmed
transition, not from the `monitor_down` alert edge; store them in
`status_incidents` with an additive `published` flag that defaults to
public for existing rows; publish only through an explicit draft then
publish step with operator written text.**

### 1. Where an incident is created

- **Source of truth is `monitor.Transition`.** A new `IncidentOpener`
  (in `internal/incident`, new package) is called from the same
  `Notify` callback in `boot_monitors.go`, after
  `MonitorDownTracker.Record`. On `StateDown` it opens or reuses an
  incident for that monitor; on `StateUp` it marks the monitor's link
  recovered. Using the transition gives one incident per monitor outage
  with the monitor id and reason, which the platform wide rule cannot.
- **Mute check before open.** The opener builds the same
  `AlertContext` the `monitor_down` rule would use (rule kind, monitor id
  and name as labels) and calls `FindMute` with active silences and
  maintenance windows. A muted outage does not open an incident; it
  writes one structured log line with the silence id. Silences keep one
  meaning across paging and incidents.
- **The `monitor_down` rule must exist and be enabled.** If an operator
  disabled the rule, the opener does nothing. This keeps a single switch
  for "I do not care about monitor outages".
- **Folding.** At most one open (not `resolved`) incident per monitor,
  enforced by a partial unique index. Monitors that share a status page
  component fold into the open incident that already lists that
  component, within `APP_INCIDENT_FOLD_WINDOW`. No cross component
  grouping in this ADR.
- **Any other alert: manual promote only.** An operator may promote a
  firing alert from the inbox to an internal incident
  (`POST /api/v1/incidents` with `source: alert` and the rule id). No
  automatic incident from other rule kinds in this ADR. Rule kinds differ
  too much in meaning (a cert 30 days out is not an incident) and most
  have no per resource edge, same as `monitor_down`.
- **Manual incidents are unchanged.** `POST
  /api/v1/status-page/incidents` keeps creating public incidents with
  `source = manual`, `published = 1`.

### 2. Internal lifecycle: triggered, acknowledged, resolved

`internal_state` is separate from the public `status` column:

| From | Event | To | Who |
| --- | --- | --- | --- |
| (none) | confirmed down, not muted | `triggered` | system |
| `triggered` | ack | `acknowledged` | operator |
| `triggered`, `acknowledged` | all linked monitors confirmed up | `resolved` | system |
| `triggered`, `acknowledged` | resolve | `resolved` | operator |
| `resolved` | same monitor down within `APP_INCIDENT_REOPEN_WINDOW` | `triggered` (reopen, same id) | system |

- Ack records `acknowledged_by` and `acknowledged_at`, adds an internal
  timeline entry, and stops repeat escalation for alerts linked to the
  incident. It does not resolve and does not touch the public status.
- System resolve on recovery applies only to `source = monitor`.
  An operator resolve while a monitor is still down is allowed and
  audited; the next confirmed down after the reopen window opens a new
  incident.
- Reopen within the window avoids a pile of one minute incidents from a
  flapping endpoint that slipped past confirmation.

### 3. Draft then publish

- A new incident has `published = 0`. It is invisible on the public
  HTML, JSON, RSS, Atom, history, badge and incident pages, and does not
  feed `impact` into component state. The `monitor` component kind
  already reflects the confirmed probe state on its own (ADR 054), so the
  public page still shows red for a down monitor; only the narrative
  stays private.
- An operator writes a draft: `draft_title`, `draft_body`, public
  `status` and `impact`, `component_ids`. Saving a draft
  (`PUT /api/v1/incidents/{id}/draft`) changes nothing public.
- `POST /api/v1/incidents/{id}/publish` copies the draft into the public
  `title`, a first public timeline entry, sets `published = 1`,
  `published_at`, `published_by`. Publishing needs an explicit request
  with the incident's current `updated_at` (optimistic check), so a
  stale form cannot publish over a newer draft.
- Nothing from probe output (`Reason`, response bodies, hostnames of
  private targets) ever reaches a public field automatically. The draft
  is prefilled from a template that uses only the monitor display name
  and the linked component names. No `auto_publish` in this ADR (see out
  of scope).
- Unpublish is not offered. A wrong publish is corrected with a public
  update, as today.
- On system resolve of a published incident the server writes a
  suggested public "resolved" draft update; the operator confirms it.

### 4. Timeline

- `status_incident_updates` gains `visibility` (`public` default,
  `internal`) and `author`. System events (opened, reopened, monitor
  recovered, folded monitor added) and ack, resolve and draft saves are
  `internal`. Public updates are only written by publish and by
  `POST .../updates` on a published incident.
- Every public read path filters `visibility = 'public'` and
  `published = 1`. Operator reads show both, marked.
- Internal update bodies are capped by `APP_INCIDENT_UPDATE_BODY_MAX`;
  the public cap stays as today.

### 5. Authz matrix

| Action | Route | Resource | Ability | Audited |
| --- | --- | --- | --- | --- |
| List and get internal incidents | `GET /api/v1/incidents`, `GET /api/v1/incidents/{id}` | `status-page` | read | no |
| Promote alert to incident | `POST /api/v1/incidents` | `status-page` | write | yes |
| Ack | `POST /api/v1/incidents/{id}/ack` | `status-page` | write | yes |
| Resolve | `POST /api/v1/incidents/{id}/resolve` | `status-page` | write | yes |
| Save draft | `PUT /api/v1/incidents/{id}/draft` | `status-page` | write | yes |
| Add internal note | `POST /api/v1/incidents/{id}/notes` | `status-page` | write | yes |
| Publish | `POST /api/v1/incidents/{id}/publish` | `status-page` | write | yes, with incident id and title, never body |
| Existing public incident routes | unchanged | `status-page` | unchanged | unchanged |

- Reading monitor details from an incident still requires `read` on
  `monitors`; the incident view shows the monitor id and name only.
- MCP: `list_status_incidents` keeps its read class and gains an
  `internal_state` field. No mutating incident tools, publishing stays
  off MCP, same as ADR 054.
- Audit payloads never carry probe reasons, header values or draft
  bodies.

### 6. Migration (additive only)

One new migration, next free number at merge time (3061 or later),
SQLite dialect like the rest:

```sql
ALTER TABLE status_incidents ADD COLUMN source TEXT NOT NULL DEFAULT 'manual';          -- manual | monitor | alert
ALTER TABLE status_incidents ADD COLUMN source_ref TEXT;                                 -- monitor id or rule id
ALTER TABLE status_incidents ADD COLUMN internal_state TEXT NOT NULL DEFAULT 'resolved'; -- triggered | acknowledged | resolved
ALTER TABLE status_incidents ADD COLUMN published INTEGER NOT NULL DEFAULT 1;
ALTER TABLE status_incidents ADD COLUMN published_at TEXT;
ALTER TABLE status_incidents ADD COLUMN published_by TEXT;
ALTER TABLE status_incidents ADD COLUMN acknowledged_at TEXT;
ALTER TABLE status_incidents ADD COLUMN acknowledged_by TEXT;
ALTER TABLE status_incidents ADD COLUMN internal_resolved_at TEXT;
ALTER TABLE status_incidents ADD COLUMN draft_title TEXT;
ALTER TABLE status_incidents ADD COLUMN draft_body TEXT;
ALTER TABLE status_incident_updates ADD COLUMN visibility TEXT NOT NULL DEFAULT 'public';
ALTER TABLE status_incident_updates ADD COLUMN author TEXT;
CREATE UNIQUE INDEX idx_status_incidents_open_source
    ON status_incidents (source, source_ref) WHERE internal_state != 'resolved' AND source_ref IS NOT NULL;
CREATE TABLE status_incident_links (
    incident_id TEXT NOT NULL REFERENCES status_incidents(id) ON DELETE CASCADE,
    monitor_id  TEXT NOT NULL,
    down_at     TEXT NOT NULL,
    up_at       TEXT,
    PRIMARY KEY (incident_id, monitor_id, down_at)
);
```

- Defaults keep every existing row public and internally resolved, so a
  rollback to the previous binary still renders the page correctly (the
  old binary ignores new columns, but it would also show unpublished
  rows, see failure modes).
- No column is dropped, renamed or retyped. `status` keeps its meaning.
- Retention: `cleanup_retention.go` keeps pruning by public `status`;
  unpublished rows are pruned when `internal_state = 'resolved'` and
  older than the same retention.

### 7. Flags and env

All read at boot, invalid or unset values fall back to the documented
default, same pattern as `statuspage.TuningFromEnv` and
`monitor.ConfigFromEnv`.

| Variable | Default | Meaning |
| --- | --- | --- |
| `APP_MONITORS_ENABLED` | off | Parent gate. Nothing in this ADR runs without it. |
| `APP_INCIDENTS_ENABLED` | off | Opens internal incidents from monitors and enables `/api/v1/incidents`. Ignored unless the parent is on. Routes answer 404 when off. |
| `APP_INCIDENT_FOLD_WINDOW` | 10m | Fold a new down into an open incident on the same component within this window. |
| `APP_INCIDENT_REOPEN_WINDOW` | 15m | Reopen a resolved monitor incident instead of opening a new one. |
| `APP_INCIDENT_SWEEP_INTERVAL` | 1m | Reconcile open monitor incidents against monitor state (see failure modes). |
| `APP_INCIDENT_UPDATE_BODY_MAX` | 4096 | Byte cap for internal notes and drafts. |
| `APP_INCIDENT_OPEN_MAX` | 200 | Cap on open internal incidents; beyond it new outages fold into one overflow incident and log a warning. |

Defaults are suggestions for the docs and the config parser, not
constants scattered in code.

### 8. Out of scope

- Automatic incidents from alert kinds other than `monitor_down`.
- `auto_publish` and any publish without a human request.
- Subscriber notifications for incidents (ADR 054 phase on subscribers).
- On call schedules, escalation policies and paging rotations beyond
  what alert channels already do.
- Postmortem documents, incident roles, severity taxonomies beyond
  `impact`.
- Multiple status pages, per project incidents.
- Persisting the monitor state machine across restarts (handled by the
  sweep instead).
- Dashboard pages beyond a list, a detail view with ack, resolve, draft
  and publish. Layout follows the existing status page screens.

## Alternatives rejected

- **Open incidents from the `monitor_down` alert firing edge.** The rule
  is platform wide: a second monitor going down while it fires has no
  edge, and the resolved edge only comes when every monitor is up. It
  would also need parsing the joined message to recover monitor ids.
  Rejected.
- **Make `monitor_down` a per monitor rule first.** Correct long term,
  but it changes rule storage, defaults and notification volume for a
  feature that only needs the transition that already exists. Can follow
  later without changing this ADR's data model.
- **A separate `incidents` table, joined to `status_incidents` on
  publish.** Cleaner separation and no leak risk on old read paths, but
  two timelines, a copy step that can drift, and every operator view
  needs a union. The additive columns plus filtered reads are smaller.
  Kept as the fallback if the leak risk below proves hard to close.
- **Publish by flipping a flag on the same row without a draft.** The
  public title would be whatever the system wrote, which is where probe
  text leaks. Rejected in favour of a draft field.
- **Auto open from every alert kind.** Too noisy and semantically wrong
  for warnings such as cert expiry. Rejected; manual promote covers it.

## Security

- The main risk is leaking an internal incident or its probe reason to
  the public page. Mitigations: `published` defaults to 0 for system
  created rows; one store function (`ListPublicStatusIncidents`) is the
  only read the public page, feeds and badge may call; a test lists every
  public route and asserts an unpublished incident and an internal
  update are absent from each.
- Monitor reasons may contain internal hostnames, status lines or body
  snippets. They live only in internal timeline entries and are never
  copied into drafts.
- Publish requires `write` on `status-page`; a token scoped to
  `monitors` cannot publish.
- All mutating routes are audited; audit rows carry ids and titles, not
  bodies.
- The partial unique index and the `APP_INCIDENT_OPEN_MAX` cap bound
  the write rate a flapping or hostile target can cause.

## Failure modes

- **Restart while a monitor is down.** Both trackers reset to clear,
  the monitor fires no `up` transition, so the incident would stay open
  forever. The sweep every `APP_INCIDENT_SWEEP_INTERVAL` resolves an
  open monitor incident when the tracker has observed the monitor and
  reports `up` with no failure streak, and leaves it open while the
  tracker reports `down` again after re-confirmation. A monitor deleted
  while linked resolves its link with an internal note.
- **Rollback to a binary without this ADR.** The old binary reads all
  rows and would show unpublished incidents publicly. The release note
  must say: resolve or delete unpublished incidents before downgrading,
  or keep `APP_INCIDENTS_ENABLED` off. This is the main operational cost
  of the shared table choice.
- **Opener error (DB busy, constraint hit).** The opener runs off the
  probe worker (the `Notifier` contract says it must not block), on a
  bounded queue; on overflow it drops and logs, and the sweep catches up.
  Paging through `monitor_down` is unaffected.
- **Silence created after an incident opened.** The incident stays; the
  silence only affects new opens and paging.
- **Concurrent publish and system resolve.** The optimistic `updated_at`
  check makes one of them retry; publish never overwrites a newer
  internal state.
- **Clock skew between nodes.** Not relevant: monitors run on the
  control plane only today.

## What cannot be verified locally

- Real outage timing end to end (confirmation counts, alert
  `ForDuration`, sweep interval) against a real endpoint over minutes;
  unit tests drive the tracker and opener with fake clocks.
- Behaviour on the QA droplet after a control plane restart during a
  live outage.
- Rollback behaviour on a real database copy with unpublished rows.
- How operators actually use draft and publish, and whether the fold
  window is right. The defaults are guesses.
- Performance of filtered public reads at the 500 row snapshot limit on
  a large history; expected fine, not measured.

## Testing plan

One focused test per slice, no blanket suites:

- Store: migration applies on a copy of a pre ADR schema; old rows read
  as public and resolved; partial unique index rejects a second open
  incident for one monitor.
- Opener: fake tracker transitions open, fold, reopen, resolve; a
  matching silence prevents open; disabled `monitor_down` rule prevents
  open.
- Public leak test: one table driven test over every `/public/status*`
  route, JSON, feeds and badge with one unpublished incident and one
  internal update present.
- API: authz table test for each new route (read vs write, wrong
  resource), audit row written for each mutation, publish with stale
  `updated_at` returns 409.
- Sweep: restart scenario with an open incident and a tracker that
  reports `up`.

## Slice plan

Each slice is 10 to 20 minutes, one agent, own worktree. Shared files
are marked; slices touching the same shared file run in order.

| # | Title | Owns (new files) | Touches (shared) | Env |
| --- | --- | --- | --- | --- |
| 1 | Incident columns migration and store reads | `internal/store/migrations/<next>_incident_internal.sql`, `internal/store/status_incident_internal.go` and test | `internal/store/status_page.go` (column list, `ListPublicStatusIncidents`), `internal/store/cleanup_retention.go` | none |
| 2 | Public read filter and leak test | `internal/statuspage/public_filter_test.go` | `internal/statuspage/service.go` (switch to public list), `internal/api/status_page_public.go`, `internal/api/mobile_overview.go` | none |
| 3 | Incident opener, fold, reopen, sweep | `internal/incident/opener.go`, `internal/incident/config.go`, tests | `cmd/levelrail/boot_monitors.go` (call opener in `Notify`, start sweep) | `APP_INCIDENTS_ENABLED`, `APP_INCIDENT_FOLD_WINDOW`, `APP_INCIDENT_REOPEN_WINDOW`, `APP_INCIDENT_SWEEP_INTERVAL`, `APP_INCIDENT_OPEN_MAX` |
| 4 | Incident API: list, get, ack, resolve, notes, promote | `internal/api/incidents.go`, `internal/api/routes_incidents.go`, tests | `internal/api/router.go` (register routes), `internal/api/openapi_gen.go` (regenerated, not edited) | `APP_INCIDENT_UPDATE_BODY_MAX` |
| 5 | Draft and publish | `internal/api/incidents_publish.go` and test | `internal/api/routes_incidents.go` (after slice 4), `internal/store/status_incident_internal.go` (after slice 1) | none |
| 6 | CLI, MCP field and docs | `cmd/levelrail-cli/incidents.go` | `internal/apiclient/alert_noise.go`, `internal/mcptools/tools_alert_noise.go`, `docs/monitors.md`, `docs/status-page.md`, MCP tool surface doc (regenerated) | none |

Order: 1, then 2 and 3 in parallel (no shared files between them), then
4, then 5, then 6. Slice 2 must land before 3 is enabled anywhere, so
no unpublished row can exist without the public filter. The dashboard
list and detail view is a follow up slice once 5 lands, owned by the web
app files only.

## Consequences

- Operators get a private record of every confirmed outage with ack and
  resolve, and choose what users see.
- `status_incidents` now holds private rows; every new public read path
  must use the public list function. The leak test enforces it for
  existing routes, not for routes added later without it.
- Downgrade needs care while unpublished rows exist.
- `monitor_down` keeps paging as today; incidents are a parallel record,
  not a replacement for alerts.
- A later per monitor alert rule can feed the same opener without a
  schema change.

## Open questions

1. Should publish need a distinct ability (for example `publish` on
   `status-page`) so a responder can ack and draft without being able to
   speak publicly? This ADR uses `write` to match ADR 054; the authz
   layer may not support a third ability on a resource yet.
2. Is the rollback leak acceptable, or should slice 1 instead use a
   separate internal table (the rejected alternative) to make downgrades
   safe by construction?
3. Should ack also silence the `monitor_down` alert, or only stop repeat
   escalation? Silencing would mute other monitors too, since the rule is
   platform wide.
4. Fold by shared component, or never fold and let the operator merge?
5. Should a muted outage still open an incident marked muted, for the
   record, instead of opening nothing?
