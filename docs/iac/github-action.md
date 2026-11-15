---
title: "GitHub Action for IaC"
tag: "Experimental"
description: Plan on every pull request with one sticky comment, apply on the protected branch with the reviewed plan hash, and read the exit code as an output.
---

<Warning>
Set `APP_EXPERIMENTAL=iac` on the control plane until v1.1.0. See [Experimental features](/experimental-features).
</Warning>

The stack Action lives in `integrations/github-action/stack`. It installs `levelrail-cli`, runs `stack plan` on pull requests and `stack apply` on the protected branch. A full workflow is in `integrations/github-action/stack/example-workflow.yml`, and the shorter copy block is on [IaC in CI](/iac/ci).

## What it does

| Step | When | Result |
| --- | --- | --- |
| Install | always | Downloads the pinned `levelrail-cli` release and verifies it against the release `checksums.txt` |
| Plan | `mode: plan` | Runs `stack plan --json --exit-code`, writes the job summary, posts or updates one comment per stack |
| Apply | `mode: apply` | Runs `stack apply --yes`, refusing when the plan no longer matches `expected-plan-hash` |

Set `version` to a release tag such as `v1.2.0`. Do not rely on `latest`: the Action warns when you do.

## Sign in

Pick one:

- **GitHub OIDC (no stored secret).** Set `oidc: "true"`, give the job `id-token: write`, and set `oidc-audience` to the dashboard URL when it differs from `api-url`. Register what each workflow may do with `levelrail-cli stack trust add`. See [Tokens in CI](/stacks#tokens-in-ci).
- **Token secret.** Pass `token` from a repository secret for the plan job and an environment secret for the apply job.

Tokens and identity tokens are masked and never printed.

## Exit codes as outputs

The `exit-code` output carries what the CLI returned. The step fails only on an error.

| Exit code | Plan mode | Apply mode | Step |
| --- | --- | --- | --- |
| `0` | No changes | Applied | Succeeds |
| `2` | Changes are pending (`has-changes` is `true`) | Revision awaits approval (`status` is `pending_approval`) | Succeeds |
| `1` or other | Error | Error, or revision ended `partial` or `failed` | Fails |

Other outputs: `has-changes`, `plan-hash`, `revision`, `status`, `expires_at`.

## The sticky comment

With `comment: "true"` and `pull-requests: write`, the plan step posts one comment per stack and updates it on every push. It only edits a comment posted by `github-actions[bot]` that carries its hidden marker, so a planted marker is ignored. The comment is capped by `comment-max-chars` (default 60000): a longer plan is cut with a pointer to the job summary, which always holds the full text. Secret values never appear in the comment, only secret names inside the plan.

## Apply the reviewed plan

Run plan and apply with the same `secrets` list, then pass the plan hash to apply so a change in live state after review is refused:

```yaml
jobs:
  deploy:
    runs-on: ubuntu-latest
    environment: production
    permissions:
      contents: read
      id-token: write
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
      - id: plan
        uses: glincker/levelrail/integrations/github-action/stack@v1.2.0
        with:
          stack: prod
          path: stacks/prod
          mode: plan
          api-url: ${{ vars.LEVELRAIL_API_URL }}
          oidc: "true"
          version: v1.2.0
      - if: steps.plan.outputs.has-changes == 'true'
        uses: glincker/levelrail/integrations/github-action/stack@v1.2.0
        with:
          stack: prod
          path: stacks/prod
          mode: apply
          api-url: ${{ vars.LEVELRAIL_API_URL }}
          oidc: "true"
          version: v1.2.0
          expected-plan-hash: ${{ steps.plan.outputs.plan-hash }}
```

Use `environment: production` with required reviewers as the approval gate. For separate plan and apply workflows, the inputs and recipes are in `integrations/github-action/stack/README.md`.

Next: [IaC in CI](/iac/ci), [Stacks and GitOps](/stacks).
