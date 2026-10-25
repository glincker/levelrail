---
title: "Stacks and GitOps"
tag: "Experimental"
description: Keep apps, databases and domains in git as a stack, review each change as a plan in the pull request, apply on merge, and roll back to any earlier revision.
"og:url": "https://docs.levelrail.com/stacks"
---

A stack is a named set of resources, such as apps, databases, domains and alert rules, that you describe in files in a git repository. A pull request shows what would change, merging applies it, and every apply is saved as a numbered revision you can compare, pin or roll back to.

Stacks sit on top of [platform as code](/platform-as-code). The same YAML documents and the same apply engine are used, so anything you can declare there can go into a stack.

<Warning>
**Experimental**

Stacks are off by default. Set `APP_EXPERIMENTAL=iac` on the control plane, and in the shell where you run `levelrail-cli` or the MCP server, to turn them on. The commands below land in this release. See [Experimental features](/experimental-features).
</Warning>

## What a stack is

A stack has a name, a project it belongs to, an optional environment, and a few server-side settings:

- **Prune**: whether a stack may delete resources it created. Off unless an admin turns it on for the stack.
- **Drift policy**: `warn` or `lock`, described below.
- **Pinned revision**: when set, applies to the stack are refused until the pin is removed.

The control plane stores the stack, its revisions and the list of resources it owns. There is no state file to keep in the repository and no state file to lose.

## Repository layout

Keep one folder per stack under `infra/stacks/`:

```text
infra/
  stacks/
    shop-staging/
      stack.yaml      the Stack document: name, project, environment, drift
      apps.yaml
      domains.yaml
    shop-prod/
      stack.yaml
      apps.yaml
      domains.yaml
```

`stack.yaml` holds the `Stack` document. The other files hold the ordinary platform documents (`App`, `Domain`, and so on). A folder without a `Stack` document is applied the old way, with `--source`, and behaves as it did before.

Promote a change to production with a pull request that edits `infra/stacks/shop-prod/`, usually the image tag. Review it like any other code change.

## The loop

1. **Pull request**: the CI job runs `stack plan` for each changed stack. The plan is posted as a comment with counts and field paths. Environment values are never shown.
2. **Merge**: the CI job runs `stack apply` for the stack. The apply creates the next revision and records the git commit and the CI run URL on it.
3. **History**: each apply is a numbered revision. You can list them, look at the documents and results of one, and compare two.
4. **Rollback**: go back to an earlier revision if the new one is wrong.

Stacks use the CLI from CI, so any CI system works. The plan posted to a pull request is the same plan the CLI prints on your machine.

## CLI

```bash
levelrail-cli stack plan infra/stacks/shop-prod
levelrail-cli stack apply infra/stacks/shop-prod --yes
levelrail-cli stack history shop-prod
levelrail-cli stack diff shop-prod 11 12
levelrail-cli stack rollback shop-prod 11
levelrail-cli stack pin shop-prod 12
```

`plan` and `diff` change nothing. `apply` writes a new revision. `rollback` and `pin` are described below. Run `levelrail-cli stack pin --help` to see how to remove a pin.

## Revisions

A revision stores the plan, the rendered documents (after `${{ env.NAME }}` values are filled in), the actor, the token used, the git repository, ref and commit, the CI run URL, the time, and the result of each item. Revision numbers start at 1 and increase by one for each apply to that stack.

Revisions never contain secret values. Only secret names are stored.

If an apply stops partway, the revision is marked `partial` and keeps the result of each item. The next apply picks up where it stopped, because apply only does what is still missing. There are no transactions across resources, so a partial apply can leave the stack between two states until the next apply succeeds.

## Rollback

`stack rollback NAME N` takes the documents of revision N, plans them against live state, and applies them as a new revision. History is never rewritten, so the rollback appears as the newest revision with a link to the one it restores.

Rollback restores the configuration only:

- **Secret values are not rolled back.** Set a secret again if the old value is needed.
- **Database contents are not rolled back.** A restored database definition does not undo data written since.

The plan for a rollback prints a warning about both.

## Pin

A pin freezes a stack. While it is set, an apply is refused unless it is a rollback to the pinned revision. Use it during an incident: pin the stack, change things by hand if you must, then fix the files in git and remove the pin. The next plan shows what differs from git.

## Drift

Drift is a difference between live state and the files, usually from a hand edit in the dashboard or the CLI.

- **warn** (default): the stack is marked drifted and the next plan shows the changed fields. Edits in the dashboard still work, with a warning that the next apply will change them back.
- **lock**: writes to fields owned by the stack, from outside a stack apply, are refused with a conflict error. Change the files and apply, or unlock.

Lists and maps such as `env`, `labels`, `tags` and `domains` are additive. Entries the files do not mention are shown as `keep` and are not removed. Operational actions such as restart, logs and exec never count as drift.

## Tokens in CI

Use two tokens, one per job, as [platform as code](/platform-as-code#git-workflow) does:

- The **plan** job uses a read token. It can list, plan, diff and read history, and it cannot change anything.
- The **apply** job uses a write token. It can apply, roll back and pin.

Store both as CI secrets. Stack-scoped CI tokens are not available yet, so the apply job's write token is broader than the stack it applies. Give it only to the protected branch and environment that deploys.

## Secrets

Manifests name secrets, they never contain values. Refer to a secret by name, for example `{ secretRef: DB_PASSWORD }`, and pass its value at apply time from the CI secret store:

```bash
levelrail-cli stack apply infra/stacks/shop-prod --yes --secret DB_PASSWORD=env:SHOP_DB_PASSWORD
```

The value travels in the request over TLS. It is not written to a revision, a plan or a log. Non-secret settings can be passed with `--var` or allowed from the environment with `--allow-env`, as described in [platform as code](/platform-as-code#providing-values-safely).

## Status

Stacks are experimental in this release, behind `APP_EXPERIMENTAL=iac`. The file format, the revision fields and the CLI flags can still change. Pin your CLI to the same version as the control plane, and read the changelog before you upgrade either.
