---
title: "MCP recipes"
tag: "Beta"
description: Real conversations with an AI assistant connected to the MCP server, showing what you ask, which prompts and tools run, and what comes back.
"og:url": "https://docs.levelrail.com/mcp-recipes"
---

Each recipe shows what you type, what the assistant runs and what comes back. The tools and prompts are described in [MCP tools and write mode](/mcp). Start read only: `levelrail-mcp --print-config claude` prints a config with `--mode read-only`.

## 1. "Why is web crashing?"

You pick the `diagnose_app` prompt with `app=web symptom=crashloop window=2h`, or just ask.

Runs, all read only:

1. `failure_context` for `web`: state `crashloop`, 7 restarts in the window, the failed deploy and the newest log lines.
2. `get_app_timeline` and `get_app_status`: an env key was removed 14 minutes before the first restart.
3. `investigate_app` with `since=2h`: error rate against baseline, likely cause "config change".
4. `list_deploy_attempts`: the last deploy succeeded, so the deploy itself is not the cause.

Comes back: a ranked cause list (missing `DATABASE_URL`), the evidence for each, and one next action: set the key. It offers `set_app_env` only when the session exposes it (not in `read-only` mode), and waits for your yes. Otherwise it points you to `levelrail-cli apps env import`, or the dashboard env editor.

## 2. "Can I safely ship reg/web:2.4.0 now?"

Prompt `plan_deploy` with `app=web image=reg/web:2.4.0`.

Runs: `get_app`, `get_app_pending_changes`, `get_deploy_freeze`, `preflight_app`, `list_deploy_attempts`, then `plan_change` for `deploy_app`, which is a dry run.

Comes back: no freeze, preflight warns about 140 MiB of free memory headroom, the rollback target is deploy `dep_8f2` (image digest pinned), and the exact `deploy_app` call. Nothing has been deployed. You say "go", the assistant calls `deploy_app`, then `wait_for_deploy`.

## 3. "The last deploy failed, why?"

Prompt `explain_failed_deploy` with `app=api`.

Runs: `get_deploy` (latest), `list_deploy_attempts`, `compare_deploys` between the last good deploy and the failed one, and `diagnose_app_failure` if the code is unclear.

Comes back: "The build failed at the install step: the lockfile and `package.json` disagree. The previous good deploy used commit `a1b2c3d`. Retrying will fail the same way." Log lines are quoted as data, and any instruction inside them is ignored.

## 4. "Are my limits sensible?"

Prompt `optimize_resources` with `severity=suggest`.

Runs: `resource_advice` (when the advisor beta is on), then `get_resource_recommendation` and `get_app_metrics` to confirm each finding over a real window.

Comes back: a table of resource, finding, evidence, suggested value and risk. It puts a tight memory limit that risks OOM kills above an oversized idle app. Applying is not done through MCP: it gives you `levelrail-cli advisor apply ID` (preview) and `--yes` to apply.

## 5. "I just installed this, what next?"

Prompt `first_hour_setup`.

Runs: `get_onboarding_status`, `get_system_doctor`, `get_attention`, `list_nodes`, `list_apps`, `backups_health`, `get_upgrade_readiness`.

Comes back: "Docker and ingress are healthy, no apps yet, no backup target. Next three: deploy a first app (`levelrail-cli apps create`), add a domain, add a backup target." The assistant changes nothing.

## Deploy and debug from your agent

Both flows need `--allow-write` for the deploy. Waiting is bounded by `APP_MCP_WAIT_WINDOW_SECONDS` (default 55) and polled every `APP_MCP_WAIT_POLL_SECONDS`; if the deploy is still running the result says `poll_again` and you call `wait_for_deploy` with the returned `deploy_id`.

### Deploy and confirm it is healthy

Prompt: "Deploy nginx:1.27 to the app web and tell me when it is healthy."

Runs: `deploy_app` with `name=web image=nginx:1.27 wait=true`.

Comes back: the app plus a `wait` block with `status: healthy` and the deploy id. One call, no polling loop in the conversation.

### Find out why a deploy failed

Prompt: "Deploy nginx:bad to web. If it fails, tell me why and how to fix it."

Runs: `deploy_app` with `wait=true`, or `wait_for_deploy` for a deploy already running.

Comes back: `status: failed` and a short `diagnosis` with the failure `class`, `cause`, `fix`, `confidence` and a log `excerpt`. The excerpt has secrets masked and terminal control codes removed, so the assistant can quote it. For the full ranked causes call `diagnose_app_failure`.

## Attach a resource instead of calling a tool

In a client that supports resources, attach `levelrail://apps/web/logs?tail=100` and ask "what is the most common error here". The log lines arrive in an untrusted data block, the same as a tool result. Attach `levelrail://docs/rollback` before asking "walk me through rolling back" to ground the answer in the runbook.
