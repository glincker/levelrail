# ADR 050: Release snapshots and environment bindings

Status: Proposed

Date: 2026-10-29

## Context

The question: is a release today an immutable snapshot (image digest,
config, env overrides) bound per environment, and is rollback a pointer
move? Verified on `release/v0.8.11` by reading the code, not only
`docs/promotions-and-releases.md` and `docs/pipelines.md`:

1. **There is no Release entity.** An app is one mutable
   `store.DesiredService` row. Apps in different environments are not
   linked: they share a `ProjectID` and an optional `EnvironmentID` tag
   (`internal/store/service.go:441`, `:455`), and promotion finds the
   target by that tag or by an explicit name
   (`internal/api/promote.go:197`, `findPromotionCandidates` at `:297`).
2. **Promote copies a tag string, not a digest.** `promoteImageRef`
   returns `source.Image` (`internal/api/promote_diff.go:17`, with a
   comment that a digest can replace it later). `handlePromoteApp`
   writes that string onto the target with `setDesiredImage`
   (`internal/api/deploys.go:489`), which overwrites `Image` in place and
   saves the row, then records a deploy attempt and nudges the
   reconciler (`promote.go:170` to `:186`). The pipeline `promote` step
   does the same through `pipelineActions.Promote`
   (`cmd/levelrail/pipelines.go:313`), reading `src.Image`.
3. **Approvals pin the env diff but not the content.** A protected
   promotion snapshots the env key diff at request time
   (`internal/api/promote_env_snapshot.go:17`) but stores the image as
   the source's tag. On approval, `setDesiredImage` applies that tag
   (`internal/api/deploy_approvals.go:446`). If the tag moved between
   request and approval, the approver ships content nobody reviewed.
4. **Deploy history is the closest thing to a release.** Each
   `deploy_attempts` row carries `Image`, `ImageDigest`, `DigestReason`,
   `RunningImageID` and a `DeployAttemptSnapshot` of config (env keys with
   literal values, port, domains, resources, health, replicas, strategy,
   volumes, labels) (`internal/store/deploy_attempt.go:56` to `:79`,
   `:133`, `:144`). Rollback pins keep the image ID from being garbage
   collected (`internal/store/rollback_pins.go`, migration
   `0461_rollback_pins.sql`).
5. **Rollback is a new deploy, not a pointer move.**
   `handleRollbackToDeploy` (`internal/api/deploy_rollback_to.go:63`)
   verifies the recorded digest against the local image or pin, rebuilds
   desired state with `deploy.RestoreSnapshot`
   (`internal/deploy/restore.go:17`) and calls `applyDeploy`, which writes
   a new desired state and a new attempt. It refuses when no digest was
   recorded (`deploy_rollback_to.go:37`). The pipeline `rollback` step
   uses `deploy.PreviousKnownGoodImage` by tag
   (`cmd/levelrail/pipelines.go:324`), with no digest check.
6. **Per-environment config is layered, not bound.** Organization,
   project and environment env vars (`internal/store/environment_env.go`,
   `project_env.go`, `organization_env.go`) plus branch and preview
   overrides merge into the app at deploy time. Promotion leaves the
   target's secrets, domains, ports, volumes and placement alone
   (`promote_diff.go:42`). Protection is a flag on the environment
   (`internal/store/environment.go:19`) and freezes are checked by
   `freezeGate` (`internal/api/deploy_safety.go:56`).

So the answer is no. A release is implied by a deploy attempt row of one
app. Promotion is a copy by tag between unrelated rows. Rollback rewrites
desired state from a snapshot. The parts exist (digest, config snapshot,
pins, approvals, freezes) but nothing ties "this exact content" to "this
environment" as a single value that can be moved.

## Decision (recommended)

Add two concepts and route promote and rollback through them. The
reconciler does not change: it keeps reading `DesiredService`.

**Release (immutable).** A row created from a succeeded deploy attempt
that has an `ImageDigest`:

- `id`, `project_id`, `source_app`, `source_deploy_attempt_id`
- `image_ref` (repo plus `@sha256:` digest, never a bare tag),
  `image_tag` (informational)
- `config` JSON: the environment neutral subset of
  `DeployAttemptSnapshot` (health, replicas, strategy, resources, labels,
  command). Never secret values, never domains, ports, volumes or
  placement.
- `commit_sha`, `created_at`, `created_by`
- No update path. Store code exposes insert and read only.

**EnvironmentBinding.** One row per target app:

- `app` (the target `DesiredService` name, primary key), `environment_id`
- `release_id` (nullable for legacy apps)
- `state`: `active` or `undeployed`
- `overrides` JSON: the per app fields a promotion never touches today,
  plus an optional per binding config override set. In the first phase
  this stays the target row's own fields, so nothing is duplicated.
- `previous_release_id`, `updated_at`, `updated_by`

**Binding history.** An append only table of `(app, release_id,
changed_at, changed_by, reason, approval_id)`, so rollback can name the
previous release without parsing deploy attempts.

**Verbs.**

- Promote: resolve the source's current release (or mint one from its
  latest succeeded attempt with a digest), then retarget the target's
  binding to that `release_id`.
- Rollback: retarget the binding to `previous_release_id` or an explicit
  release from history.
- Retarget is one function: write the binding and history in one
  transaction, then materialize `DesiredService` = release image by
  digest + release config + binding overrides + env layers, and record a
  deploy attempt carrying `release_id`. The reconciler converges as it
  does today.

Honest framing: this makes rollback a pointer move in the store and an
exact content match, not an instant swap on the node. Containers still
roll through the normal strategy.

## Rollout status

The flag stays `off` by default until the owner decides. `on` has been reviewed for undo (conditional, uncancelled, audited), approval pinning and private registry digest checks (an auth failure is inconclusive, never a missing manifest). Operator docs: `docs/promotions-and-releases.md`.

## Alternatives rejected

- **Keep deploy attempts as the release.** Attempts are per app and
  mutable in status, carry env literal values, and a promotion would still
  copy by tag. Pinning approvals would need the same digest work anyway.
- **Full environment snapshots (all apps of an environment as one
  release).** Correct for multi service stacks but needs an app identity
  across environments that does not exist (fact 1). Too large for one
  step. Left as a later ADR.
- **Switch only `promoteImageRef` to a digest.** Fixes fact 2, not facts
  3 and 5, and gives no history of what each environment ran.
- **Move bindings into the reconciler.** Breaks the rule that the
  reconciler reads one desired state shape, and widens the riskiest
  package for no gain.

## Schema impact

Forward only migrations after the current newest (`3053_*` on this
branch):

1. `releases` table, unique on `(source_deploy_attempt_id)`.
2. `environment_bindings` and `environment_binding_history`.
3. Nullable `release_id` on `deploy_attempts` and `deploy_approvals`.

No column is dropped. `DesiredService.Image` stays the field the
reconciler reads.

## Migration

- Backfill releases from succeeded attempts with a non empty
  `ImageDigest`, newest N per app (`APP_RELEASE_BACKFILL_PER_APP`).
- Backfill one binding per app that has an `EnvironmentID`, pointing at
  the release matching its current attempt. Apps with no digest get a
  binding with `release_id` null and are labeled legacy in the UI.
- Rollout behind `APP_RELEASE_BINDINGS` = `off` (default for one
  release), `shadow` (write bindings and history next to today's path,
  compare, change nothing), `on` (promote and rollback go through
  retarget).
- A direct edit of an app (env set, image deploy outside promote) marks
  the binding `drifted` instead of silently rewriting it. Drift is a
  derived flag, not a new state.

## API, CLI, MCP and dashboard surface

- API: `GET /api/v1/projects/{id}/releases`, `GET /api/v1/releases/{id}`,
  `GET /api/v1/environments/{id}/bindings`,
  `GET /api/v1/apps/{name}/binding` (with history). Existing
  `POST .../promote` and `POST .../deploys/{id}/rollback` keep their shape
  and add `release_id` to request (optional) and response.
- CLI: `apps releases list|show`, `apps promote --release ID`,
  `apps rollback --release ID`, `environments bindings ENV`.
- MCP: read tools `list_releases`, `get_release`, `get_app_binding`;
  `promote_app`, `rollback_app` and `plan_change` gain `release_id`.
- Dashboard: release column on the Deploys tab, a bindings view on the
  environment page, promote and rollback dialogs show the release id and
  digest.

## Approvals and freezes

- A pending approval stores `release_id`. Approval retargets to exactly
  that release, closing fact 3.
- `freezeGate` runs on every retarget, including rollback, same override
  rules as today.
- Protected environments still need `confirm` and a different approver.
  Agents still cannot approve (`docs/ai-control.md`).
- Spend guard and image signature checks run against the release digest.

## Security

- A release never holds secret values. Env literals stay in the binding
  side (target row and env layers), as promotion already behaves.
- Digest pinning removes tag mutation as a way to slip content past an
  approval.
- Release rows are immutable at the store layer. An audit entry is
  written on create and on every retarget.

## Failure modes

- **No digest resolvable** (registry inspect fails, local only image):
  with `APP_RELEASE_REQUIRE_DIGEST=on` promote refuses with a clear
  message; otherwise it falls back to today's tag path and records no
  release.
- **Pinned image collected on a node**: retarget succeeds in the store,
  the pull by digest fails, the attempt fails visibly as today.
  Retention comes from `APP_RELEASE_RETAIN_PER_APP`.
- **Binding and app row disagree** after a crash between the two writes:
  both are in one transaction; a startup check reports any mismatch.
- **Backfill picks the wrong release** for an app whose current image was
  set by tag after its last attempt: marked legacy, not guessed.

## What cannot be verified locally

- Digest resolution against private registries and multiple nodes.
- Image presence per node after GC with pins.
- Real rollout timing of a rollback on a multi replica app.
- Backfill correctness on a large, old database.

## Testing plan

- Store: release insert has no update path; binding retarget and history
  in one transaction; migration backfill on a fixture database.
- API: promote mints or reuses a release and retargets; approval applies
  the stored release even after the source tag moves; rollback retargets
  to the previous release; freeze blocks retarget.
- Shadow mode: bindings written, desired state unchanged.
- One real run on a test server: promote, rollback, approve, freeze.

## Slices and file ownership

1. Schema and store (`internal/store/release.go`,
   `internal/store/environment_binding.go`, two migrations). Sequential,
   schema work is never parallel.
2. Retarget function and shadow writes (`internal/deploy/retarget.go`,
   hook calls in `internal/api/promote.go`, `deploy_rollback_to.go`,
   `deploy_approvals.go`). Env: `APP_RELEASE_BINDINGS`,
   `APP_RELEASE_REQUIRE_DIGEST`.
3. Backfill (`internal/store/release_backfill.go`). Env:
   `APP_RELEASE_BACKFILL_PER_APP`, `APP_RELEASE_RETAIN_PER_APP`.
4. API and generated OpenAPI (`internal/api/releases.go`,
   `internal/apiclient/releases.go`).
5. CLI and MCP (`cmd/levelrail-cli/apps_releases.go`,
   `internal/mcptools/tools_releases.go`, `classes.go` entries).
6. Dashboard (`web/src/features/releases/*`, locale keys).
7. Pipeline steps use retarget (`cmd/levelrail/pipelines.go`).

Slices 4 to 6 ship together so the capability reaches API, CLI and
dashboard in one change.

## Out of scope

- Multi app or whole environment releases.
- Database schema or data rollback (pre deploy snapshots stay separate).
- Cross project promotion.
- Static site rollback (`rollbackStatic` keeps its path).
- Build provenance or signing changes.

## Consequences

- Promotion and rollback become exact content operations with a history
  per environment.
- Two more tables and one more concept for operators to learn.
- Direct edits on bound apps surface as drift, which may feel noisy at
  first.

## Open questions

- Should `overrides` move off the app row into the binding in a later
  phase, or stay on the app row for good?
- Is a release per source app enough, or do compose stacks need a group
  release before this ships?
- Default for `APP_RELEASE_REQUIRE_DIGEST` once `on` is the default mode.
- How long to keep `shadow` before switching the default to `on`.
