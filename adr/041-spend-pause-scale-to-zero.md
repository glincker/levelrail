# ADR 041: A real spend pause that scales an app to zero

Status: Proposed

Date: 2026-10-23

## Context

A spend cap can alert, throttle, block new deploys and "pause" (see
`docs/spend-caps.md`). The pause is only an ingress response: domains answer
503 while every replica keeps running and costing money. The pause preview
(`spendcap.PreviewPause`) estimates the saving but changes nothing, and the docs
say a real pause needs its own design review. This is that review.

What exists and must be reused, not duplicated:

- `internal/spendcap.Controller` re-derives its decision from stored usage every
  two minutes, holds no state, and leaves enforcement alone when usage is
  unreadable. This is the level-triggered shape CLAUDE.md 4.2 requires.
- The application reconciler converges `DesiredService.Suspended` to zero
  containers on every resync. That flag also carries operator stop and
  sleep-when-idle, and the autoscaler treats it as "leave replicas alone".
- The autoscaler (ADR 036) writes only desired replicas and already refuses to
  add replicas while a spend action is enforced (`SpendBlocked`).
- Environments have `Protected`, with a recorded approval workflow
  (`deploy_approval`). Databases have their own `Suspended` flag and reconciler.
  Volumes are separate desired state that outlives containers.
- CLAUDE.md 4.14 asks for dry-run and confirmation before mass actions.

## Decision

1. **A distinct desired-state flag, not an imperative stop.** The cap action
   gains a fourth value, `scale_to_zero`. The controller records a per-app
   `spend_paused` field (with its period) in the same transaction as its state
   row. The application reconciler treats it like `Suspended` for convergence
   (zero containers, volumes and config untouched, condition `SpendPaused`), but
   the two flags stay separate: `Suspended` means an operator or the idle sleeper
   wants the app stopped, `spend_paused` means the cap does. Resume clears only
   what the cap set, so a cap lifting never wakes an app an operator stopped on
   purpose. The controller writes the flag; only the reconciler touches
   containers. The existing 503 page stays up for the whole pause.
2. **Desired replicas are preserved.** `replicas` and the autoscaler min and max
   are never rewritten. The reconciler converges to zero while the flag is set,
   so resume needs no memory of the previous count.
3. **Never paused by default:** databases (no cap scope, and other apps depend on
   them), apps mounting a volume that holds data, apps in a protected
   environment, and static sites (nothing to stop). Root opts one app in with
   `pause_policy`: `include_volumes` and `include_protected`, each its own
   boolean. An excluded app keeps the ingress-only pause, and the dry-run marks
   it "would not be stopped: reason". Databases have no opt-in (question 3).
4. **Autoscaler.** While `spend_paused` is set the autoscaler records a new
   blocker `BlockedSpendPaused` and writes nothing. It can neither scale up to
   escape the pause nor read zero replicas as a signal. On resume it continues
   from the preserved count and its normal cooldowns apply, so it cannot flap
   the app straight back down or up.
5. **Manual scale and deploy while paused** are stored as desired state but start
   nothing. The CLI and UI say "stored, app is spend paused, nothing started" and
   point at resume. `scale_to_zero` implies `block_deploys`.
6. **Resume rules.** Three triggers clear `spend_paused`; the reconciler then
   starts replicas:
   - Manual resume by root with a required reason. It also opens the existing
     override window (`APP_SPEND_OVERRIDE_TTL`) so the controller does not
     re-pause at once. Extensions per month are limited by
     `APP_SPEND_PAUSE_MAX_OVERRIDES`.
   - Rollover: on the first tick of a new UTC month, when usage is under the cap,
     the flag clears and a `rollover` event is recorded.
   - Cap raised or cleared: the next tick sees usage under the new limit.
   Each is decided from stored usage, so a missed tick delays resume by at most
   one tick.
7. **Control plane restart.** Nothing about a pause lives in memory. After a
   restart the flag is read from the store and the reconciler converges to zero
   again, a no-op if already stopped. Notices are gated by a stored `notified`
   marker, so a restart never repeats a final warning. If usage cannot be read
   the flag is held as it is: an outage never pauses and never resumes.
8. **Dry-run and arming before the first automatic pause.** The controller
   refuses to set `spend_paused` unless both hold:
   - A recorded dry-run. The existing preview is stored with a `dry_run_at` and
     a hash of what it listed (apps, replica counts, exclusions). It stops
     counting after `APP_SPEND_DRYRUN_MAX_AGE` or when the listed set changes.
   - An operator arm: `armed_by` and `armed_at`, set by root with
     `spend caps arm app NAME`. Arming without a valid dry-run fails.
   Without both, `scale_to_zero` degrades to the ingress-only pause and records
   `PauseNotArmed`, so nobody believes replicas are being stopped when they are
   not. Project caps need a dry-run listing every member and each member armed;
   unarmed members are skipped and named. Changing the action, the exclusions or
   the app set disarms.
9. **Notifications.** Reuse the `budget_threshold` rule and its channels. Warn
   percentages stay as today (default 50, 80, 100). A final warning fires at the
   last level below 100 (or a configured default when none) and states the
   action and a countdown at the recent run rate ("replicas stop at 100 percent,
   about 3 h 20 min from now"). A paused notice follows (what stopped, what was
   spared and why, how to resume), then a resumed notice. Each is sent once per
   period. Levels and the countdown horizon come from env, not constants.
10. **Audit.** Every transition is a spend-cap history event and an audit-log
    entry with actor `spend-cap-controller` or the root user: `armed`,
    `disarmed`, `dry_run_recorded`, `paused`, `resumed` (with trigger),
    `pause_skipped` (with reason), `override`. Manual actions carry the reason.
11. **Who may do what.** Arm, disarm, edit `pause_policy`, choose `scale_to_zero`
    and manual resume are root only. This is stricter than today's cap edit
    (deploy ability) because the action can stop production. Reading status
    stays on the read ability. Two-person approval is not added here: protected
    apps are excluded by default, and including one is a root opt-in (question 4).
12. **Failure modes and half success.**
    - Some replicas stopped, some not: the reconciler is idempotent and retries
      the rest each resync. The condition reads `SpendPauseIncomplete` with
      "n of m stopped" until none remain. A test for this case is required by
      AGENTS.md.
    - Ingress page up but containers running: the page and the flag are
      independent. The mismatch is shown, never hidden, and a failed stop never
      lifts the page.
    - Containers stopped but the page not applied (`EnforcementPending`):
      ingress already errors for an app with no backend, and the nudge retries
      each tick.
    - A node unreachable: its replicas cannot be stopped. The pause stays
      incomplete, the agent converges when it returns, and an alert names the
      node.
13. **Cold start cost on resume.** Resume is normal convergence from zero: image
    pull if evicted, container create, health check, then ingress adds the
    backend. The resume notice and the UI show the expected cold start (recent
    start-to-ready time when known). One replica starts first, the rest follow
    once it is healthy, so a database behind the app is not hit all at once. If
    the first replica fails its health check the app stays at one replica with
    condition `ResumeFailed` and is not re-paused in a loop.
14. **UI and CLI.** The Spend card shows `Active`, `Final warning`, `Paused
    (replicas stopped)`, `Paused (page only: not armed or excluded)` and `Paused
    incomplete`, with dry-run age, armed by, exclusions with reasons, next
    automatic resume, and Arm and Resume buttons for root. CLI: `spend caps
    arm|disarm|resume app NAME [--reason]`; `status` prints the same fields;
    `dry-run` prints "would stop" and "would spare: reason" per app. The API
    mirrors these under `/budget/`. MCP: read in read-only mode, arm and resume
    in full mode only.

## Rejected alternatives

- **An imperative "stop containers" call from the spend controller.** It is
  edge-triggered: a missed tick or restart leaves containers running, and it
  fights the reconciler and the autoscaler.
- **Reusing `Suspended`.** Resume could not tell a cap pause from an operator
  stop or idle sleep, and rollover would wake apps someone stopped deliberately.
- **Writing `replicas = 0` into desired state.** It destroys the intended count
  and needs a remembered value to restore.
- **The autoscaler owning the pause as "min 0".** It scales on a metric, and a
  stopped app has none. Spend is a hard limit, not a target.
- **Pausing everything in scope, including databases and volumes.** Spend control
  must not become a data availability incident.
- **Auto-arming after the first dry-run.** Arming is deliberate consent that a
  preview must not imply.
- **Pausing at 100 percent with no countdown.** The final warning is the only
  chance to act before production stops.
- **Two-person approval for every pause.** The cap is configured in advance, and
  a live approval at 3 am while spend runs is the failure being avoided.
  Approval belongs at arming time.

## Consequences

- An armed capped app really stops costing compute and egress until rollover, a
  raise or a root resume. Storage and idle node cost remain, as the preview says.
- The service and cap state rows gain fields, so this needs migrations and a
  reconciler change. Every place that honors `Suspended` (egress sidecars,
  ingress sleeping list, deploy watcher) must honor `spend_paused` too. Missing
  one is the main implementation risk.
- The two minute tick and cached telemetry mean a spike can pass the limit before
  the pause lands. The countdown warning is the early defense, not the pause.
- A paused app is an outage by design. Operators who cannot accept that keep
  `throttle` or `block_deploys`.

## What we do not do

- No code in this change. The slices below are separate work.
- No database or volume pausing, no partial per-replica pause, no pause for team
  budgets (they stay alert only).
- No automatic arming, and no AI in deciding or enforcing a pause.

## Open questions for the owner

1. Which level triggers the final warning when a cap has no warn level below 100
   (default 90, or none)?
2. Is rollover resume acceptable for every app, or should some need a manual
   resume (for example after an abuse spike)?
3. Should databases ever be pausable (a compute-only stop of a replica
   database), or is "never" permanent?
4. Does including a protected environment need a recorded second approver
   (`deploy_approval`), or is root opt-in enough?
5. How many manual override extensions per month, and how long, before an app
   must stay paused?
6. Dry-run validity: 30 days plus "app set unchanged", or shorter?
7. Project caps: pause every armed member at once, or one by one, lowest
   priority first?
8. Should the resumed notice use the same channels as warnings, or a separate
   rule so a 3 am resume does not page on-call?

## Slice plan

1. **Schema and flag.** `spend_paused` on the service; `armed`, `dry_run_at`,
   `pause_policy` and `notified` on the cap state; store methods and migrations.
   The reconciler converges the flag with its own condition and the
   half-succeeded test. No controller wiring yet.
2. **Dry-run record and arm.** Persist the dry-run hash, arm and disarm in API,
   CLI and audit log, exclusion reasons in the preview. Root only. Still no
   automatic pause.
3. **Controller action.** `scale_to_zero` sets and clears the flag, checks arm
   and dry-run validity, degrades to ingress-only with `PauseNotArmed`, adds the
   autoscaler blocker, rollover and cap-raise resume, and restart tests.
4. **Notifications.** Final warning with countdown, paused and resumed notices,
   once-per-period markers, env-driven levels.
5. **Manual resume and override.** Root resume with reason and limited
   extensions, scale and deploy behavior while paused, cold start handling.
6. **Dashboard, CLI status and docs.** Spend card states, Arm and Resume
   buttons, `spend caps status`, MCP tools, `docs/spend-caps.md` and `llms.txt`.
