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

- [ ] Add an e2e test that creates a Redis database via the API, lets
      it reconcile, and verifies a real running container + volume via
      the Docker Engine API. Done: `test/e2e` has database-reconciliation
      coverage (currently zero). (builder dispatched)
- [ ] Add an integration test proving a non-default app.yaml port is
      actually published on the container and routed by Caddy
      end-to-end. Done: a fixture on a non-default port deploys and
      answers over HTTPS on the right upstream port. (builder dispatched)
- [ ] Add an integration test deploying a fixture with a plain env var
      and a `{ secret: true }` env var, asserting via ContainerInspect
      that only the resolved plaintext lands in the container's env.
      Done: a real, repeatable test instead of a one-time manual check.
      (builder dispatched)
- [ ] Add an integration test proving `internal/telemetry`'s stats
      collector captures a real running container's CPU/memory and that
      it's queryable via `GET /api/v1/apps/{name}/metrics` end-to-end.
      Done: metrics collection has a test touching a real container,
      not just unit-level pieces. (builder dispatched)
- [ ] Add a toast/notification primitive under `web/src/components/ui`
      and wire it into the database and node delete flows. Done: a
      successful delete gives visible feedback outside the dialog it
      was triggered from, not just a silent list refresh. (builder
      dispatched)

## Next up (priority order)

- [ ] Wire `internal/secrets.Manager` into `internal/reconcile/database`'s
      Postgres path (`cmd/levelrail/main.go`'s dynamic controller
      source: generate/store per-database credentials, pass via
      `database.WithPostgresCredentials`). Done: creating a Postgres
      database through the API converges to a running container with a
      real password instead of the permanent credentials-blocked
      condition. **Reconciler-core + secrets-adjacent: PM does this one
      personally, not delegated (CLAUDE.md 8).**
- [ ] Add a `docker.Runtime` health-check method and wire it into
      `GET /api/v1/system/status` and the General settings page. Done:
      frontend shows real Docker daemon connectivity instead of
      omitting it (`status.go`'s own comment names this exact gap).
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

- [x] Virtualize the Databases list table (already done when
      `routes/databases/index.tsx` was originally built, verified via
      grep during queue triage, not a real gap).
