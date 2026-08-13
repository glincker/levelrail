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

- Research before build. No implementation agent gets dispatched for the
  new shell until a wireframe has been reviewed and approved.
- Real evidence, not vibes: research docs cite actual colors, spacing,
  copy, or structure from what was actually found, not paraphrased
  impressions. Say explicitly when something couldn't be verified.
- No em dashes (—) or en dashes (–) anywhere, same as every other file
  in this repo.
- Parallel agents get an explicit, non-overlapping file list per CLAUDE.md
  8's "give each agent an explicit exit criterion and a list of files it
  may touch" rule. Collisions get resolved by re-scoping the task list,
  not by hoping two agents don't touch the same line.
- Scope check before adding anything here: Levelrail is Phase 1, single
  admin, no teams, no RBAC yet (CLAUDE.md 6). A pattern borrowed from
  Vercel/Railway/Linear that assumes multi-tenant/team features doesn't
  belong on this list yet just because the source product has it well
  designed; note it as a Phase 4 candidate instead if it's genuinely
  gated on that scope.

## Phase R: Research (parallel agents, no code changes)

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
      implementation agent touches `web/src` for the shell itself.
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
      issue (independently confirmed by three separate polish agents
      earlier in this session, not caused by this change), so this
      was not visually screenshotted, flagged explicitly rather than
      claimed.
- [ ] Merge the in-flight dark-mode `ThemeProvider`/`ThemeToggle` pass
      (dispatched before this shell rewrite) into the new shell: that
      agent's own `__root.tsx` diff will conflict with the rewrite
      above, so only its new component files
      (`ThemeProvider.tsx`/`ThemeToggle.tsx`) get pulled in, wired into
      `AppSidebar`'s header or footer directly rather than reapplying
      its top-nav-shaped diff.
- [ ] Reassess the KPI-strip/`SectionCards` pattern for an apps
      overview once the shell has real content flowing through it
      (deferred, not blocking: the current `/apps` route already has
      its own list-page polish from the earlier pass).

## Phase 2: Screen migration into the new shell

Not detailed yet, same reasoning as Phase 1. Expect this to reuse the
content-level work already landing from the parallel visual-polish pass
on `apps/index.tsx`, `apps/$name.tsx` and its panels, the observability
components, and the auth/settings screens (dispatched 2026-08-13, before
this redesign was scoped): that work improved the content inside the
current shell, most of it should still be valid content once re-wrapped
in the new shell, but expect real adjustment work, not a pure copy-paste.

## Phase 3: Rich interactions (stretch, prioritize from research findings)

Not detailed yet. Candidates already flagged by the research asks:
command palette (cmd+k), richer empty/loading states, keyboard
navigation. Whether any of these make the cut depends on the "lowest
effort, highest value" synthesis the cross-cutting research doc is
asked to produce, not assumed here.

## Status notes

- 2026-08-13: three research agents dispatched in parallel (Vercel deep
  dive, Railway deep dive, cross-cutting sidebar patterns). A separate,
  earlier-dispatched pass of five visual-polish agents (dark mode
  ThemeProvider, app list, app detail, observability screens,
  auth/settings screens) was already in flight when this redesign was
  scoped; that work is being allowed to land since it improves content
  inside the current shell and is expected to mostly carry forward into
  the new one, but no further content-polish agents are being dispatched
  until the Phase D wireframe is approved.
