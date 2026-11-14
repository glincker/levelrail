---
title: "IaC in CI"
tag: "Experimental"
description: Plan on every pull request and apply on merge, with the CLI or the stack Action. Copy blocks for GitHub Actions and notes for any other CI.
---

<Warning>
Set `APP_EXPERIMENTAL=iac` on the control plane and in the CI job until v1.1.0. See [Experimental features](/experimental-features).
</Warning>

The loop is the same everywhere: a pull request runs a plan and shows the diff, merging runs the apply. Use two tokens: a read-only one for the plan job, a write one for the apply job on the protected branch.

| Job | Command | Token | Exit 2 means |
| --- | --- | --- | --- |
| Plan | `levelrail-cli diff -f infra/` | read only | Changes are pending (not a failure) |
| Apply | `levelrail-cli apply -f infra/ --yes` | write, protected branch only | Not used |

## GitHub Actions with the CLI

Works with plain files. The install step is yours: download the `levelrail-cli` release binary for the runner. The blocks on this page that are CI workflows, not Levelrail documents, are not checked by the examples guard.

```yaml
# .github/workflows/infra.yml
name: infra
on:
  pull_request:
    paths: ["infra/**"]
  push:
    branches: [main]
    paths: ["infra/**"]

jobs:
  plan:
    if: github.event_name == 'pull_request'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install the CLI
        run: ./ci/install-levelrail-cli.sh   # download levelrail-cli from GitHub Releases
      - name: Plan
        env:
          APP_EXPERIMENTAL: iac
          APP_API_URL: ${{ secrets.LEVELRAIL_API_URL }}
          APP_API_TOKEN: ${{ secrets.LEVELRAIL_READ_TOKEN }}
          REGION: ${{ vars.REGION }}
        run: levelrail-cli apply -f infra/ --source infra-repo --allow-env REGION --dry-run --exit-code || [ $? -eq 2 ]

  apply:
    if: github.event_name == 'push'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install the CLI
        run: ./ci/install-levelrail-cli.sh
      - name: Apply
        env:
          APP_EXPERIMENTAL: iac
          APP_API_URL: ${{ secrets.LEVELRAIL_API_URL }}
          APP_API_TOKEN: ${{ secrets.LEVELRAIL_WRITE_TOKEN }}
          PROD_API_KEY: ${{ secrets.PROD_API_KEY }}
          REGION: ${{ vars.REGION }}
        run: levelrail-cli apply -f infra/ --source infra-repo --yes --secret API_KEY=env:PROD_API_KEY --allow-env REGION
```

Pass placeholder values with `--var`, `--values` or `--allow-env`. The plan job needs the same flags as the apply job. The first run with `--source` also plans a `managed-by:<source>` tag change that plain `diff` does not show, so it can exit 2 even when the files match live state; later runs are stable. The runner's own credentials (`GITHUB_TOKEN`, `ACTIONS_*`, cloud keys) are never read unless you name them exactly.

## GitHub Actions with the stack Action

For numbered revisions, rollback and approvals, use the stack Action in `integrations/github-action/stack`. It posts one sticky plan comment per stack on a pull request and applies on merge.

```yaml
# .github/workflows/stack.yml
name: stack
on:
  pull_request:
    paths: ["stacks/**"]
  push:
    branches: [main]
    paths: ["stacks/**"]

jobs:
  plan:
    if: github.event_name == 'pull_request'
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write
    steps:
      - uses: actions/checkout@v4
      - uses: glincker/levelrail/integrations/github-action/stack@main
        env:
          DATABASE_URL: ${{ secrets.STAGING_DATABASE_URL }}
        with:
          stack: staging
          path: stacks/staging
          mode: plan
          api-url: ${{ secrets.LEVELRAIL_API_URL }}
          token: ${{ secrets.LEVELRAIL_PLAN_TOKEN }}
          comment: "true"
          secrets: |
            DATABASE_URL=DATABASE_URL

  apply:
    if: github.event_name == 'push'
    runs-on: ubuntu-latest
    environment: staging
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@v4
      - uses: glincker/levelrail/integrations/github-action/stack@main
        env:
          DATABASE_URL: ${{ secrets.STAGING_DATABASE_URL }}
        with:
          stack: staging
          path: stacks/staging
          mode: apply
          api-url: ${{ secrets.LEVELRAIL_API_URL }}
          token: ${{ secrets.LEVELRAIL_APPLY_TOKEN }}
          secrets: |
            DATABASE_URL=DATABASE_URL
```

Rules that bite:

- Pass the same `secrets` list to the plan step and the apply step. The plan hash covers secret names, so a plan without them has a different hash.
- In apply mode, exit 2 means the revision awaits approval, not a change. The step reports the status `pending_approval`.
- Pin the Action to a release tag or commit SHA instead of `main`, and set its `version` input to the release your server runs. Do not rely on `latest`.
- Environments are the approval gate: give the production apply job `environment: production` with required reviewers, and keep one token secret per environment.
- Approvals need a root token and a different person from the one who applied, so a CI token cannot approve. See [Approvals](/stacks#approvals).

Inputs, outputs and the full recipe are in `integrations/github-action/stack/README.md`.

## No stored secret

Instead of a long-lived token, a job can swap its GitHub identity for a short-lived token bound to one stack. Set `oidc: "true"` on the Action, give the job `id-token: write`, and register what each workflow may do once with `levelrail-cli stack trust add`. An apply trust needs an exact branch or tag and an environment. For tokens created by hand, give one token per job with `stack_verbs` set to `plan` or `apply`. See [Tokens in CI](/stacks#tokens-in-ci).

## Any other CI

GitLab CI, CircleCI, Jenkins and Buildkite run the same two commands from the table above. Ready templates for deploying an image are in [Use any CI](/use-any-ci), and the GitHub deploy Action is described in [GitHub Actions](/github-actions).

```bash
# plan job: exits 0 when nothing changes, 2 when changes are pending
levelrail-cli diff -f infra/ || [ $? -eq 2 ]

# apply job on the protected branch
levelrail-cli apply -f infra/ --yes
```

Next: [How it works](/iac/how-it-works), [IaC errors](/iac/errors).
