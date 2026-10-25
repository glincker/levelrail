# Levelrail Stack Action

A composite action that runs the GitOps loop for a Levelrail stack: plan on pull requests, apply on merge. It installs `levelrail-cli` and wraps `levelrail-cli stack plan` and `levelrail-cli stack apply`.

Stacks are gated by `APP_EXPERIMENTAL=iac`. The action sets it for the CLI; the server needs it too.

## Inputs

| Input | Required | Default | Notes |
| --- | --- | --- | --- |
| `stack` | yes | | Stack name. |
| `path` | yes | | Manifest file or directory. `kind: Stack` documents are stripped by the CLI and supply the default project and prune. |
| `mode` | no | `plan` | `plan` or `apply`. |
| `api-url` | yes | | Control plane base URL. |
| `token` | yes | | API token with write access. Store it as a secret. |
| `project` | no | | Needed when the stack does not exist yet. |
| `prune` | no | `false` | Delete resources the files no longer declare. |
| `vars` | no | | Newline list of `NAME=VALUE` placeholders. |
| `secrets` | no | | Newline list of `NAME=ENVVAR`. Set each `ENVVAR` in the `env:` of the action step. Values are masked and never printed. |
| `expected-plan-hash` | no | | Apply only. Refuse (409) when the plan no longer has this hash. |
| `version` | no | `latest` | `levelrail-cli` release tag. Pin it to the version your server runs. |
| `comment` | no | `false` | `true` posts one sticky PR comment per stack (plan mode). |
| `github-token` | no | `github.token` | Used only for the comment. |

## Outputs

| Output | Mode | Meaning |
| --- | --- | --- |
| `has-changes` | plan | `true` when the plan holds changes. |
| `plan-hash` | plan | Hash to pass as `expected-plan-hash`. |
| `revision` | apply | Revision number written. |
| `status` | apply | Revision status. The step fails when it is `partial` or `failed`. |

## Behavior

Plan mode runs `stack plan --json --exit-code`. Exit 0 and 2 (changes pending) both succeed the step; anything else fails it. A markdown summary (adds, changes, removals, plan hash, warnings) goes to the job summary and, with `comment: 'true'`, to a single PR comment. The comment carries a hidden marker with the stack name (`<!-- levelrail-stack:NAME -->`), so reruns update it and several stacks in one PR get one comment each.

Apply mode runs `stack apply --yes --json`. Git repository, ref, commit, CI run URL and the idempotency key are derived from the `GITHUB_*` environment, so a rerun of the same job returns the stored revision instead of applying twice.

The token is masked with `::add-mask::`, passed through the environment (not argv), and secret values are masked before the CLI runs.

## The GitOps recipe

See [example-workflow.yml](example-workflow.yml).

1. **Pull request runs plan.** Trigger on `pull_request` with a `paths` filter on your stack directory. Give the job `pull-requests: write` (only needed for the comment) and `contents: read`. Reviewers read the sticky comment.
2. **Merge to main runs apply.** Trigger on `push` to `main` with the same filter. Apply jobs need only `contents: read`.
3. **Environments are the approval gate.** Give each apply job `environment: staging` or `environment: production` and add required reviewers to the production environment in the repository settings. The job waits for approval before the action runs. Keep a separate stack (and ideally a separate token secret per environment) for each.
4. **Pin the plan you approved.** If plan and apply share a workflow run, pass `${{ steps.plan.outputs.plan-hash }}` as `expected-plan-hash`. The apply then fails with a conflict if the live state moved after review.
5. **Rollback.** Apply an earlier revision as a new one:

   ```sh
   levelrail-cli stack rollback <name> <n> --yes
   ```

   Rollback restores configuration only. Secret values and database contents are not rolled back. Use `levelrail-cli stack pin <name> <n>` to refuse applies while you investigate.

## Permissions and tokens

Use the minimum: `contents: read` at the workflow level, and `pull-requests: write` only on the job that posts the comment. The apply job does not need it.

Stack-scoped OIDC tokens are not available yet, so the action cannot swap the job identity for a token. Create a write token in Levelrail and store it as a repository or environment secret (`LEVELRAIL_TOKEN`). Prefer environment secrets so the production token is only released to the approved production job. Rotate it if it leaks.

Fork pull requests get a read-only `GITHUB_TOKEN` and no secrets, so plan and comment do not run for them. The comment step only warns if posting fails.
