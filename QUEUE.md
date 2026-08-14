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

(nothing yet, round 2 not dispatched)

## Next up (priority order)

- [ ] Add a `docker.Runtime` health-check method and wire it into
      `GET /api/v1/system/status` and the General settings page. Done:
      frontend shows real Docker daemon connectivity instead of
      omitting it (`status.go`'s own comment names this exact gap).
- [ ] Document `internal/reconcile/application`'s `resolveEnv` precedence
      (a `SecretEnv` key silently wins over a literal `Env` entry of the
      same name) with a doc comment and a unit test. Found by the env-var
      e2e agent as a real, unintentional-looking gap, not a defect, just
      undocumented and untested. Small.
- [ ] Write a short design note deciding the deploy-attempt ID scheme
      and whether build logs persist for replay or stay live-only
      (options: telemetry log store vs. a new table). Done:
      `web/src/queries/deployLogs.ts`'s "assumed backend contract, not
      implemented" comment gets replaced by a real, agreed contract.
      Design decision only, no code, until a human signs off on the
      choice (this is ADR-worthy per CLAUDE.md 7).

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

- [ ] Expose `docker.Runtime.ListImages` via a small API route (e.g.
      `GET /api/v1/apps/{name}/images`) so `DeployTriggerForm` can offer
      a dropdown of previously built tags instead of a hand-typed
      image string.
- [ ] Add a "paste a .env block" bulk-import affordance to
      `EnvEditor.tsx`, parsed client-side into the existing key/value
      fields (Coolify and Dokploy both support this; current editor is
      add-one-row-at-a-time only).
- [ ] Add a one-click "restart" action per app (stop+start without a
      new image tag) so an operator does not have to retype the current
      image into the deploy trigger form just to force a restart.
- [ ] Once the deploy-attempt-log design note lands, add a deploy-history
      list UI with a one-click "rollback to this build" button, reusing
      the existing `POST /api/v1/apps/{name}/deploys` endpoint with an
      older image tag.

## Done (most recent first)

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
