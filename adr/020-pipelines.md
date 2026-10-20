# ADR 020: CI/CD pipelines as an orchestration layer over deploy

Status: Accepted

Date: 2026-09-24

## Context

Builds and deploys were triggered directly: a push built an image and
pointed the app at it. There was no way to say "run the tests first",
"wait for approval", "build once, then deploy to staging and production",
or "test on three runtimes". Operators either bolted an external CI on
top or ran a chain of CLI calls from a script.

## Decision

Add `internal/pipeline`: a user-defined pipeline (YAML: triggers, stages,
jobs with a `needs` graph, matrix, conditions, steps) executed by the
control plane.

**Pipelines orchestrate; they never reconcile.** The built-in
`build`, `deploy`, `promote`, and `rollback` steps call the same
functions the API uses (`internal/deploy`'s builder and
`TriggerImageDeploy`), so the reconciler stays the only thing that
converges containers. No AI is in the path.

**Level-triggered, resumable executor.** Runs, jobs, and steps are rows
with a status and a reason. Each pass re-derives the next action from the
database; a job recorded as running with no live goroutine is resumed
(stale containers removed, finished steps skipped). Concurrency groups,
cancellation, approvals, and timeouts are all state in those rows, not
in-memory flags, so a restart loses nothing.

**Jobs run in one container per job, steps as execs.** A job container
(image chosen by the user, workspace volume at `/workspace`) is created on
the chosen node through the Engine API and steps run inside it with
`ExecWithInput`, script and secrets on stdin so they never appear in
argv. This works identically on the local daemon and on remote agents,
which expose the same runtime interface. Steps that need only the control
plane start no container.

**JSON Schema plus semantic validation** at parse and at save: the schema
catches shape errors with line numbers for the editor, the semantic pass
checks the `needs` graph, expressions, step arguments, and cron
expressions.

## Rejected alternatives

- **Embed Tekton or Argo Workflows.** Both are Kubernetes controllers.
  Embedding either brings the whole Kubernetes surface back, which the
  platform exists to avoid (ADR 002), and neither runs against a bare
  Docker daemon.
- **Run `act` (a GitHub Actions emulator).** It ties the file format to
  GitHub's workflow semantics and marketplace actions, shells out to the
  Docker CLI (against the no-CLI rule), and cannot express approvals,
  deploys, or rollbacks as first-class steps.
- **Delegate to GitHub Actions only.** The bundled Action already deploys
  an image from a workflow, and remains supported. It cannot gate on an
  approval inside the platform, follows only GitHub, and puts build and
  deploy history in two places. Pipelines cover the self-hosted case and
  work with any git provider.

## Consequences

- Pipeline steps can read their app's secrets and start containers on
  nodes, so saving a pipeline is a `write` action, running one is
  `deploy`, and steps targeting other apps need `root` to save.
- The `build` step clones the repository itself rather than reading the
  job workspace, and artifacts are per node. Both are documented limits in
  `docs/pipelines.md`, not hidden.
- Deploys to protected environments are refused until pipelines can
  participate in that approval flow.

## Addendum: repository sync, fork policy, and trigger decisions

Status: Accepted, 2026-09-25. Extends the decision above; nothing in it is
reversed.

**Repository sync.** Definitions can be sourced from the repository's
pipeline directory (`DiscoverDirs`). A push to the tracked branch reads the
directory with a depth-1 in-memory clone (no disk, no checkout) using the
git source's existing token, then upserts definitions. Each synced row
records the commit (`source_sha`) and the SHA-256 of the YAML as synced
(`synced_hash`); a definition is *diverged* when its current YAML hashes
differently. A sync never overwrites a diverged or dashboard-created
definition unless the per-app `repo_is_truth` setting is on, and it never
saves a definition whose steps act on another app, since that requires the
`root` ability at save time and a push must not be a way around it. The
push's own pipelines start only after the sync finishes, off the webhook
request goroutine, so a run uses the pushed commit's files.

**Fork pull requests are untrusted by default.** `on.pull_request.forks` is
`block` (default), `approve`, or `allow`. Fork detection fails closed: a
payload naming no head or base repository counts as a fork. `approve`
creates the run with a run-level hold (`hold_state = pending`) instead of a
per-job approval row, because a job-level gate only exists after jobs are
materialized and a held run must resolve no secret and create no container
at all. A pending hold never occupies a concurrency group.

**Trigger decision log.** Every git event records, per pipeline, whether it
started a run, held one, or was skipped and why, in a bounded per-app table.
Skips that are expected (a pipeline with no trigger of that kind) are not
recorded individually; one row says that no pipeline listens.

**Considered and rejected.** Resuming failed jobs inside a finished run:
approvals are unique per (run, job, step) and would replay their old
decision, the per-run artifact volume is deleted when the run finishes, and
run and job timestamps cannot be cleared. A new run from the start is the
supported path until those are reworked. Reading pipeline files through each
provider's contents API: it would need four clients for one directory
listing, while the git transport already exists for every provider.
