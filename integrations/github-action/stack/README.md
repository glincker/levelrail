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
| `token` | no | | API token with write access. Store it as a secret. Not needed with `oidc: true`. |
| `oidc` | no | `false` | `true` swaps the job's GitHub identity for a short-lived token bound to this stack. Needs `id-token: write`. See below. |
| `oidc-audience` | no | `api-url` | Audience of the identity token. Must equal the dashboard URL in Levelrail Settings. |
| `project` | no | | Needed when the stack does not exist yet. |
| `prune` | no | `false` | Delete resources the files no longer declare. |
| `vars` | no | | Newline list of `NAME=VALUE` placeholders. |
| `secrets` | no | | Newline list of `NAME=ENVVAR`. Set each `ENVVAR` in the `env:` of the action step. Values are masked and never printed. **Pass the same list to plan and apply.** See below. |
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
| `status` | apply | Revision status: `applied`, `pending_approval`, `partial` or `failed`. The step fails when it is `partial` or `failed`. |
| `expires_at` | apply | When a `pending_approval` revision expires. Empty otherwise. |

## Secrets and the plan hash

**Pass the same `secrets` list to the plan step and the apply step.** The plan hash covers the names of the secrets a stack sets (`secrets.NAME` changes), so a plan without them has a different hash. Apply then refuses with a 409 when `expected-plan-hash` is set. The hash covers names, not values, and the values are masked in both steps. Each `ENVVAR` must be set in the `env:` of the plan step too.

## Behavior

Plan mode runs `stack plan --json --exit-code`. Exit 0 means no changes. **Exit 2 means changes exist** (`has-changes` is `true`). Both succeed the step; anything else fails it. A markdown summary (adds, changes, removals, plan hash, warnings) goes to the job summary and, with `comment: 'true'`, to a single PR comment. The comment carries a hidden marker with the stack name (`<!-- levelrail-stack:NAME -->`), so reruns update it and several stacks in one PR get one comment each.

Apply mode runs `stack apply --yes --json`. Git repository, ref, commit, CI run URL and the idempotency key are derived from the `GITHUB_*` environment, so a rerun of the same job returns the stored revision instead of applying twice.

**In apply mode, exit 2 means awaiting approval, not a change.** When the stack has `require_approval`, the CLI stores a `pending_approval` revision and exits 2 (HTTP 202). The step succeeds with `status=pending_approval`, the `revision` and `expires_at` outputs, and a job summary line. Nothing is applied yet. The approval is a separate step, see the recipe below.

The token is masked with `::add-mask::`, passed through the environment (not argv), and secret values are masked before the CLI runs.

## The GitOps recipe

See [example-workflow.yml](example-workflow.yml).

1. **Pull request runs plan.** Trigger on `pull_request` with a `paths` filter on your stack directory. Give the job `pull-requests: write` (only needed for the comment) and `contents: read`. Reviewers read the sticky comment.
2. **Merge to main runs apply.** Trigger on `push` to `main` with the same filter. Apply jobs need only `contents: read`.
3. **Environments are the approval gate.** Give each apply job `environment: staging` or `environment: production` and add required reviewers to the production environment in the repository settings. The job waits for approval before the action runs. Keep a separate stack (and ideally a separate token secret per environment) for each.
4. **Pin the plan you approved.** If plan and apply share a workflow run, pass `${{ steps.plan.outputs.plan-hash }}` as `expected-plan-hash`. The apply then fails with a conflict if the live state moved after review. Remember the secrets rule above: the plan step needs the same `secrets`.
5. **Approve a pending revision.** When a stack has `require_approval`, apply stops at `pending_approval` and the approval is a separate job. The approver must be a different principal from the one that requested the apply, and must hold a root token (a CI token cannot approve). Approvers re-supply the secret values, because secrets are never stored with a pending revision:

   ```sh
   levelrail-cli stack approve <name> <n> --secret NAME=env:VAR
   ```

   Pending requests expire after `APP_STACK_APPROVAL_TTL` (default 24h). If the live state changes before approval, approve returns 409 and nothing is applied. Re-run the apply job to request a fresh approval.
6. **Rollback.** Apply an earlier revision as a new one:

   ```sh
   levelrail-cli stack rollback <name> <n> --yes
   ```

   Rollback restores configuration only. Secret values and database contents are not rolled back. Use `levelrail-cli stack pin <name> <n>` to refuse applies while you investigate.

## Permissions and tokens

Use the minimum: `contents: read` at the workflow level, and `pull-requests: write` only on the job that posts the comment. The apply job does not need it.

Either use OIDC (below) or create a write token in Levelrail and store it as a repository or environment secret (`LEVELRAIL_TOKEN`). Prefer environment secrets so the production token is only released to the approved production job. Rotate it if it leaks.

### OIDC, no stored secret

With `oidc: true` the action asks GitHub for the job's identity token (audience: your dashboard URL), posts it to `POST /api/v1/ci/stack-oidc-exchange` and gets back a token that lives `APP_STACK_OIDC_TTL` (default 15 minutes), is bound to this one stack and holds only the verbs the matching trust allows. The token is masked with `::add-mask::` and is never written to the environment or the logs. The job needs `id-token: write`.

A root user registers what each workflow may do, once per stack:

```
levelrail-cli stack trust add shop-prod --repo acme/shop --ref 'refs/pull/*/merge' --verbs plan
levelrail-cli stack trust add shop-prod --repo acme/shop --ref refs/heads/main --environment production --verbs plan,apply
```

Rules the server enforces, and fails closed on (HTTP 403, same answer for every miss):

- `repository` is an exact `owner/name`. `ref_pattern` is an exact ref or has at most one `*` per `/` segment (`refs/pull/*/merge`, `refs/tags/v*`). No regex.
- An `apply` trust needs an exact branch or tag ref (no `*`, never a pull request) and an `environment`. Plan trusts may be broader.
- A token never gets more than the trust's verbs, and never destroy, unlock, settings, approve or pin.
- If two trusts match one workflow identity, the exchange is refused. Make the patterns disjoint.

```yaml
jobs:
  apply:
    runs-on: ubuntu-latest
    environment: production
    permissions:
      contents: read
      id-token: write
    steps:
      - uses: actions/checkout@v7
      - uses: glincker/levelrail/integrations/github-action/stack@main
        with:
          stack: shop-prod
          path: stacks/shop
          mode: apply
          api-url: https://levelrail.example.com
          oidc: 'true'
```

Approvals need a root token, stored as a production environment secret (`LEVELRAIL_ROOT_TOKEN`) and released only to the approve job. Do not reuse the CI write token for approvals: it cannot approve.

Fork pull requests get a read-only `GITHUB_TOKEN` and no secrets, so plan and comment do not run for them. The comment step only warns if posting fails.
