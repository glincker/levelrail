# Queue

Lightweight backlog, list format only. No narrative, no "why" writeups,
no completed-work log entries, those still belong in `TASKS.md` (Phase
0-2 build log) and `TASKS-v2.md` (dashboard redesign log). This file
answers one question: what's next. One line per item. Check items off in
place rather than deleting them, so the queue's own history stays
visible.

Run `scripts/verify.sh` before moving anything out of "In progress."

## 2026-08-14: Phase 3 landed on main

The separate concurrent session's `integration/phase3-consolidation`
branch (multi-node build routing, distributed cert storage, WireGuard
mesh, node health/cordon/drain, plus an org-wide icon migration from
`lucide-react` to `@phosphor-icons/react`, ADR 014) merged into `main`
today. Every item previously listed under "Blocked" in this file is
unblocked as of this merge; they moved to "Next up" below.

**Real incident, resolved clean**: the merge was executed directly
against the PM's own isolated `main`-tracking worktree (not the shared
checkout) by something outside this session's control, twice, each time
mid-conflict. Both times resolved with `git merge --abort` (the second
needed a `git reset --hard HEAD` after `--abort` partially failed on a
concurrently-edited file). No commits were ever lost, verified via
`git merge-base --is-ancestor` before touching anything. The resulting
merge commit was fully re-verified before trusting it: full backend
suite (`go test ./... -race`, all packages including all 9 e2e tests),
`golangci-lint run` (0 issues after clearing a stale cache), coverage
gate (83.2% vs 70%), full frontend (`tsc -b`, `eslint`, `prettier`,
`vite build`). Two files created earlier in this session
(`DeleteAppDialog.tsx`, `ui/toast.tsx`) still imported the now-fully-
removed `lucide-react`; fixed using ADR 014's own name-mapping table,
verified against the installed package's real exports. All future
frontend work must use `@phosphor-icons/react/dist/ssr`, not
`lucide-react`, this project's own icon convention changed for real.

## In progress

- [ ] MySQL as a third database engine: the creation wizard's step 1
      originally wanted this as a 5th option, dropped because
      `internal/reconcile/database/controller.go` only handled
      `postgres`/`redis`. Reconciler-core per CLAUDE.md section 8:
      claiming this one myself (PM), single-session, starting now,
      touches `internal/reconcile/database/*` and
      `internal/api/databases.go`, not the frontend files the other
      session is working in.

## Next up (priority order)

- [ ] Wire the new MySQL picker option into `CreateResourceWizard.tsx`
      once the reconciler item above lands (5th step-1 card, matching
      the design spec's original intent).
- [ ] Frontend main bundle chunk crossed the 500kB vite build warning
      threshold this round (`index-*.js`, currently ~501KB raw). Not
      urgent, but worth a look before it grows further: CLAUDE.md's own
      frontend rule calls for a bundle-size budget, not yet enforced in
      CI. Quick-checked (concurrent session): nothing obviously wasteful
      in root-level imports (`routes/__root.tsx`), `BrandIcon`/
      `@thesvg/react` isn't pulled into the always-loaded chrome, `zod`
      and `CreateResourceWizard` are already their own split chunks. The
      overage is marginal (~0.2% over threshold, 146kB gzipped), and a
      real fix needs actual bundle-composition tooling (a visualizer
      plugin isn't installed), not a guess. Leaving unclaimed rather than
      half-fixing it.
- [ ] A concurrent session merged a real, useful gap-close while this
      round was in flight: a notification bell (`NotificationBell.tsx`,
      polls node/certificate state), a toast-based consolidation
      retiring `success-message.tsx` entirely, and `cmd/levelrail-cli`
      (a scriptable client for app creation, config/client/git-detect
      pieces plus tests, an early slice of Phase 4's own "CLI:
      levelrail deploy/logs/exec/rollback" roadmap item). All already
      landed on `main` directly (not through this queue), noting here
      only so the history isn't a mystery to whoever reads this file
      next.

- [ ] Deploy strategy (rolling/recreate/blue-green) + replicas: defined
      in `internal/spec` but `store.DesiredService` has no such fields
      and the application controller has zero handling for either.
      Every deploy today is an implicit single-container image swap.
      Large, and this is reconciler-core per CLAUDE.md section 8 (one
      agent, one long session, human review of every decision, do not
      parallelize). Not dispatched yet for that reason.
- [ ] Railpack auto-detection: stubbed the same way static builds were
      (`internal/deploy/deploy.go`), but larger scope (real
      auto-detection heuristics, not just wiring). Do after static
      support lands and after the deploy-strategy work above, so
      Railpack-built images have somewhere real to go besides a bare
      image swap.

## Resolved by investigation, no build needed

- [x] Build-capable node routing: no separate frontend surface exists
      or is needed beyond the `accepts_build_workloads` toggle already
      in scope for the Nodes UI work above. `internal/build.SelectBuildNode`
      is pure internal reconciler logic with zero API exposure.
- [x] WireGuard mesh (`internal/reconcile/mesh`, `internal/network/*`):
      confirmed via grep, zero routes in `internal/api` expose any mesh
      state. Genuinely backend-only infrastructure right now, nothing
      for a frontend to build against. Re-check next time `internal/api`
      changes, don't build speculative UI ahead of a real contract.

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

- [x] Databases detail route gets the same dynamic/contextual sidebar
      treatment `/apps/$name/*` got, honestly scoped to what Databases
      actually has (one section, Overview, not eight): new
      `DatabaseScopedSidebar.tsx` (back link, name+status badge, one nav
      item), `AppSidebar.tsx` extended with a second, mutually-exclusive
      scope pattern for `/databases/$name`, `routes/databases/$name.tsx`
      split into a layout route (loader, header, delete action, Outlet)
      plus `routes/databases/$name/overview.tsx` (the Overview card +
      `ConditionsPanel` that used to live directly in the flat page) and
      `routes/databases/$name/index.tsx` (redirect to overview), mirroring
      `apps/$name.tsx`/`apps/$name/overview.tsx` exactly. New shared
      `lib/databaseStatus.ts` (mirrors `lib/appStatus.ts`) so the status
      rollup isn't copy-pasted between the layout header and the sidebar.
      No second Databases section found worth its own route: grepped for
      a connection-string/credentials UI, the only frontend hit is the
      unrelated change-password flow, and `cmd/levelrail`'s
      `database_credentials.go` is a CLI subcommand, not a web surface.
      Browser-verified end to end: real database created via the API,
      sidebar swaps to the scoped nav on `/databases/$name`, back-link
      returns to `/databases`, Overview content (engine/version/node,
      reconcile status) renders, hard navigation straight to
      `/databases/$name/overview` resolves correctly, and an app's own
      detail page was re-checked in the same session to confirm the
      shared scope-detection change didn't regress it.
- [x] Creation wizard + brand icons + dynamic sidebar (the full
      `docs/superpowers/specs/2026-08-14-creation-wizard-and-sidebar-design.md`,
      brainstormed with the founder): `BrandIcon.tsx` (`@thesvg/react`,
      4 tree-shaken brand logos), `AppSidebar.tsx`'s Vercel-style
      global/app-scoped dynamic nav plus floating variant, `/apps/$name/*`
      converted from client-side Tabs to real nested routes, and
      `CreateResourceWizard.tsx` replacing the old single-step
      `CreateAppDialog`/`CreateDatabaseDialog` with a 2-step
      pick-a-type-then-configure flow (Docker image, Dockerfile from
      git, Postgres, Redis). Browser-verified end to end: real app and
      database creation, real git-triggered build, real deletion,
      floating-sidebar visual confirmed, deep-linked nested routes
      confirmed on a fresh hard navigation. Landed across 3 rounds (7
      dispatched agents total across icons/sidebar/wizard), each
      independently re-verified before merge.
- [x] Manual build trigger from the dashboard: `POST /apps/{name}/builds`
      (`internal/api/builds.go`), invokes the same `internal/deploy.Pipeline`
      the git webhook uses given a `repo_url`/`ref` in the request body
      (no app has a stored git config anywhere in this codebase, so it's
      asked for on every call, not stored once). Synchronous/blocking,
      no build-log streaming yet, that SSE endpoint genuinely doesn't
      exist (`useDeployLogStream.ts`'s own doc comment says its contract
      is "assumed," not backed by a real route). Frontend: a second tab
      on the deploy trigger card.
- [x] `build.type: static` support: a static site gets its own
      `store.StaticSite` table (migration 0015), bypassing the
      application controller entirely (no image, no port, nothing to
      converge to a container). `internal/deploy.Pipeline` copies build
      output to a durable directory under the data dir, the ingress
      controller lists both desired services and static sites every
      pass and builds either a `reverse_proxy` or `file_server` route.
      Backend only; no frontend surface for creating/viewing a static
      site exists yet (flagged as its own follow-up gap by the builder,
      not actioned).
- [x] TLS certificate renewal visibility: `GET /api/v1/certificates`
      parses stored certmagic leaf certs directly (no new tracking
      needed), read-only card on the General settings page
      (healthy/expiring_soon/expired). No last-renewal-error signal
      exists to surface, since this codebase's TLS automation only
      drives Caddy's offline internal issuer today; real ACME renewal
      tracking is a separate, not-yet-built gap.
- [x] Fixed a real Base UI "expected a native &lt;button&gt;" console
      warning, but not the one my own initial dispatch note guessed at:
      the 11 `DialogTrigger render={<Button/>}` sites I originally
      flagged as the suspects turned out to be fine (`DialogTrigger`
      merges its own props onto the `Button` element via
      `cloneElement` rather than injecting a `render` prop, so `Button`
      still renders its own native `<button>` correctly there,
      confirmed both by reading Base UI's own source and by live
      browser click-testing all 11 after the fix, zero warnings). The
      real bug was 2 sites where `Button` itself was given a `render`
      prop pointing at something that isn't a button
      (`routes/apps/index.tsx`'s "New database" CTA rendering a
      `Link`, `routes/settings/general.tsx`'s Support/Documentation
      buttons rendering an `<a>`), missing the `nativeButton={false}`
      Base UI's own docs say is required whenever `Button`'s `render`
      target isn't a native button. Correcting the record here since
      the original dispatch note above was wrong about the mechanism,
      even though it correctly caught that something was broken.
- [x] Node health/cordon/drain/workload-capability UI: new
      `/nodes/$id` detail route, `CordonNodeDialog`, `DrainNodeDialog`
      (target-node picker, renders the real 200/207 partial-success
      shape), workload-capability `Switch` toggles. `NodeRow` now shows
      two independent badges (connectivity from `status`, cordon from
      `schedulable`, confirmed via grep that nothing in the backend
      actually ever writes `status: "cordoned"`) plus a "Manage" link.
      Browser-validated end to end against a live backend: created a
      real app, confirmed the reconciler converged it
      (`Ready: AlreadyRunning`), confirmed Delete/Node-field/env-save
      all render correctly on `pm-main`'s own dev server.
- [x] "Saved." success-message consolidation: new
      `components/ui/success-message.tsx`, 6 call sites
      (HealthCheckEditor, DomainEditor, ResourceLimitsEditor, EnvEditor,
      SecretsEditor, DeployTriggerForm) switched over. Browser-verified:
      saving an app's env variables renders the shared component's
      green "Saved." text correctly.
- [x] Domain-conflict now surfaces as a real 409 naming the conflicting
      domain instead of a generic 500 (`handleCreateApp`/`handleUpdateApp`
      never checked for `store.ErrDomainTaken`, which the backend has
      enforced since the Phase 3 merge). No frontend change needed, the
      existing generic error display already surfaces any non-ok
      response's message verbatim.
- [x] Previously-built image tag dropdown for `DeployTriggerForm`
      (`GET /api/v1/apps/{name}/images`, `ImageLister` Router
      dependency, repo derivation via `github.com/distribution/reference`
      to correctly handle registry:port hosts).
- [x] App deletion dialog (`DeleteAppDialog`) + node placement display
      on the app detail page.
- [x] e2e test: full auth lifecycle (register -> login -> change
      password -> revoke other sessions), against a real
      `httptest.NewTLSServer`-wrapped router.
- [x] e2e test: rollback actually converges (image swap both
      directions, verified against the Docker Engine API).
- [x] e2e test: node placement affects transport, honestly scoped to
      what's provable without real multi-node infra: an
      `application.Controller` only ever calls `InspectByName`/
      `Create`/`Start` through the `docker.Runtime` it was constructed
      with (verified via a spy wrapper around two real Runtime values,
      one of them a real gRPC agent transport).
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
