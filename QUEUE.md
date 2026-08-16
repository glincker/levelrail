# Queue

Lightweight backlog, list format only. No narrative, no "why" writeups,
no completed-work log entries, those still belong in `TASKS.md` (Phase
0-2 build log) and `TASKS-v2.md` (dashboard redesign log), or in the
PR/commit history itself. This file answers one question: what's next.
One line per item. Check items off in place rather than deleting them,
so the queue's own history stays visible.

Run `scripts/verify.sh` before moving anything out of "In progress."

**Standing rule (founder, 2026-08-15)**: don't run the full backend
test suite repeatedly. Scope every test run, dev-agent or PM-level, to
the touched packages only. Don't re-verify locally what CI already
confirmed. Don't loop-rerun CI waiting out a confirmed-unrelated flake,
main has no branch protection, merge past it after one investigation
pass. New tool for this: `scripts/affected-tests.sh [base-ref]`, diffs
against `base-ref` (default `origin/main`), maps changed files to
packages, then does a fixed-point BFS over the whole module's import
graph (source + test + external-test imports) to also include every
package that transitively depends on a changed one, and runs `go test
-race` on exactly that set. Use this instead of `go test ./... -race`
during iteration; `scripts/verify.sh`/pre-push still run the real full
suite, unchanged, that's still the CI-parity gate. Also added: pre-commit
now runs frontend eslint/prettier/tsc (scoped to staged files) and
`golangci-lint run --new-from-rev=HEAD` (scoped to changed lines), it
was previously Go-only.

**Live concurrency note (2026-08-15, do not "fix" this, just be aware)**:
`.claude/worktrees/agent-abf9c84341bbc2e53` (branch `feat/exec-endpoint`)
is mid an interactive rebase onto `origin/main` driven by someone/
something outside this session, currently paused on a real conflict in
`internal/api/router.go` for the base feature commit. The security fix
commit (`0e2019e`, re-gates exec to `AbilityRoot`, see below) is queued
next in that rebase (`git reflog` in that worktree confirms it's intact,
not lost) and should reapply once the current conflict is resolved and
`git rebase --continue` runs. Confirmed via `origin/main`: the exec
route isn't merged yet, so there's no live exposure right now. Whoever
finishes that rebase: please confirm `router.go`'s exec route still says
`AbilityRoot`, not `AbilityDeploy`, after it lands, before merging.
Left that worktree alone rather than touching it mid-operation.

## Must-have gap scan (2026-08-15/16, founder-directed priority)

Founder call: recent rounds drifted toward secondary/edge polish (e.g.
database backup-schedule UI, parked below) instead of the primary
capabilities that actually justify this product as something a real
Coolify/Dokploy user migrates to. `[Architect]` gap-scan against
`docs-local/research/prior-art-*.md` plus a fresh code check, cross-
referenced against the rest of this file so nothing already tracked
below is duplicated. Ranked most load-bearing first. **These take
priority over everything else in this file, including "In progress"
items that aren't already mid-flight with a dispatched agent.**

**Round 1 dispatched (2026-08-15), design-only per pm-orchestration.md's
"design changes before building" gate: every item below is a new
feature/component, none of it is a bugfix or wiring-existing-route work,
so no code gets written until a design is presented and approved.**
Founder approved all three real recommendations (2026-08-16); round 2
implementation dispatch follows.

**IMPORTANT correction discovered during round 2 prep (2026-08-16):**
this local checkout's `main` had drifted 17 commits behind
`origin/main` (PRs #28, #30, #33, #34, #36, #37 all already merged
there) while also carrying 6 unpushed direct-to-main commits of its
own (a process violation — CLAUDE.md section 7 requires branch+PR for
source changes; the QUEUE.md/TASKS.md bookkeeping commits themselves
match this repo's own established direct-commit convention per real
history, so only the one source fix was affected). Resolved:
`docs/queue-restore-and-repo-presence-work` branch preserves the raw
commits, `fix/crashloop-alert-log-link` branch holds the alert-link
fix properly (not merged, no PR opened, per standing instruction),
`main` reset clean to `origin/main`, this file rebuilt on top of that.
**A `docs/repo-presence-and-docs` design spec + implementation plan
from earlier in this session also surfaced on that backup branch,
disconnected from the (currently empty) `docs/repo-presence-and-docs`
worktree it was meant for — flagging for founder follow-up, not
resurrected here since its approval status from earlier in this
session isn't independently confirmable.**

**Also discovered during the same drift-check: several other worktrees
have real, substantial, uncommitted or unmerged work already in
flight that overlaps this round:**
- `.claude/worktrees/agent-multiuser-oauth` (branch
  `feat/multiuser-oauth`): 21 files of **uncommitted** work — deletes
  the single-row `internal/store/admin.go`/`admin_test.go`, adds
  `internal/api/oauth.go`/`oauth_client.go`/`oauth_settings.go`/
  `oauth_state.go`/`users.go`, migration `0030_users.sql`. **This is
  very likely the team/RBAC item below, already substantially built.**
  Not yet reviewed for completeness/correctness. Team/RBAC below is
  updated to point here instead of commissioning fresh design work.
- `.claude/worktrees/agent-email-settings` (branch
  `feat/email-settings-password-reset`): 26 files uncommitted,
  **also modifies `internal/store/admin.go`** (adds email +
  password-reset fields) — real collision risk against
  multiuser-oauth's deletion of that same file. Needs sequencing:
  whichever lands second must rebase onto the first's actual shape,
  not just re-apply blind.
- `.claude/worktrees/agent-a7230f2a4ba9b20e1` (branch
  `feat/dns-guided-domain-setup`): 7 files uncommitted, real DNS-check
  work for domain setup guidance. Unrelated to this round's items, not
  investigated further here.
- `.claude/worktrees/agent-cli-expansion` (branch `feat/cli-expansion`,
  commit `9ca00eb`): **already committed** (not just uncommitted),
  "CLI exec, auth login/whoami, tokens, domains, backups commands" —
  not yet merged/PR'd. Worth checking before dispatching the
  migration-tool CLI work below, since both touch `cmd/levelrail-cli`.

None of the above were touched (no commits amended, nothing merged,
nothing discarded) — flagged for review/sequencing, not acted on
blindly.

- [ ] **Multi-service apps aren't actually supported end-to-end.**
      `internal/spec/spec.go`'s `Spec.Services` is a map (per CLAUDE.md
      4.9's own app.yaml example), but `internal/deploy/deploy.go:242`
      only ever passes a single `spec.Service` through, and
      `internal/webhook/webhook.go`'s own doc comment says outright:
      "one secret, one repo, one branch, one spec.Service." Every app is
      exactly one container today. Blocks the single most common real
      migration case (web + worker/cron sidecar in one app), which both
      Coolify and Dokploy support natively. This is Phase 1 scope that
      never actually finished, not Phase 4 polish. Large: touches
      `internal/store` schema (one `DesiredService` row per app today),
      `internal/deploy`, application reconciler convergence.
      **Reconciler-core + db-schema, single session only, do not
      parallelize** (CLAUDE.md section 8).
      **Design proposal landed (round 1, 2026-08-16), founder-approved:**
      new `apps` table (`id`/`name`/`project_id`, `ON DELETE CASCADE`)
      layered on top of the existing `desired_services`, plus an
      `app_id` FK. Service names stay flat/globally-unique
      (`appName-serviceKey` convention, founder-approved default), so
      the existing reconciler fan-out (`dynamicSource` in
      `cmd/levelrail/main.go`, already builds one controller per store
      row) needs **zero changes** — the app concept is a read-side
      rollup only. `internal/deploy` fans out build+deploy per service
      independently with partial-failure semantics (founder-approved
      default: one broken service doesn't block the rest). Backfill
      migration makes every existing app a transparent 1-service app,
      zero upgrade behavior change. ~3-4 focused control-plane days;
      schema/deploy/webhook reshape must stay one session (tightly
      coupled), API/frontend parallelizable after. **Next migration
      number must be re-checked at dispatch time** — `0027`-`0029` are
      already claimed across multiple in-flight branches (see drift
      note above), do not assume `0027_apps.sql` is still free.
- [ ] **No team/multi-user access or RBAC.** `internal/store/admin.go`'s
      `AdminUser` is a hard single row (`CreateAdminUser` errors
      `ErrAdminAlreadyExists` on a second insert). The ability system
      (`internal/api/abilities.go`) scopes API *tokens*, not human
      users, and says so in its own comment. No audit/activity log
      exists anywhere either (empty grep), same root cause. Anyone
      migrating as a team, not a solo operator, can't use this without
      sharing one password and zero per-actor accountability. Phase 4,
      large, single-session (db schema + secrets-adjacent auth).
      **Correction (2026-08-16): do not commission fresh design/build.**
      `.claude/worktrees/agent-multiuser-oauth` already has 21 files of
      substantial uncommitted work covering exactly this (see drift
      note above) — deletes single-admin `admin.go`, adds
      `oauth.go`/`users.go`/migration `0030_users.sql`. Next action is
      review-for-completeness and conflict resolution against
      `agent-email-settings`'s overlapping `admin.go` changes, not new
      work.
- [ ] **No install/upgrade path for the product itself.** `scripts/`
      only has `install-hooks.sh` (contributor git hooks); no
      `install.sh`, no systemd unit found anywhere. There is no
      one-command way to stand this up on a fresh box, which is the
      literal first step of switching off Coolify/Dokploy (both ship a
      curl installer). Distinct from the parked "Coolify self-upgrade"
      item below, which is about updating an already-installed binary,
      not the initial install. Phase 4, medium, parallelizable (ops/
      packaging, no reconciler/agent/schema touch) but blocked on the
      binary-vs-container packaging decision (CLAUDE.md section 9).
      **Research landed (round 1, 2026-08-16), founder-approved:** one
      binary (`levelrail`) for v1 single-node install —
      `internal/agent/transport.go` already has a working in-process
      `Local` transport, so the control plane is already self-sufficient
      without `levelrail-agent` for the common single-node case; the
      agent binary would be fetched at Phase 3 node-enrollment time
      instead of bundled upfront. Install mechanism: curl-pipe-sh +
      systemd unit, not a Docker-wrapped container (rejected:
      reintroduces a "who-manages-the-manager" problem, cited Dokku
      issue #1076) and not a distro package for v1 (real value, but
      real infra cost, Phase 5 candidate). v1 minimum (install.sh +
      systemd unit + release-build GitHub Actions job): ~1-2 days.
      Ready to dispatch.
- [x] ~~**No GitHub App / OAuth connect flow.**~~ **Corrected (round 1,
      2026-08-16): this was stale. The feature already exists as code**
      in **open, unmerged PR #35** (`feat/github-app-integration`) —
      GitHub App manifest self-registration (per-instance, not a shared
      maintainer-owned App — deliberately, so no single credential
      touches every self-hoster's org), org/repo/branch picker, and
      installation-token minting via `internal/secrets`, reusing the
      exact ability-tiering pattern (`AbilityRoot`/`AbilityReadSensitive`)
      the rest of the API already uses. **PR #35 is unreviewed**, and
      currently `mergeStateStatus: DIRTY` / `CONFLICTING` (confirmed live
      via `gh pr view 35`, re-confirmed 2026-08-16 after the drift fix
      above) — its migration `0029_github_app_connection.sql` collides
      with PR #36's already-merged `0029_service_git_sources.sql`, plus
      a failing SonarCloud check. Real remaining gaps once merged: (1)
      the picker only feeds ad-hoc app creation, not PR #36's persisted
      git-source table — needs a small additive `auth_source` column;
      (2) no App-level webhook route exists yet for auto-deploy-on-push
      (the manifest currently requests zero webhook events) — this is
      the one piece with genuine remaining design work. **Next action
      is: review + rebase + renumber migration + merge PR #35, not
      commissioning new work.** Ready to dispatch.
- [ ] **No preview environments per PR.** Zero grep hits for "preview"
      anywhere in `internal/` or `web/src`. Explicit Phase 4 scope,
      supported by both Coolify and Dokploy, so a concrete parity gap
      for an evaluator. Depends on multi-service (#1 above) landing
      first if previews should mirror production topology. Large; the
      webhook/PR-event lifecycle piece isn't trivially parallel with
      reconciler work, though a frozen-API frontend slice would be once
      it exists. Not dispatched this round, unchanged.
- [ ] **No migration tooling from Coolify/Dokploy.** CLAUDE.md Phase 5
      only commits to a written comparison/migration guide, not an
      importer. No trace of any Coolify/Dokploy config parser anywhere.
      For someone with 10+ existing apps, hand-recreating every
      app.yaml/env var/domain by hand is real switching-cost friction
      that decides whether a migration actually completes. Medium-large,
      Phase 4/5, parallelizable (one-way conversion tool, no reconciler/
      schema/secrets touch).
      **Design proposal landed (round 1, 2026-08-16), founder-approved:**
      Coolify as the v1 target (documented OpenAPI REST API vs
      Dokploy's undocumented tRPC/Drizzle internals), delivered as a
      CLI subcommand (`levelrail-cli migrate coolify`), calling the
      existing `POST /api/v1/apps` or writing `app.yaml` files — zero
      touch to `internal/store`/reconciler/secrets, pure client of
      `internal/spec`'s existing types. Secret values default to
      names-only export (mirrors Levelrail's own `read` vs
      `read:sensitive` ability split). ~3-5 days Coolify-only, +2-3 days
      Dokploy fast-follow. Multi-service/Compose source apps bucketed
      as manual-review, deferred to item #1 above. **Check
      `agent-cli-expansion`'s already-committed CLI work (`9ca00eb`,
      not yet merged) before dispatching — both touch
      `cmd/levelrail-cli`, avoid a collision.**

Two items the last dashboard-gap audit flagged are now confirmed
resolved, not re-listed: no login screen, no first-run flow
(`web/src/routes/login.tsx` has both login and register, backed by
`CreateAdminUser`).

## Needs PM personally (not yet dispatched)

- [x] Private git repo support: landed as part of the persisted
      per-app git source feature (PR #36, see In progress/Done below).
      PAT auth through `internal/secrets`, not SSH keys (simpler,
      correct HTTPS-with-token auth via `go-git`'s `BasicAuth`, no
      SSH-agent/known_hosts wiring needed). `internal/api/builds.go`'s
      manual "Dockerfile from git" flow still has no Auth field (still
      asks for a public repo URL fresh each time), that specific path
      not extended, only the new persisted git-source connect flow.
- [ ] Explicit "Stop" action for apps, distinct from Delete/Restart. No
      Stopped/Suspended desired-state concept exists in
      `internal/store`/`internal/reconcile/application`. Reconciler-core.
- [ ] Deploy-fixtures token seeding bug (`internal/api/devfixtures.go`):
      `GetAPITokenByHash` returns not-found for a fixture-seeded token
      despite a matching hash; `MaybeBootstrapDevAdmin` silently
      overwrites an operator-set admin account, contradicting `main.go`'s
      own doc comment. Pre-existing, real, reproducible.
- [x] `useDeployLogStream.ts`'s stuck "Connecting..." label: re-checked,
      already fixed. Both `internal/api/deploy_attempts.go` and
      `live_logs.go` write a harmless SSE comment line immediately on
      connect, guaranteeing `EventSource.onopen` fires even for a
      zero-log-line finished attempt. Stale backlog item, removed.

## In progress / awaiting review or merge decision

- [ ] Founder-requested, out of band: unified env var UI (list view,
      developer/raw-text view, secure lock for secret-flagged keys),
      Coolify-referenced. Real gap confirmed before dispatch:
      `store.DesiredService.SecretEnv` (key names) exists but was never
      exposed via `GET /apps/{name}` at all, and the frontend has two
      disconnected components (`EnvEditor.tsx` plain-only,
      `SecretsEditor.tsx` write-only) instead of one list. Dev agent
      dispatched (worktree isolation), briefed on the load-bearing
      constraint the exec-endpoint security fix above just surfaced:
      this codebase never decrypts a secret back into an API response,
      so "lock" for secret keys means permanently non-revealable by
      construction. Not yet reviewed.
- [ ] **QA CONFIRMED, ready for merge decision.** Founder-requested, out
      of band: smart "restart required" toast after saving env/health/
      resource changes. Branch `worktree-agent-acb4ef384b9d9e346`, commit
      `713db21`, frontend-only (`useRestartRequiredToast.ts`,
      `HealthCheckEditor.tsx`, `ResourceLimitsEditor.tsx`). QA
      independently reproduced the underlying reconciler gap with a real
      Docker test of its own (not just re-reading the PM's claim): a
      256Mi resource-limit save left the running container at its old
      128Mi through a no-op reconcile, only applied after a restart-nonce
      bump. Also independently verified the toast API claims against
      Base UI's real `.d.ts`, confirmed no scope creep, confirmed the
      Coolify research citations are real. No blockers, no follow-ups.
      Env/Secrets wiring (below) is the same pattern once the unified-
      env-view agent lands. Underlying gap: `ContainerName` is keyed
      only on service/image/restartNonce (`controller.go:640`); once a
      container by that name exists, `Reconcile` returns `AlreadyRunning`
      immediately and never compares the running container's actual
      env/health/resources against desired state. Domain/project changes
      are NOT affected (ingress reconciler applies independently every
      tick), no nudge needed there. Scoped to `HealthCheckEditor.tsx`/
      `ResourceLimitsEditor.tsx` only, explicitly told NOT to touch
      `EnvEditor.tsx`/`SecretsEditor.tsx`
      since the unified-env-view agent above is concurrently editing
      those same files. Env/Secrets wiring is a fast follow-up once
      both land. Not yet reviewed.
- [ ] Round 17 #2, one-shot container `exec`: built + security-fixed
      (re-gated `AbilityRoot`, was wrongly `AbilityDeploy` which let a
      plain deploy token exfiltrate container secrets via `env`), full
      re-verification green. Branch `feat/exec-endpoint`, commit
      `0e2019e`. **Ready for a founder merge decision**, not merged
      (that session's own "don't auto-raise PRs" rule). Known follow-ups,
      not blockers: no audit trail for a successful exec, remote-node
      stub returns a generic 500.
- [ ] Round 17 #5, per-node disk space: built, QA found the feature has
      zero end-to-end observable value in single-node mode (`GET /nodes`
      returns `[]` locally, so the new metric can never be queried).
      Follow-up dispatched: surface local-node disk space via
      `GET /system/status` instead (General settings page). Not yet
      reviewed.
- [ ] Round 17 #4, database resource limits (CPU/memory): **duplicate
      work discovered.** This session independently dispatched and
      QA'd its own implementation (typed columns, PUT+DELETE), then
      found a concurrent session had already opened **PR #28** with a
      materially different design (JSON blob column matching
      `desired_services.resources`'s own convention, PUT only, its own
      dedicated route + sidebar entry) before this session's dev agent
      finished. This session's duplicate was discarded (never merged,
      never even committed) rather than compete with an already-open
      PR from another session. PR #28 is that session's own work to
      land; not touched here. Lesson: worth checking `gh pr list --state
      open` before dispatching, not just local worktrees, since another
      session's work can be miles ahead in the PR pipeline with no
      local worktree trace visible to `git worktree list`.
- [ ] Round 16 #7, backup retention/pruning: last known claimed by a
      concurrent session, no active worktree found on re-check
      (2026-08-15 late). Re-verify claim before dispatching.
- [ ] Round 16 #8, Redis backup restore (currently honest
      `ErrRedisRestoreNotSupported`): same, last known claimed, re-verify
      before dispatching.
- [ ] Round 16 #10, shared/group environment variables: same, last known
      claimed, secrets-adjacent, needs sequencing regardless.
- [ ] `backup-core` (connect bucket / backup / restore CRUD, pre-dates
      scheduled backups PR #20 which builds on the same tables): was
      "ready for a PR whenever founder wants one," never opened. Re-check
      whether it's now superseded/subsumed by PR #20 before treating as
      still-open.
- [ ] Caddy/certmagic upstream data race (`caddytls.(*TLS).CaddyModule()`
      value-receiver race): root cause fully identified, no upstream fix
      exists yet, recommendation is "do nothing for now." Full trail:
      `docs-local/research/caddy-certmagic-race-decision.md`. Holding for
      founder sign-off before any fallback (a scoped `t.Skip`) is built.
- [ ] Coolify self-upgrade (control-plane binary self-update, progress
      UI): researched, medium-large effort, gated on a packaging decision
      (binary+systemd vs self-hosted-container, CLAUDE.md section 9 has
      this as an open question). Parked, needs that decision first.
- [ ] Deploy-attempt ID / build-log persistence design note (superseded,
      already implemented and merged, kept only as a pointer):
      `docs-local/research/deploy-attempt-id-and-log-persistence.md`.

## Backlog (lower priority / needs a confirmation pass first)

- [ ] Round 17 #10, non-GitHub webhook sources (GitLab/Bitbucket): lower
      confidence, needs a confirmation pass against Coolify's actual
      support before dispatch.
- [ ] Docker Compose as a deploy target: large, needs its own reconciler
      controller.
- [ ] Curated ~15-entry template catalog: large, parallelizable
      per-template once Compose lands.
- [ ] `internal/docker`'s `TestClient_ListByPrefix_Stop_Remove_Live`
      CI-only flake pattern (a container's `Running` state can lag
      immediately after `Stop()` on some Docker daemons) has recurred in
      new forms (`TestClient_Prune_Live_RemovesOnlyGenuineGarbage` hit
      the same class 2026-08-15). Worth a real poll/wait fix if it keeps
      recurring, not chased per-occurrence.
- [ ] Frontend main bundle chunk sits right at the 500kB vite warning
      threshold. Not urgent (~0.2% over, 146kB gzipped last checked), no
      bundle-visualizer-driven real fix attempted yet.

## Done (PR reference only, see the PR/commit itself for detail)

- [x] PR #36: persisted per-app git source + multi-app GitHub webhook
      (per-app HMAC secret, PAT auth via `internal/secrets`).
- [x] PR #33: custom Docker labels escape hatch (`spec.Service.Labels`).
- [x] PR #32: backup file download to browser.
- [x] PR #31: database publicly-accessible toggle + host port (Redis
      no-auth-aware security fix applied post-QA).
- [x] PR #29: App Clone.
- [x] PR #28 (concurrent session): database resource limits (CPU/
      memory). This session independently built a duplicate before
      discovering PR #28 already existed; duplicate discarded, never
      merged.
- [x] PR #27: warn on missing secrets master key before database
      creation.
- [x] PR #26: App Overview hero+checklist, batched Apps list status dot.
- [x] PR #23: dedupe `internal/store/database.go` scan helper +
      `internal/deploy` finishDeploy tail.
- [x] PR #22: dedupe `NodeMetricsDashboard`/`MetricsDashboard`
      (`TimeRangeControls`, `useMergedChartQuery`).
- [x] PR #20: scheduled/cron backups (`internal/cronexpr`,
      `internal/backup.Scheduler`).
- [x] PR #19: deploy-outcome notifications (Slack/Discord/Telegram/
      email/generic webhook).
- [x] PR #25 (concurrent session): app-detail-polish (deploy-attempt
      commit-source tracking, `ConvergenceIndicator`, `BuildLogHints`).
- [x] PR #24 (concurrent session): centralized Domains page + ACME
      settings.
- [x] PR #21 (concurrent session): mesh DNS wired into container
      creation.
- [x] Creation wizard + brand icons (`thesvg.org` via `@thesvg/react`) +
      dynamic/contextual sidebar, full
      `docs/superpowers/specs/2026-08-14-creation-wizard-and-sidebar-design.md`
      spec landed across 3 rounds (concurrent session).
- [x] MySQL as a third database engine, dynamic engine registry
      (concurrent session).
- [x] Databases detail route: dynamic/contextual sidebar, mirrors Apps'
      pattern (concurrent session).
- [x] PR #18: lightweight non-RBAC Projects grouping.
- [x] PR #17: Docker disk/image/volume pruning.
- [x] PR #16: node-level metrics dashboard.
- [x] PR #14: CLI `rollback`/`restart` subcommands.
- [x] PR #13: accessibility audit + fix round (`DomainEditor`/`EnvEditor`
      row-input `aria-label`s).
- [x] PR #12: live app log streaming (SSE, `LogBroadcaster`,
      `LogTerminal`).
- [x] PR #11: real multi-marker deploy history on the Metrics chart.
- [x] PR #9: bundle-size fix (`ChangePasswordCard.tsx` extraction).
- [x] PR #8: fixed stuck "Connecting..." SSE label (Vite dev-proxy
      artifact).
- [x] PR #6: fixed `internal/docker` CI-only Stop/Remove flake (poll
      instead of asserting immediately).
- [x] Deploy history, rollback, real build-log persistence
      (`deploy_attempts`/`deploy_logs` tables, `internal/deploylog`).
- [x] Restart action for apps (`RestartNonce`, folded into
      `ContainerName`'s hash).
- [x] Deploy strategy/replicas surfaced on the app screen
      (`DeployStrategyEditor.tsx`).
- [x] Railpack auto-detection (Node.js/Go providers,
      `github.com/railwayapp/railpack`).
- [x] Deploy strategies backend (`rolling`/`recreate`/`blue-green`,
      migration `0017`).
- [x] Manual build trigger from the dashboard (`POST /apps/{name}/builds`).
- [x] `build.type: static` support (own `StaticSite` table, no frontend
      surface yet).
- [x] TLS certificate renewal visibility (`GET /api/v1/certificates`).
- [x] Base UI native-button accessibility fixes (`nativeButton={false}`
      on 2 sites).
- [x] Node health/cordon/drain/workload-capability UI.
- [x] "Saved." success-message consolidation.
- [x] Domain-conflict real 409 (was a generic 500).
- [x] Previously-built image tag dropdown for `DeployTriggerForm`.
- [x] App deletion dialog + node placement display.
- [x] `backup-core`: connect S3-compatible bucket, backup, history,
      restore (Postgres/MySQL live-verified, Redis honestly unsupported).
      Never PR'd into main directly; superseded/absorbed by later work,
      see the re-check item above.
- [x] Badge component `success`/`warning` variants (replaced 6 files'
      hand-rolled status colors).
- [x] `.env` block bulk-import in `EnvEditor.tsx`.
- [x] Docker daemon connectivity on `GET /system/status`.
- [x] `resolveEnv` `SecretEnv`-over-`Env` precedence, documented + tested.
- [x] Postgres reconciler wired to `internal/secrets.Manager`.
- [x] Phase 3: multi-node build routing, distributed cert storage,
      WireGuard mesh, node health/cordon/drain, org-wide Phosphor icon
      migration (ADR 014).
- [x] e2e coverage: auth lifecycle, rollback convergence, node-placement
      transport, Redis reconciliation, non-default port routing, env var
      injection, container-to-HTTP metrics.
- [x] Toast notifications for database/node deletion.
- [x] Databases list table virtualization (confirmed already done).
