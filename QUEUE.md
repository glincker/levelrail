# Queue

Lightweight backlog, list format only. No narrative, no "why" writeups,
no completed-work log entries, those still belong in `TASKS.md` (Phase
0-2 build log) and `TASKS-v2.md` (dashboard redesign log). This file
answers one question: what's next. One line per item. Check items off in
place rather than deleting them, so the queue's own history stays
visible.

Run `scripts/verify.sh` before moving anything out of "In progress."

A separate, concurrently-running Claude Code session owns Phase 3
backend work (multi-node build routing, distributed cert storage, the
WireGuard mesh, node health/cordon/drain) and is consolidating it on its
own `integration/phase3-consolidation` branch. Items below marked
"Blocked" depend on that landing on `main` first; do not build against
those APIs before they merge, the shapes can still change.

## In progress

(nothing yet, round 3 not dispatched)

## Next up (priority order)

- [ ] Expose `docker.Runtime.ListImages` via a small API route (e.g.
      `GET /api/v1/apps/{name}/images`) so `DeployTriggerForm` can offer
      a dropdown of previously built tags instead of a hand-typed
      image string. Safe to build now, docker-health-check (the other
      item that touched `internal/api/router.go` /
      `cmd/levelrail/main.go`'s `rootHandler`) has already landed.
- [ ] Look for more real, scoped gaps the same way the last two rounds
      did: re-scan for duplicated inline styling, missing test coverage
      on real user-facing paths, and small honest UX gaps against
      Coolify/Dokploy (`docs-local/competitor-clones/`). Not a fixed
      list, PM triages fresh each round.

## Blocked

- [ ] Route the default build path through the registry-backed remote
      BuildKit cache and build-capable node routing (blocked on:
      `integration/phase3-consolidation`, "build-capable node routing +
      remote BuildKit cache").
- [ ] Close the domain-uniqueness-across-deploys gap in the frontend
      (surface the 409/validation error from create/update) and wire
      multi-node ingress instances to share cert state in the UI
      (blocked on: `integration/phase3-consolidation`, "certmagic
      distributed cert storage + domain uniqueness enforcement").
- [ ] Make a service's internal DNS name resolve correctly after a
      cross-node placement move, the literal Phase 3 exit criterion
      (blocked on: `integration/phase3-consolidation`, "internal/network
      WireGuard mesh + DNS").
- [ ] Surface node health (last-seen/heartbeat) and cordon/drain
      controls in the Nodes management UI (blocked on:
      `integration/phase3-consolidation`, "node health/cordon/drain
      API, TASKS.md 3.7").

## Backlog (not yet prioritized)

- [ ] **Needs a design decision, not just wiring**: a one-click
      "restart" action per app turns out not to be a simple frontend
      task. Checked `internal/reconcile/application/controller.go`'s
      `Reconcile`: the container name itself is keyed by
      `ContainerName(serviceName, desired.Image)`, so re-POSTing the
      exact same image tag to `POST /apps/{name}/deploys` is a genuine
      no-op, the reconciler sees no diff and does nothing, no restart
      happens. A real restart needs either a new backend endpoint that
      force-recreates the container regardless of desired-state diff,
      or a decision that "restart" isn't a first-class operation this
      architecture's immutable-container-per-tag design should support
      at all. Don't dispatch a builder on this until that's decided.
- [ ] Once the deploy-attempt-log design note lands (needs founder
      sign-off first, see Done below), add a deploy-history list UI
      with a one-click "rollback to this build" button, reusing the
      existing `POST /api/v1/apps/{name}/deploys` endpoint with an
      older image tag.

## Done (most recent first)

- [x] Consolidated 6 files' hand-rolled status-badge color classes
      (green/red/amber) into `Badge`'s new `success`/`warning` variants
      plus its existing `destructive`. Found during queue triage: real,
      repeated inline-styling that should have been shared component
      variants from the start.
- [x] Added a "paste a .env block" bulk-import affordance to
      `EnvEditor.tsx` (Coolify/Dokploy both support this; the editor
      was add-one-row-at-a-time only).
- [x] Docker daemon connectivity on `GET /api/v1/system/status` +
      General settings page display. `Ping` on the concrete
      `*docker.Client`, not `docker.Runtime` (would have rippled into
      6+ fake implementations). Live-verified: `docker_connected: true`
      against a running instance.
- [x] Documented + tested `internal/reconcile/application`'s
      `resolveEnv` `SecretEnv`-over-`Env` precedence: confirmed
      deterministic, not map-iteration-order luck.
- [x] Deploy-attempt ID scheme + build-log persistence design note.
      Recommendation: opaque `crypto/rand` ID in a new `deploy_attempts`
      table (`internal/store`) + a dedicated log table in `telemetry.db`
      shaped like `internal/telemetry/logs.go`, not live-only and not
      reusing `log_entries` directly. Full note:
      `docs-local/research/deploy-attempt-id-and-log-persistence.md`
      (gitignored, ask the founder to read it, needs sign-off before
      anyone implements this).
- [x] Wire `internal/secrets.Manager` into `internal/reconcile/database`'s
      Postgres path. Creating a Postgres database now converges to a
      real running container with real generated credentials. Live-
      verified end to end (real `postgres:16` container, `Ready: True`).
      PM did this one personally (reconciler-core + secrets-adjacent).
- [x] e2e test: Redis database reconciliation (`test/e2e/database_test.go`).
- [x] e2e test: non-default port routing through Caddy
      (`test/e2e/port_mapping_test.go`, `test/fixtures/hello-e2e-altport/`).
- [x] e2e test: plain + secret env var injection
      (`test/e2e/env_test.go`), first e2e coverage of `internal/secrets`.
- [x] e2e test: container-to-HTTP metrics seam (`test/e2e/metrics_test.go`).
- [x] Toast notifications for successful database/node deletion
      (`web/src/components/ui/toast.tsx`, shadcn Base UI `toast`
      registry component).
- [x] Virtualize the Databases list table (already done when
      `routes/databases/index.tsx` was originally built, verified via
      grep during queue triage, not a real gap).
