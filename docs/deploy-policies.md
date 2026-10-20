---
title: "Deploy policies"
description: Guardrails checked before every deploy at organization, project and environment scope, with warn or enforce modes and an audited override.
"og:url": "https://docs.levelrail.com/deploy-policies"
---

A deploy policy is a set of rules an app must satisfy before it deploys. Rules are set at organization, project or environment scope. For each rule the most specific scope wins, so an environment can relax or tighten what the organization set.

Each rule is **enforce** (blocks the deploy), **warn** (reported, never blocks) or **off**.

## Rules

| Rule | Value | Violation code |
| --- | --- | --- |
| `allowed_registries` | registry hosts | `policy_registry_not_allowed` |
| `require_readiness_probe` | none | `policy_readiness_probe_required` |
| `max_memory_mb` | MB ceiling | `policy_memory_ceiling_exceeded` |
| `max_cpu` | core ceiling | `policy_cpu_ceiling_exceeded` |
| `forbid_privileged` | none | `policy_privileged_forbidden` |
| `forbid_host_mounts` | allowed path prefixes (optional) | `policy_host_mount_forbidden` |
| `required_labels` | label keys | `policy_label_required` |
| `max_replicas` | replica ceiling | `policy_replicas_exceeded` |

A ceiling treats a missing limit as exceeding it, so an app with no memory limit fails a memory ceiling.

## Where it is enforced

- Preflight reports every violation as a check with its `code` and a `fix`: enforce is a fail, warn is a warning.
- A deploy that breaks an enforced rule is refused with HTTP 422, code `policy_violation` and a `violations` list, each with a stable `code`, a `message` and a `next_action`.
- Rollbacks skip policy so restoring a known state is never blocked.

## Override

Pass `override_policy: true` and `override_policy_reason` on the deploy request. The caller needs the root ability. The override is recorded as a `policy.override` app event and noted on the deploy attempt.

## Dashboard

Settings, Deploy policies. Pick a scope, set each rule, then use the dry run to see which current apps would be blocked before you save.

## CLI

```
levelrail-cli policy list
levelrail-cli policy set org --rule allowed_registries=enforce:ghcr.io --rule max_memory_mb=warn:2048
levelrail-cli policy set project:PROJECT_ID --clear
levelrail-cli policy check --fail
```

`policy check` is a dry run. With `--rule` and `--scope` it previews unsaved rules. `--fail` exits non-zero when any app would be blocked.

## API

- `GET /api/v1/deploy-policies`
- `PUT /api/v1/deploy-policies/{scope}/{id}` with `{"rules": {...}}`, scope `org` (id `org`), `project` or `environment`
- `DELETE /api/v1/deploy-policies/{scope}/{id}`
- `POST /api/v1/deploy-policies/check`

There is no MCP tool for policies yet: the tool list is at its token budget, so it returns once heavier tool descriptions are trimmed.

## Not covered yet

Builds from git check policy at their image deploy, not before the build starts. The privileged rule applies to preflight requests that declare it, since apps have no privileged setting today.
