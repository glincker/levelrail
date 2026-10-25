---
title: "Stacks and GitOps"
tag: "Experimental"
description: Keep apps, databases and domains in git as a stack, review each change as a plan in the pull request, apply on merge, and roll back to any earlier revision.
"og:url": "https://docs.levelrail.com/stacks"
---

A stack is a named set of resources, such as apps, databases, domains and alert rules, that you describe in files in a git repository. A pull request shows what would change, merging applies it, and every apply is saved as a numbered revision you can compare, pin or roll back to.

Stacks sit on top of [platform as code](/platform-as-code). They use the same YAML documents and the same apply engine, so anything you can declare there can go into a stack.

<Warning>
**Experimental**

Stacks are off by default. Set `APP_EXPERIMENTAL=iac` on the control plane, and in the shell where you run `levelrail-cli` or the MCP server, to turn them on. See [Experimental features](/experimental-features).
</Warning>

## What a stack is

A stack has a name, the project it belongs to, and a few settings that a root token changes with `levelrail-cli stack settings`:

- **Prune**: whether the stack may delete resources it owns. Off unless you turn it on with `--allow-prune`.
- **Drift policy**: `warn` (default) or `lock`. See [Drift](#drift).
- **Require approval**: holds applies until another administrator approves them. See [Approvals](#approvals).
- **Pin**: when set, applies are refused except a rollback to the pinned revision. See [Pin](#pin).

The control plane keeps a ledger of the resources each stack owns, across all kinds. There is no state file to keep in the repository.

## Repository layout

Keep one folder per stack:

```text
infra/
  stacks/
    shop-staging/
      stack.yaml      the Stack document: project, prune
      apps.yaml
      domains.yaml
    shop-prod/
      stack.yaml
      apps.yaml
      domains.yaml
```

`stack.yaml` holds the `Stack` document. The CLI strips it before it sends the other files, and uses its `project` and `prune` fields as defaults. The other files hold the ordinary platform documents (`App`, `Domain`, and so on).

Promote a change to production with a pull request that edits `infra/stacks/shop-prod/`, usually the image tag. Review it like any other code change.

## The loop

1. **Pull request**: CI runs `stack plan` for each changed stack. The plan is posted as a comment with counts and field paths. Environment values are never shown.
2. **Merge**: CI runs `stack apply` for the stack. The apply creates the next revision and records the git commit and the CI run URL on it.
3. **History**: each apply is a numbered revision. You can list them, look at the documents and result of one, and compare two.
4. **Rollback**: go back to an earlier revision if the new one is wrong.

Stacks use the CLI from CI, so any CI system works. The plan posted to a pull request is the same plan the CLI prints on your machine.

## CLI

```bash
levelrail-cli stack plan shop-prod -f infra/stacks/shop-prod
levelrail-cli stack apply shop-prod -f infra/stacks/shop-prod --yes
levelrail-cli stack history shop-prod
levelrail-cli stack diff shop-prod 12 --against 11
levelrail-cli stack rollback shop-prod 11 --yes
levelrail-cli stack pin shop-prod 12
levelrail-cli stack unpin shop-prod
```

`plan`, `show` and `diff` change nothing. `apply` writes a new revision. `plan --exit-code` exits 2 when changes are pending. Run `levelrail-cli stack --help` for the full list, including `show-revision`, `reject`, `unlock` and `delete`.

## Ownership and prune

A stack owns the resources it created, and the resources it adopted. Prune deletes only owned resources that the files no longer declare. A resource that exists outside every stack makes the apply fail unless the apply sets `adopt`.

Databases and projects are never pruned, because they hold data or other stacks' members.

Prune is also guarded against mass deletes. An apply that would delete more than `APP_STACK_PRUNE_MAX_COUNT` resources (default 10), or more than `APP_STACK_PRUNE_MAX_PERCENT` percent of the stack's resources (default 50), is refused. Review the plan, then send `allow_mass_prune` with the apply request to go ahead.

## Revisions

A revision stores the plan, the rendered documents (after `${{ env.NAME }}` values are filled in), the actor, the token used, the git repository, ref and commit, the CI run URL, the time, and the result of each item. Revision numbers start at 1 and increase by one for each apply to that stack.

Revisions never contain secret values. Only secret names are stored.

If an apply stops partway, the revision is marked `partial` and keeps the result of each item. The next apply picks up where it stopped. There are no transactions across resources, so a partial apply can leave the stack between two states until the next apply succeeds.

## Rollback

`stack rollback NAME N` takes the documents of revision N, plans them against live state, and applies them as a new revision. History is never rewritten, so the rollback appears as the newest revision.

Rollback restores the configuration only:

- **Secret values are not rolled back.** Set a secret again if the old value is needed.
- **Database contents are not rolled back.** A restored database definition does not undo data written since.

The plan for a rollback prints a warning about both.

## Approvals

Turn approval on with `levelrail-cli stack settings NAME --require-approval` (root only). An apply to a stack with approval required is stored as a `pending_approval` revision. Nothing is applied, and the API answers 202.

Another administrator approves or rejects it. The person who requested the apply can never approve their own revision.

```bash
levelrail-cli stack approve shop-prod 13 --secret DB_PASSWORD=env:SHOP_DB_PASSWORD
levelrail-cli stack reject shop-prod 13 --reason "wrong image tag"
```

Secret values are not stored with a pending revision, so the approver supplies them again. A pending request expires after `APP_STACK_APPROVAL_TTL` (default 24h). Approving is refused with a conflict if the live state changed since the plan.

## Pin

A pin freezes a stack. While it is set, an apply is refused unless it is a rollback to the pinned revision. Use it during an incident: pin the stack, change things by hand if you must, then fix the files in git and run `stack unpin`. The next plan shows what differs from git.

## Drift

Drift is a difference between live state and the files, usually from a hand edit in the dashboard or the CLI.

A drift checker plans each stack's latest applied revision against live state every `APP_STACK_DRIFT_INTERVAL` (default 15m). Set it to `0` to turn the checker off. The checker only reads. It never applies a plan or changes a resource.

- **warn** (default): a drifted stack is recorded and raises an attention item.
- **lock**: recorded the same way, and manual changes to stack-owned apps, databases, domains, projects and environments get `409 stack_locked` naming the stack. Start, stop, restart, exec, builds, backups, cache purge and read-only checks still work. To change a locked resource, change the manifest and apply, or ask an admin. A root user can override one request with the header `X-Levelrail-Override-Stack-Lock: <stack>`; it is audited as `stack.lock_override` and CI or stack-scoped tokens cannot use it. If the ledger lookup fails, the request is allowed and a warning is logged. Forgetting or destroying a stack unlocks its resources.

## Tokens in CI

Create one token per job through the tokens API, setting the stack and the verbs it needs:

- A **plan** token, with `stack` set to the stack and `stack_verbs` set to `plan`.
- An **apply** token, with `stack` set to the stack and `stack_verbs` set to `apply`.

A stack-scoped token can reach only that stack's endpoints. During an apply it can also reach resources in the stack's ledger and project. Manifests that use `--secret`, create new projects, or change environment protection need a human token instead. A stack token also cannot create or update a `Pipeline`, because pipeline steps run commands with the app's secrets; apply those with a human token. An apply token can still change an app's image, so treat it like deploy access to the stack's apps. Approvals need a root token, so a CI token cannot approve.

Store each token as a CI secret, and give the apply token only to the protected branch and environment that deploys.

## Secrets

Manifests name secrets, they never contain values. Refer to a secret by name, for example `{ secretRef: DB_PASSWORD }`, and pass its value at apply time from the CI secret store:

```bash
levelrail-cli stack apply shop-prod -f infra/stacks/shop-prod --yes --secret DB_PASSWORD=env:SHOP_DB_PASSWORD
```

The value travels in the request over TLS. It is not written to a revision, a plan or a log. Non-secret settings can be passed with `--var` or allowed from the environment with `--allow-env`, as described in [platform as code](/platform-as-code#providing-values-safely).

## GitHub Action

The stack action in `integrations/github-action/stack` runs the loop for you. On a pull request it runs a plan and posts one comment per stack. On merge it runs an apply.

- Pass the same `secrets` list to the plan step and the apply step. The plan hash covers secret names, so a plan without them has a different hash.
- In apply mode, exit code 2 means the revision awaits approval. It does not mean a change. The step reports the status `pending_approval`.

## Status

Stacks are experimental in this release, behind `APP_EXPERIMENTAL=iac`. The file format, the revision fields and the CLI flags can still change. Pin your CLI to the same version as the control plane, and read the changelog before you upgrade either.
