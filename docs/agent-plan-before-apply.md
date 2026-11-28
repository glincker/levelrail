---
title: "Plan before apply"
tag: "Beta"
description: How an AI agent or script asks what an apply would do, reads a machine readable plan, and branches on stable error codes before anything changes.
---

An agent should never apply blind. Every apply path in Levelrail has a read only form that returns what would change, and every refusal carries a stable `code` you can match on instead of parsing text.

## The flow

1. Plan. Send the same files you would apply to a plan call. Nothing is written.
2. Read the plan. Check `summary`, each change `action`, `warnings`, and whether the stack needs approval.
3. Apply with the plan hash. Pass the `hash` from the plan as `expected_plan_hash`. If live state moved, the apply is refused with `plan_stale` and you plan again.
4. On a refusal, branch on `code`, not on the message.

## Where each plan lives

| What you apply | Plan (no side effects) | API | CLI | MCP |
| --- | --- | --- | --- | --- |
| Resource files | plan | `POST /api/v1/apply/plan` | `levelrail-cli apply -f DIR --dry-run -o json` | `plan_apply` |
| A stack | plan | `POST /api/v1/stacks/{name}/plan` | `levelrail-cli stack plan -o json` | `plan_stack` |
| App env (plain, secret, vault) | dry run | `POST /api/v1/apps/{name}/env/apply` with `"dry_run": true` | `levelrail-cli env apply APP --dry-run -o json` | `plan_change` with `set_app_env` |
| One mutating tool call | preview | none | none | `plan_change` |

`--dry-run` with `--exit-code` exits 2 when changes are pending, so a script can gate on it. The dashboard shows the same plan as a preview before the apply button on the stack and resource pages.

## Plan shape

A resource or stack plan is one JSON object:

```json
{
  "changes": [
    {
      "kind": "App",
      "name": "web",
      "action": "update",
      "fields": [{ "path": "spec.image", "before": "web:1.2.2", "after": "web:1.2.3" }],
      "warnings": []
    },
    { "kind": "Database", "name": "db", "action": "noop" },
    { "kind": "App", "name": "old", "action": "delete" }
  ],
  "summary": { "create": 0, "update": 1, "delete": 1, "noop": 1, "error": 0 },
  "hash": "1f0c...",
  "warnings": []
}
```

- `action` is one of `create`, `update`, `delete`, `noop`, `error`. An `error` change has a `code` from the table below and a `reason`.
- Created, changed and removed resources are the changes with `create`, `update` and `delete`.
- Secret values and env values are never in a plan. Sensitive fields show `(hidden)`.
- A stack plan response adds `issues` (validation problems with file and line) and, when the stack needs approval, the apply answers 202 with `pending_approval` instead of changing anything.
- An env dry run returns `dry_run: true`, `changed`, `env_version` and `plan` (key names and the kind of change, no values). Pass `env_version` back as `expected_version` when you apply.

## Error codes

Match on the `code` field. Released codes are never renamed or reused.

Stack and resource apply codes are listed in full, with what to do for each, in [IaC error codes](/iac/errors). The ones an agent meets most:

| Code | HTTP | Meaning | What to do |
| --- | --- | --- | --- |
| `invalid_request` | 400 or 422 | The request is malformed. | Fix the request. |
| `invalid_document` | 422 | A document failed validation. | Read `issues`, fix, plan again. |
| `permission_denied` | 403 | The token lacks the ability. | Use a token with the needed ability. |
| `plan_stale` | 409 | Live state changed after the plan. | Plan again and apply the new hash. |
| `plan_hash_version` | 409 | The hash has an old shape. | Plan again. |
| `stack_locked` | 409 | The stack drift policy refuses manual changes. | Change the manifest, or ask an administrator. |
| `ownership_conflict` | 409 | Another stack or source owns the resource. | Apply from the owner. |
| `prune_guard` | 422 | Prune would delete too much. | Review the plan, confirm with `allow_mass_prune`. |
| `approval_pending` | 409 | A revision awaits approval. | Approve or reject it, or plan with `supersede=true`. |
| `apply_disabled` | 503 | Apply is switched off. | Ask an administrator. |

Env apply codes:

| Code | HTTP | Meaning | What to do |
| --- | --- | --- | --- |
| `env_version_conflict` | 409 | The env changed since `expected_version`. | Re-run the dry run, then apply with the new version. The CLI exits 8. |
| `env_locked_secret` | 409 | A locked secret is in the plan. | Set `overwrite_locked` if replacing it is intended. |
| `env_dual_home` | 409 | A key would be both plain and secret. | Remove one copy. |
| `env_key_is_secret` | 409 | A plain key is already stored as a secret. | Remove the secret first. |

Other plain refusals carry a status based code (`validation_failed`, `forbidden`, `not_found`, `conflict`, `unavailable`).

## Example

```bash
levelrail-cli apply -f infra/ --dry-run --exit-code -o json > plan.json
echo $?   # 0 nothing to do, 2 changes pending, 4 API error
jq -r '.summary, .hash' plan.json

levelrail-cli apply -f infra/ --yes
```

Over MCP, call `plan_apply` (or `plan_stack`) first, show the `summary` to the person, then call `apply_resources` (or `apply_stack` with `confirm: true`) with `expected_plan_hash` set to the plan `hash`. If it answers `plan_stale`, plan again.
