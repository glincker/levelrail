---
description: Define CI/CD pipelines as YAML with triggers, jobs, matrix builds, approvals, and deploys, and run them on your own nodes.
---

# Pipelines

A pipeline is a YAML file that describes what should happen when code changes: run tests in a container, build an image, wait for a person to approve, then deploy. Pipelines sit on top of the build and deploy features that already exist. They call those primitives; they never touch the reconciler, and no AI is involved in running one.

Each pipeline belongs to one app. Open **Pipelines** in an app's sidebar, use the CLI (`levelrail pipelines ...`), or the API under `/api/v1/apps/{name}/pipelines`.

## A first pipeline

```yaml
version: 1
name: release
on:
  push:
    branches: [main]
  manual: {}
stages: [test, build, deploy]
jobs:
  test:
    stage: test
    image: golang:1.23
    steps:
      - run: go test ./...
  build:
    stage: build
    steps:
      - uses: build
        id: image
  ship:
    stage: deploy
    steps:
      - uses: approval
        with:
          message: Deploy to production?
          approvers: deploy
      - uses: deploy
        with:
          strategy: blue-green
      - uses: notify
        with:
          message: release shipped
```

What happens on a push to `main`:

1. `test` runs `go test ./...` in a `golang:1.23` container with your repository checked out at `/workspace`.
2. `build` starts once `test` succeeded (stages run in order). It builds the app's image from the same commit and records it as the job output `image`.
3. `ship` pauses at the approval gate. Anyone holding the `deploy` ability can approve or reject it from the run page, the CLI, or the API.
4. After approval, `deploy` points the app at the image `build` produced and waits until the app reports ready. `notify` then sends the message to the app's deploy notification targets.

## Where pipeline files live

You can edit a pipeline in the dashboard, or keep it in your repository and load it with the CLI:

```
levelrail pipelines validate .pipelines/release.yaml
levelrail pipelines save my-app .pipelines/release.yaml
levelrail pipelines save my-app .            # every file in the repo's pipeline directory
```

A repository's pipeline directory is found by trying `.pipelines/`, then `.ci/pipelines/`, then a directory named after the platform's brand (the CLI asks the control plane for it). `validate` runs locally with no API call, so it works in CI and pre-commit hooks. Saving copies the file into the control plane; a push does not sync files automatically, so run `save` from your own CI when the file changes.

The JSON Schema behind validation is served at `GET /api/v1/pipelines/schema`, and the dashboard editor shows problems by line.

## Triggers

```yaml
on:
  push:
    branches: [main, "release/*"]
  pull_request:
    branches: [main]
  tag:
    patterns: ["v*"]
  schedule: ["0 3 * * *"]
  manual:
    inputs:
      env: { default: staging, options: [staging, production] }
  api: true
```

| Trigger | Starts a run when |
| --- | --- |
| `push` | a push reaches a matching branch (through the app's git webhook) |
| `pull_request` | a pull request is opened or updated against a matching branch |
| `tag` | a tag matching a pattern is pushed |
| `schedule` | a five-field cron expression comes due (missed runs are not replayed) |
| `manual` | someone starts it from the dashboard or CLI, with the declared inputs |
| `api` | an API token starts it |

A trigger key with no value (`push:`) means "on, any branch". A file with no `on` block is manual and API only. Branch and tag filters accept `*` (within one path segment) and `**`.

## Jobs and steps

A job runs in one container, on one node, with a workspace volume mounted at `/workspace`. Steps run in order inside that container.

```yaml
jobs:
  test:
    image: node:22
    node: build-1             # optional, default is the local node
    env: { CI: "true" }
    resources: { memory: 2Gi, cpu: 2 }
    timeout: 20m
    retries: 1                # re-run the whole job once if it fails
    cache:
      - { key: npm, path: /root/.npm }
    steps:
      - run: npm ci
      - run: npm test
        env: { NODE_ENV: test }
        timeout: 10m
        retries: 2
        continue_on_error: true
```

Steps that run in the container: `run` (a shell script), `test` (`with.command`), `artifact-upload`, and `artifact-download`. Steps that act through the control plane, with no container: `build`, `deploy`, `promote`, `rollback`, `notify`, and `approval`. A job made only of those steps starts no container and needs no `image`.

| Step | `with` keys | Effect |
| --- | --- | --- |
| `build` | `type` (`dockerfile` or `railpack`), `context`, `dockerfile`, `image`, `tag` | Builds an image from the run's commit with the same builder deploys use. Sets the output `image`. It does not deploy. |
| `deploy` | `service`, `image`, `strategy`, `wait` | Points a service at an image and waits for it to become ready. `image` defaults to the `image` output of a needed job. `strategy` is `rolling`, `recreate`, or `blue-green`. |
| `promote` | `from`, `to` | Deploys the image currently running in `from` to `to`. |
| `rollback` | `service` | Redeploys the previous known-good image. |
| `notify` | `message`, `app`, `on` (`always`, `success`, `failure`) | Sends a message to the app's deploy notification targets. |
| `approval` | `message`, `approvers` (`deploy`, `write`, `root`), `timeout` | Pauses the job until someone with that ability decides. |
| `artifact-upload` | `name`, `path` | Copies files from the workspace into a run-wide artifact. |
| `artifact-download` | `name`, `path` | Copies an artifact into the workspace. |

Approval steps must come before any step that uses the container, because the container is not started while a job waits. `service`, `from`, `to`, and `app` must be literal names. Steps that target a different app than the pipeline's own can only be saved by a caller with the `root` ability.

Every step accepts `if`, `timeout`, `retries`, `continue_on_error`, `env`, and `secrets`. A step after a failed one is skipped unless its `if` says otherwise (`if: failure()` or `if: always()`).

### Reusable steps

```yaml
templates:
  go-test:
    params: [pkg]
    steps:
      - run: go test ${{ inputs.pkg }}
jobs:
  test:
    image: golang:1.23
    steps:
      - uses: template/go-test
        with: { pkg: ./... }
```

Templates are expanded when the file is parsed. A template cannot use another template.

### Artifacts and caches

Artifacts are stored in a per-run volume and are shared between jobs that run on the same node. A `cache` entry mounts a named volume that survives between runs of the same app, keyed by `key`.

## Dependencies, stages, and matrix

`needs` builds a directed graph of jobs. `stages` is shorthand: a job in a later stage waits for every job in the earlier ones.

```yaml
jobs:
  test:
    image: golang:1.23
    matrix:
      go: ["1.22", "1.23"]
      os: [alpine, bookworm]
      exclude:
        - { go: "1.22", os: bookworm }
    steps:
      - run: echo testing on go ${{ matrix.go }} / ${{ matrix.os }}
  publish:
    needs: [test]
    steps:
      - uses: build
```

A matrix job becomes one job per combination (named like `test[go=1.23,os=alpine]`), up to 256. `needs: [test]` waits for all of them. Dependency cycles are rejected when the file is validated.

## Conditions and expressions

`if` accepts `==`, `!=`, `&&`, `||`, `!`, parentheses, string literals in single quotes, and the functions `success()`, `failure()`, `cancelled()`, `always()`, `contains()`, `startsWith()`, and `endsWith()`. The same values are available inside `${{ }}` anywhere in a step's `run`, `env`, or `with`.

| Value | Meaning |
| --- | --- |
| `app`, `trigger`, `actor`, `ref`, `branch`, `tag`, `sha` | Run metadata |
| `run.number`, `run.id`, `job` | The run and the current job |
| `inputs.<name>` | Manual inputs |
| `matrix.<name>` | The current matrix combination |
| `env.<name>` | Pipeline and job `env` values |
| `needs.<job>.result` | `success`, `failure`, `skipped`, or `cancelled` |
| `needs.<job>.outputs.<name>` | Outputs of a needed job |
| `secrets.<NAME>` | An app secret (see below) |

A step can publish an output by printing a line of the form `::set-output name=key::value`.

```yaml
- uses: deploy
  if: branch == 'main' && needs.test.result == 'success'
```

## Secrets

Secrets come from the app's existing secrets (`PUT /api/v1/apps/{name}/secrets/{key}`, or **Environment** in the dashboard). Use `${{ secrets.NAME }}` in a step, or list names under a step's `secrets:` to export them as environment variables. Values are sent to the container through standard input, not command arguments, and are masked (`***`) in stored logs. A missing secret fails the step before it runs.

Pipeline steps can read every secret of their own app, so treat write access to a pipeline as write access to those secrets.

## Concurrency, timeouts, and retries

```yaml
concurrency:
  group: deploy-${{ branch }}
  cancel_in_progress: true
timeout: 2h
```

Runs that share a group run one at a time, oldest first. With `cancel_in_progress`, a newer run cancels older active runs in its group instead of waiting behind them. `timeout` on the pipeline, a job, or a step bounds each. Retries re-run only what failed: a step retry re-runs that step, a job retry re-runs the job's steps from the start.

## Running and watching

```
levelrail pipelines run my-app release --ref refs/heads/main --input env=staging --follow
levelrail pipelines runs my-app
levelrail pipelines runs my-app <run-id>
levelrail pipelines logs my-app <run-id> --job "test[go=1.23]" --follow
levelrail pipelines approve my-app <run-id> --comment "ship it"
levelrail pipelines cancel my-app <run-id>
```

The run page in the dashboard draws the jobs as columns by dependency depth, streams the selected job's output live, and offers **Approve**, **Reject**, **Cancel**, and **Re-run**. Log lines are capped per job, and old runs are pruned to the newest 100 per pipeline.

The MCP server exposes `list_pipeline_runs` and `explain_pipeline_run`. Both are read-only: the second reports the failed job and step, its exit code, the tail of its output, and any approval it is waiting on.

## Permissions

| Action | Ability |
| --- | --- |
| Read pipelines, runs, and logs | `read` |
| Create, edit, delete a pipeline | `write` |
| Start, cancel, or re-run a run | `deploy` |
| Decide an approval gate | `deploy`, plus the gate's own `approvers` ability |

## How runs are executed

The engine stores every run, job, and step in the database with a status and a reason, and re-derives what to do from that state on each pass. If the control plane restarts, jobs recorded as running are picked up again: leftover containers are removed, finished steps are skipped, and the interrupted step runs again. Write steps so that running them twice is safe.

Environment variables tune the engine: `APP_PIPELINE_MAX_PARALLEL_JOBS` (default 4), `APP_PIPELINE_MAX_LOG_LINES` (50000 per job), `APP_PIPELINE_STEP_TIMEOUT` (30m), `APP_PIPELINE_JOB_TIMEOUT` (1h), `APP_PIPELINE_APPROVAL_TIMEOUT` (72h), `APP_PIPELINE_KEEP_RUNS` (100), `APP_PIPELINE_GIT_IMAGE` (the image used for checkout, default `alpine/git`), and `APP_PIPELINE_TICK_INTERVAL`.

## Current limits

- Checkout and `build` need a git source connected to the app. Without one, container jobs run with an empty workspace.
- The `build` step clones the repository itself instead of reading the job workspace, so files created by earlier steps are not part of the build context.
- Artifacts are per node. Jobs that exchange artifacts must run on the same node.
- Workspace and artifact volumes are removed automatically on the local node; on remote nodes they are left for the volume cleanup.
- `deploy`, `promote`, and `rollback` refuse services in a protected environment.
- Pipeline files are not synced from the repository automatically. Run `levelrail pipelines save` when they change.
