# Dashboard Redesign (v2)

Status: research in progress (started 2026-08-13)

## Why this file exists, not TASKS.md

TASKS.md is the Phase 0-2 build log: backend, reconciler, observability
collection, auth foundation. It is already large and mostly settled. This
initiative is different in kind, not just another checklist item: a
ground-up redesign of the frontend's shell and information architecture
(left sidebar, "like Railway/Vercel," per the founder's own framing,
current shell called out explicitly as "very simple and literally
nothing"). Keeping it in its own file avoids burying a large, still-moving
design effort inside an already-settled build log, and avoids TASKS.md's
existing "Frontend: dashboard UI" section (which documents the *current*
shell) reading as contradicted by an in-progress redesign of that same
shell.

Once the shell lands and stabilizes, fold a short summary back into
TASKS.md the same way every other completed initiative in this project
is recorded (evidenced `[x]` entries, not a rewrite of this file's
process notes), and this file can be archived or deleted. Until then,
this is the single source of truth for the redesign.

## Ground rules (same discipline as TASKS.md, restated because a whole
new file invites drift)

- Research before build. No implementation work starts on the new shell
  until a wireframe has been reviewed and approved.
- Real evidence, not vibes: research docs cite actual colors, spacing,
  copy, or structure from what was actually found, not paraphrased
  impressions. Say explicitly when something couldn't be verified.
- No em dashes (—) or en dashes (–) anywhere, same as every other file
  in this repo.
- Parallel work streams get an explicit, non-overlapping file list, per
  the standing rule that independent work needs a clear exit criterion
  and a defined set of files it may touch. Collisions get resolved by
  re-scoping the task list, not by hoping two streams don't touch the
  same line.
- Scope check before adding anything here: Levelrail is Phase 1, single
  admin, no teams, no RBAC yet. A pattern borrowed from
  Vercel/Railway/Linear that assumes multi-tenant/team features doesn't
  belong on this list yet just because the source product has it well
  designed; note it as a Phase 4 candidate instead if it's genuinely
  gated on that scope.

## Phase R: Research (parallel research streams, no code changes)

- [x] `docs-local/research/dashboard-redesign/vercel-dashboard-analysis.md`:
      done 2026-08-13. Live authenticated dashboard unreachable (login
      wall); sourced from Geist design docs, the official Feb 2026
      dashboard-redesign blog/changelog, and two cross-corroborating
      reverse-engineered token extractions, all confidence-tiered
      inline. Confirmed: a real resizable/hideable sidebar shipped Feb
      2026 (replacing horizontal tabs), unified team/project nav via
      "projects as filters," Cmd+K command menu, documented deploy
      states (queued/building/error/ready), Badge/Table/Entity/Tabs/
      Skeleton/Empty-state component specs, an Observability tab with a
      click-drag-to-zoom metrics interaction.
- [x] `docs-local/research/dashboard-redesign/railway-dashboard-analysis.md`:
      done 2026-08-13. **Correction to the "sidebar like Railway/Vercel"
      framing**: Railway's actual product is canvas-first, a pannable
      real-time node graph of services with a top bar (workspace/env
      switcher, Settings, Observability), not a persistent left sidebar.
      Flagging this now, before Phase D, so the wireframe isn't built on
      a false premise. Still highly relevant for non-shell patterns:
      full deployment status state machine with inline recovery actions
      (Restart/Rollback/Redeploy/Cancel), tabbed service detail
      (Deployments/Variables/Metrics/Backups/Settings), deploy-boundary
      markers overlaid on metric charts, synced cross-chart cursors,
      shift-drag-to-zoom, structured log filter query language
      (`@attribute:value`, boolean ops, ranges), Cmd+K palette.
- [x] `docs-local/research/dashboard-redesign/cross-cutting-sidebar-patterns.md`:
      done 2026-08-13. **Highest-leverage finding of the three**:
      shadcn/ui's registry ships a `base` (Base UI) variant of every
      block under `apps/v4/registry/bases/base/`, matching Levelrail's
      actual `@base-ui/react` dependency exactly, not the default
      Radix docs examples. Pulled real source for `sidebar-07` (icon-
      collapse nav shell) and `dashboard-01` (fuller shell with KPI
      cards, secondary nav group, data table) directly from
      `github.com/shadcn-ui/ui`. Recommendation: build from these blocks
      rather than from scratch, skip the team/workspace switcher
      (single admin, nothing to switch between yet), use icon+label
      badges over bare status dots (corroborates Vercel's `StatusDot`
      convention), reuse the `SectionCards` KPI pattern, keep the
      first-run empty state to one line + one CTA.

## Phase D: Design synthesis and wireframe (gated: needs founder review before Phase 1 starts)

- [x] Synthesized all three research docs into a concrete direction:
      build the shell from shadcn's `base` (Base UI) variant of
      `sidebar-07`/`dashboard-01`, skip the team/workspace switcher
      (single admin), two-zone sidebar (primary nav in `SidebarContent`,
      a pinned secondary group for Settings/Docs above the account
      footer via `NavUser`), icon+label status badges modeled on
      Vercel's `StatusDot` state model, deploy markers on metric charts
      and structured log filtering from Railway, tab organization
      (Overview/Domains/Env/Secrets/Metrics/Logs/Alerts) per app
      instead of Railway's canvas.
- [x] Built an interactive HTML wireframe via the Artifact tool
      (2026-08-13): sidebar shell (collapsible), app list with KPI
      strip and status badges, app detail with working tabs (Overview,
      Domains, Env vars, Secrets, Metrics with a deploy-marker chart,
      Logs with a structured filter bar and live-tail indicator, Alert
      rules), an API tokens settings screen, and a first-run empty
      state. Reuses Levelrail's real light/dark oklch tokens verbatim
      from `web/src/index.css`; the only new tokens are proposed
      semantic status colors (success/warning/info), called out
      explicitly in the wireframe itself since they don't exist in the
      codebase yet. Published, awaiting founder review before any
      implementation work touches `web/src` for the shell itself.
- [ ] Record the approved direction here once reviewed (adjustments,
      sign-off, or a different direction entirely). Blocks Phase 1.

## Phase 1: Shell

- [x] **Real shell built (2026-08-13)**, approved by the founder looking
      at the live preview rather than a formal wireframe sign-off step:
      `npx shadcn add sidebar` (Base UI variant, confirmed via
      `@base-ui/react` imports in the generated `sidebar.tsx`) installed
      `sidebar.tsx`, `sheet.tsx`, `tooltip.tsx`, `skeleton.tsx`,
      `use-mobile.ts`, no `package.json` changes (all built on
      already-installed deps). New `web/src/components/AppSidebar.tsx`:
      `Sidebar collapsible="icon"` with a brand-mark header (real
      `useBrand()` data, no team switcher, per the research's "skip it,
      nothing to switch between yet" recommendation), a primary nav
      group (Apps, real active-route highlighting via
      `useRouterState`), a `mt-auto`-pinned secondary group (API
      tokens, Documentation linking to the real `brand.DocsURL`), and a
      footer with the real signed-in username (`useAuthUsername`) and a
      working sign-out button (`useLogout`). `web/src/routes/__root.tsx`
      rewritten: `AppShell` now renders `SidebarProvider > AppSidebar +
      SidebarInset` for any authenticated route, and just `<Outlet />`
      (no chrome) for `/login`, replacing the old flat top-nav bar
      entirely. Fixed a real `react-hooks/set-state-in-effect` lint
      violation in the shadcn-generated `use-mobile.ts` (lazy
      `useState` initializer instead of setting state synchronously
      inside the mount effect) since installing that file makes it
      this repo's problem to keep lint-clean, not vendored-and-ignored.
      `tsc --noEmit`/`eslint`/`vite build` all clean on every file this
      change touched (a separate, unrelated, concurrently in-progress
      edit to `CreateAlertRuleDialog.tsx` from the other active session
      working on alerting notification channels was failing `tsc -b`
      at the same time; confirmed via the error list that none of the
      failures were in any file this pass touched, left untouched and
      uncommitted as it wasn't in scope). Live-verified via `curl`
      against the real embedded binary (real login, real 200 on
      `/apps`); a from-scratch browser screenshot check hit this
      session's known "script injection timeout" browser-automation
      issue (independently confirmed by three separate polish passes
      earlier in this session, not caused by this change), so this
      was not visually screenshotted, flagged explicitly rather than
      claimed.
- [x] **Dark mode merged (2026-08-13)**: took the dark-mode work's two
      independent, non-conflicting new files as-is
      (`ThemeProvider.tsx`: context + `localStorage['levelrail-theme']`
      persistence + `matchMedia` system-preference sync;
      `ThemeToggle.tsx`: light/dark/system cycling icon button) plus its
      `index.html` flash-of-wrong-theme inline script (all three
      independently correct regardless of shell shape). Its `__root.tsx`
      diff (added `ThemeToggle` to the old top-nav) was superseded by
      the shell rewrite rather than reapplied: wired `ThemeProvider`
      around `RootLayout` and `ThemeToggle` into the new
      `SidebarInset` header (right-aligned, next to `SidebarTrigger`)
      by hand instead. `tsc`/`eslint`/`prettier`/`vite build` clean,
      live-verified via the running preview server.
- [ ] Reassess the KPI-strip/`SectionCards` pattern for an apps
      overview once the shell has real content flowing through it
      (deferred, not blocking: the current `/apps` route already has
      its own list-page polish from the earlier pass).

## Phase 2: Screen migration into the new shell

- [x] All five content-polish passes landed and are running inside the
      real sidebar shell (2026-08-13): app list (`apps/index.tsx`,
      `AppRow.tsx`), app detail (`apps/$name.tsx` rebuilt on `Tabs`,
      `AppOverview`/`ConditionsPanel`/`DeployTriggerForm`/
      `DomainEditor`/`EnvEditor`/`SecretsEditor`), observability
      (`MetricsDashboard`/`LogSearchPanel`/`AlertRulesPanel`/
      `CreateAlertRuleDialog`/`DeleteAlertRuleDialog`/the per-deploy
      build-log route), and auth/settings (`login.tsx`,
      `LoginForm`/`RegisterForm`, `settings/tokens.tsx`,
      `TokenTable`/`CreateTokenDialog`/`RevokeTokenDialog`). Each
      merged individually with its own `tsc`/`eslint`/`prettier`/`vite
      build` verification, not a bulk copy.
- [x] **Real bug found and fixed while merging observability**:
      `apps/$name/deploys/$deployId/logs.tsx` (the live SSE build-log
      viewer) has been a structurally unreachable route since it was
      first added (`routeTree.gen.ts` confirms it nests under
      `/apps/$name`, but `$name.tsx` never rendered an `<Outlet />` in
      any version, checked back through its full git history, not a
      regression from this pass). Fixed: `$name.tsx` now renders just
      `<Outlet />` (stepping fully aside, not nesting the build-log view
      inside the tab shell, since `DeployLogsPage` already renders its
      own complete header) whenever the URL contains `/deploys/`.
      **Deliberately not fixed in the same pass**: there is still no
      in-app link to that route, because there is no backend
      deploy-history/attempt-listing endpoint to source a `deployId`
      from yet (`internal/api`'s own doc comment: `GET .../deploys`
      returns current reconcile conditions, not an attempt-by-attempt
      log). Building that is a real, store-schema-sized feature (the
      database schema is explicitly on the do-not-parallelize list),
      not a wiring fix, so it wasn't improvised here. **Follow-up task,
      not yet started**: a real
      `deploy_attempts` (or similar) table plus a list endpoint, then a
      "recent deploys" UI in the Overview tab linking each entry to its
      build log.
- [x] **Merge coordination note**: `CreateAlertRuleDialog.tsx` was
      independently modified by the other concurrent work (email/
      Telegram notification channels, commit `1caabff`) while the
      observability polish work was still based on the prior version.
      Hand-merged rather than overwritten: kept both the
      channel additions (email/Telegram options, per-channel
      destination validation) and the visual additions (kind-select
      icons, dialog title icon), verified together with `tsc --noEmit`
      afterward.

## Phase 3: Rich interactions (stretch, prioritize from research findings)

Not detailed yet. Candidates already flagged by the research asks:
command palette (cmd+k), richer empty/loading states, keyboard
navigation. Whether any of these make the cut depends on the "lowest
effort, highest value" synthesis the cross-cutting research doc is
asked to produce, not assumed here.

## Phase 4: Account and settings features (real functionality, not just a shell)

Prompted directly by the founder looking at the live sidebar shell and
pointing out that competitors (Coolify named specifically) show real
account/settings screens and a way to actually create something, even
before a single app exists, not just an empty state. This phase is
about closing concrete functional gaps the redesign surfaced, not more
visual polish.

- [x] **Backend (2026-08-13, single-session, trust-boundary code)**:
      `PUT /api/v1/auth/password` (session-only, requires current
      password re-confirmation per finding 5's re-auth-for-destructive-
      changes principle, rotates every other live session on success
      while leaving the caller's own session alone, per finding 6).
      `GET /api/v1/auth/session` (current session's username + expiry).
      `POST /api/v1/auth/sessions/revoke-others` (the standalone,
      user-triggered version of the same session-rotation mechanism).
      All three TDD'd (`internal/api/account.go`/`account_test.go`),
      `gofmt`/`go vet`/`golangci-lint`/`go test -race` clean,
      live-verified against the real running preview server (password
      changed and reverted back to the known preview credentials
      afterward so they kept working).
- [x] **Sidebar nav restructure (2026-08-13)**: the old single "API
      tokens" secondary-nav item became a real "Settings" group
      (`SidebarGroupLabel`) with four items: Account, Security, API
      tokens (unchanged), General. Three placeholder route files
      created (`routes/settings/{account,security,general}.tsx`) so
      the nav links type-check against real routes (TanStack Router's
      `Link` types are generated from files on disk) before each was
      built out as an independent unit of work.
- [x] **Account page (2026-08-13)**: profile card (read-only username,
      honest that Phase 1's `store.AdminUser` has no other field to
      edit yet) + a real change-password form (React Hook Form + Zod,
      `web/src/queries/account.ts`'s `useChangePassword`) against the
      new endpoint. Live-verified the full contract via curl against
      the real binary (wrong current password, too-short new password,
      success, old password rejected afterward, new password accepted)
      before this was merged.
- [x] **Security page (2026-08-13)**: current-session card (username +
      formatted expiry), a confirm-gated "sign out other sessions"
      action (`web/src/queries/security.ts`), and a static login-
      protection info panel. Live-verified with two real concurrent
      sessions: revoking others left the caller's own session working
      and the other one truly dead (401 on every route, not just the
      session endpoint).
- [x] **General page (2026-08-13)**: platform/brand info from the
      already-cached `/api/v1/brand` response, conditional Support/Docs
      links, and an honest "more settings coming later" card.
      Deliberately did NOT invent a new system-status backend endpoint
      (Docker health, secrets/webhook configured, disk usage, version)
      in this pass; confirmed via `grep` that no hardcoded brand-name
      string exists anywhere in the file, only `brand.Name`/
      `brand.ShortName` reads, per the brand indirection rule that no
      product name string appears anywhere in source code.
- [x] **Create App dialog (2026-08-13)**: the apps list page had no way
      to create an app at all, not even a button, despite
      `POST /api/v1/apps` always having existed and being fully tested
      on the backend. New `CreateAppDialog.tsx` (name/image/port form)
      wired into both the list header ("New app" button) and the
      empty-state CTA via one shared component with a `trigger` prop,
      so the empty state stopped being purely descriptive text. On
      success, navigates straight to the new app's detail page. Verified
      this one the most thoroughly of the four: real curl round-trip
      (201/400/409 all matched) plus an actual browser
      click-through (`agent-browser`: clicked "New app," filled the
      form, submitted, watched it navigate to the new app; separately
      confirmed the empty-state CTA opens the same dialog, and that a
      duplicate-name submission shows the real server error inline
      without closing the dialog or adding a phantom row).

## Phase 5: resource-creation and config depth (Coolify gap-closing)

Started 2026-08-13, prompted by the founder comparing the "new app" flow
directly against Coolify's and calling out how much is missing. **This is
not "chase Coolify's 280 templates"**, the non-goals list already rejects
that ("ten good ones beat 280 stale ones") and it stays rejected here.
The real gap is narrower and more concrete: Phase 1's own committed scope
("Managed Postgres and Redis as first-class resources," "env editor,
domain config") had backend support that was never fully wired to a UI,
or in the database case, had no API layer at all.

Verified state before this phase (direct file reads, not taken on faith
from an earlier summary, since one earlier research pass on this turned
out stale on point 7):

- Env vars and domains: already fully wired (`EnvEditor.tsx`,
  `DomainEditor.tsx`, `SecretsEditor.tsx`, live in `apps/$name.tsx`'s
  tabs). Not a gap.
- Health check probes and resource limits (memory/CPU): backend accepts
  both on `PUT /api/v1/apps/{name}` (`store.ServiceHealth`,
  `store.ServiceResources`), but `AppOverview.tsx` only ever displays
  them read-only, explicit in that file's own comment. **Real gap.**
- Databases: `store.DesiredDatabase` and the reconciler
  (`internal/reconcile/database`) existed; `internal/api` had zero
  routes for it. Nothing could create a database through the product at
  all, the single biggest gap and the one closest to Coolify's actual
  "new resource" flow (App vs Database as the first choice). **Real
  gap, now closed on the backend.**
- Postgres specifically still can't actually run (needs TASKS.md 1.7's
  envelope-encrypted secrets, not built): the new API and reconciler
  both handle this honestly (create succeeds, `GET
  /databases/{name}/status` shows a real `NotReady` condition) rather
  than hiding Postgres as an option or pretending it works.

- [x] **Database CRUD API (2026-08-13, backend, single-session TDD)**:
      `POST/GET /api/v1/databases`, `GET/DELETE /api/v1/databases/{name}`,
      `GET /api/v1/databases/{name}/status`. New `store.DeleteDesiredDatabase`
      (mirrors `DeleteDesiredService`). New `DatabaseStore` interface in
      `internal/api/router.go`, folded into `Store`. Full suite +
      `-race` + `golangci-lint` clean. No `PUT` (full update): engine/
      version/name aren't meant to change in place, an engine change is
      a migration not a config edit.
- [x] **Database creation frontend (2026-08-13)**: `queries/databases.ts`,
      `CreateDatabaseDialog.tsx` (mirrors `CreateAppDialog.tsx`),
      `routes/databases/{index,$name}.tsx`, `DatabaseRow.tsx`,
      `DeleteDatabaseDialog.tsx`, sidebar "Databases" nav item, a
      secondary "New database" button on the apps list header, status
      display on the detail page (`ConditionsPanel`, reused as-is)
      reading the new status endpoint. Built as an independent unit of
      work against the frozen contract above; reviewed file-by-file,
      independently re-verified (`tsc -b`, `eslint .`, `prettier --check`, `vite
      build`, plus a real curl round-trip against the running backend:
      create redis, create postgres, list, delete, all matched) before
      merging. Postgres is a selectable engine, not hidden or disabled:
      the status endpoint is where its real `NotReady` condition will
      show once the reconciler actually runs it.
- [x] **Health check + resource limits editors (2026-08-13)**:
      `HealthCheckEditor.tsx`, `ResourceLimitsEditor.tsx`, mirroring
      `DomainEditor.tsx`'s exact pattern (`useUpdateApp`, full-resource
      PUT, `keepDirtyValues` so the four editors on this page never
      stomp each other's unsaved edits), wired into `apps/$name.tsx` as
      two new tabs ("Health", "Resources"). Unit conversions documented
      in-code: CPU cores <-> nano_cpus (x1e9), seconds <-> duration
      nanoseconds (x1e9), a whole-number MiB field for memory chosen
      specifically to keep the byte round-trip lossless. Built as an
      independent unit of work, reviewed file-by-file, independently
      re-verified (`tsc --noEmit`, `eslint`, `prettier --check`) before
      merging.
      `AppOverview.tsx`'s read-only summary was left in place.

## Status notes

- 2026-08-13: three research streams run in parallel (Vercel deep
  dive, Railway deep dive, cross-cutting sidebar patterns). A separate,
  earlier pass of five visual-polish work streams (dark mode
  ThemeProvider, app list, app detail, observability screens,
  auth/settings screens) was already in flight when this redesign was
  scoped; that work is being allowed to land since it improves content
  inside the current shell and is expected to mostly carry forward into
  the new one, but no further content-polish work is being started
  until the Phase D wireframe is approved.
