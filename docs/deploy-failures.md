---
description: The structured failure object every failed or blocked deploy carries, and what each failure code means.
---

# Deploy failures

A deploy that fails, is held, or does not become healthy carries one structured `failure` object. The same object comes back from the API, the CLI and the MCP tools, so a person and an agent read the same explanation.

```json
{
  "code": "dockerfile_error",
  "cause": "The Dockerfile is invalid or a file it references is missing from the build context.",
  "failing_step": "building",
  "log_excerpt": "failed to read dockerfile: open Dockerfile: no such file or directory",
  "suggested_fix": "Check build.path and build.baseDirectory in the app spec ...",
  "docs_url": "/deploy-failures#dockerfile_error",
  "retryable": false,
  "deploy_id": "da_8f2c",
  "app": "web",
  "at": "2026-09-26T10:02:11Z"
}
```

`log_excerpt` is the last relevant lines, capped and passed through the same secret redaction as the rest of the platform. Set `APP_FAILURE_EXCERPT_LINES` (default 20) and `APP_FAILURE_EXCERPT_BYTES` (default 4000) to change the caps. `retryable` is true when running the same deploy again can succeed without changing anything (a transient registry error, a freeze window that ends). Unrecognised failures get code `unknown` with the excerpt so nothing is hidden.

## Where to read it

- API: `GET /api/v1/apps/{name}/deploys/{deployId}` (`deployId` may be `latest`), and the additive `failure` field on `GET /api/v1/apps/{name}/deploy-attempts`, `GET /api/v1/deploys/failed`, `GET /api/v1/deployments` (next to the existing `error_summary`) and `GET /api/v1/apps/{name}/diagnose`.
- CLI: `levelrail-cli apps deploys show <name> [deploy-id]` (add `--json` for the raw object). `apps diagnose` prints it too.
- MCP: `get_deploy` (a deploy id or `latest`), and the `failure` field of `diagnose_app_failure`, `list_deployments` and `list_failed_deploys`.

The first stage is a table of known classes. When none matches, the runtime cause analysis behind `apps diagnose` is consulted before falling back to `unknown`.

## Waiting for a deploy

`GET /api/v1/apps/{name}/deploys/{deployId}` also returns an `outcome`: `in_progress`, `healthy`, `failed`, `canceled`, `superseded` or `blocked` (held by a freeze window or an approval). It is keyed on that one deploy: a container that reports serving this deploy's image is healthy, and rollout failures only count for the app's newest deploy, so a later restart or deploy cannot change an earlier deploy's result.

- CLI: `levelrail-cli apps deploys wait <name> [deploy-id] [--timeout 10m]` blocks and prints the final status plus the failure. Exit codes: 0 healthy, 7 failed, canceled, superseded or blocked, 6 timeout. Without a deploy id the newest deploy is resolved once and followed by id. The older `apps wait` keeps its flags and exit codes (5 failed) and now also pins the deploy it started on.
- MCP: `wait_for_deploy` blocks at most `APP_MCP_WAIT_WINDOW_SECONDS` (default 55) per call, because MCP call timeouts are shorter than deploys. When the deploy is still running it returns `{"status":"in_progress","poll_again":true,"deploy_id":"..."}`: call again with that `deploy_id` until `poll_again` is false. `APP_MCP_WAIT_POLL_SECONDS` (default 2) sets the poll period.

## Failure codes

### dockerfile_error
The Dockerfile is invalid or a file it references is missing. Check `build.path` and `build.baseDirectory`, fix the syntax, and make sure every `COPY` source is committed and not excluded by `.dockerignore`.

### dependency_install_failed
A dependency install step in the build failed. Read the failing command in the excerpt, fix the lockfile or version pin, and confirm the package registry is reachable from the build node.

### build_out_of_memory
The build was killed for memory. Give the build node more memory, lower build parallelism, or build a smaller target.

### build_timeout
The build passed its time limit. Retryable. Enable the remote build cache or split slow stages.

### image_pull_failed
The image could not be pulled: wrong name or tag, or the registry denied access. Check the name and add or refresh the registry credential.

### port_not_listening
The container started but nothing listens on the configured port. Bind `0.0.0.0` on the spec port (the `PORT` env var is injected) or fix the spec port.

### health_check_failed
The container never passed its readiness check. Confirm `health.readiness.path` answers quickly, or raise `health.readyTimeout` for a slow start.

### container_crashed
The container exited or is restarting. Read the excerpt for what happens right after start (bad command, missing file, config error), then fix and redeploy or roll back.

### oom_killed
The running container was killed for exceeding its memory limit. Raise the memory limit or reduce usage.

### missing_env
A required environment variable or secret has no value. Set it on the app and redeploy.

### registry_push_failed
Pushing the built image failed. Retryable. Check the registry credential, repository permissions and reachability.

### disk_full
The host ran out of disk. Prune unused images and build cache, then retry.

### docker_unreachable
The control plane could not reach the Docker daemon on the target node. Retryable once the daemon is back.

### freeze_window_hold
The deploy is held by a freeze window. Retryable: it is released when the window ends, or redeploy with an explicit override and a reason.

### approval_pending
The deploy waits for an approval. An approver approves or rejects it under deploy approvals.

### scan_gate_blocked
The supply chain scan gate blocked the release. Fix the vulnerable packages or record a reasoned override.

### rollback_target_gone
The rollback image was garbage collected. Redeploy from source and raise the retained rollback image count.

### unknown
No known class matched. The excerpt and the full deploy log (`apps deploys logs <name> <deploy-id>`) are the source of truth.
