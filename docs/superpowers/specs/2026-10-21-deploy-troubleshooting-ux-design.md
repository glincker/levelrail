# Deploy troubleshooting UX (v0.8.1)

Status: approved by owner in chat, 2026-10-21. Base: `release/v0.8.1`.

## Goal

A newcomer who deploys an app and watches it fail can find the cause, fix the
right setting, redeploy, and copy the evidence, without leaving the deploy
pages or searching online. Log streaming must degrade visibly, never silently.

## Scope

Three pieces, shipped on one branch.

### A. Deploy detail page (`routes/apps/$name/deploys/$deployId/logs.tsx`)

1. **Fix and redeploy row** under `DeployFailureSummaryCard`. Links chosen by
   failing stage and matched cause (`lib/buildLogHints`):
   - env var causes: `/apps/$name/environment`
   - build path or build type causes: `/apps/$name/source`
   - port, health check, strategy causes: `/apps/$name/deploy-settings`
   After saving, the deploy page shows a "Redeploy with your changes" banner.
   Pure mapping lives in `lib/deployFixLinks.ts` with unit tests.
2. **Retry split button**: Retry (same image), Rebuild from source, Rebuild
   without cache. "Without cache" needs the backend to forward `no_cache` to
   the build request (engine already supports `NoCache`).
3. **Copy as report**: Markdown with app, deploy id, stage, error, likely cause,
   last 100 log lines, config summary. Secrets redacted. Pure builder in
   `lib/deployReport.ts` with unit tests.
4. **Streaming resilience UI**: banner when the stream drops (transport,
   next retry time, "switch to polling", "download full log"). Terminal gets
   copy line, copy all, jump to first error.
5. **Cache visibility**: build tab header shows "Cache: N of M steps reused"
   derived from BuildKit `CACHED` lines; "Cache: unknown" when absent.
   Pure parser in `lib/buildCacheStats.ts` with unit tests.

### B. Deploy list (`routes/apps/$name/deploys/index.tsx`)

- One primary CTA (Deploy now).
- Latest attempt pinned with status, stage, duration; failed rows link to
  the failing stage log.
- Sleep, autoscale and budget cards leave this page (they already have homes
  elsewhere or move to the app overview; verify before removing).
- The collapsed "Deploy settings" section becomes one link to
  `/deploy-settings`, one home for settings.

### C. Deploy settings (`routes/apps/$name/deploy-settings.tsx`)

- Group the cards under Build, Run, Safety, Automation with a sticky section
  nav.
- Each card gets a one-line "What this does" and a recommended default shown
  as a filled value or a "Use recommended" button. No empty inputs that need
  outside research.
- Top strip "How your deploy works" (Build, Push, Rollout, Health check)
  linking to the relevant card.

## Non-goals

No change to the reconciler, deploy pipeline stages or log storage. No new
dependencies. No `package.json` edits.

## Rules

Tailwind only, Phosphor icons only, no `any`, no `console.log`, no inline
styles, no em or en dashes, comments 1 to 3 lines about why. Thresholds from
env. All user-facing strings go through the existing i18n namespaces.

## Verification

Scoped vitest for new `lib/` modules and touched components, `tsc` and eslint
on touched files, `go build` and the scoped Go test for the API change. Then
`baseline-ui`, `fixing-accessibility`, `fixing-motion-performance`.

## Addendum: competitor research (2026-10-21)

Added after comparing Coolify, Dokploy, CapRover, Render, Railway, Vercel.
- Show the detected builder ("Detected: Node, Railpack") with override on the deploy page and in Build settings.
- Rollback clarity: say which image is restored and that env vars, volumes and migrations are not reverted.
- Terminal: log search, error line highlight, jump to first error; keep it fast on very large logs (other platforms freeze the browser).
- The copy-as-report must redact secrets (other platforms leak them in build errors).
