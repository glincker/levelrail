# Levelrail Deploy and Report

Deploys an already-built image to a Levelrail app, waits for the rollout to
converge, then reports back to GitHub: a deployment status (shown on the PR's
Deployments panel) and a check run on the commit.

```yaml
- uses: glincker/levelrail/integrations/github-action@main
  with:
    api-url: ${{ secrets.LEVELRAIL_API_URL }}
    api-token: ${{ secrets.LEVELRAIL_API_TOKEN }}
    app: my-app
    image: ghcr.io/${{ github.repository }}:${{ github.sha }}
```

The job needs `permissions: { contents: read, deployments: write, checks: write }`.
See `example-workflow.yml` for push, `workflow_dispatch` and `repository_dispatch` triggers.

## Inputs

| Input | Default | Meaning |
| --- | --- | --- |
| `api-url`, `api-token`, `app`, `image` | required | Same as the plain deploy action |
| `confirm` | `false` | Deploy into a protected environment |
| `wait` | `true` | Wait for convergence; `false` reports the check as neutral |
| `timeout` | `5m` | Rollout wait limit |
| `github-token` | `github.token` | Posts the reports; empty turns reporting off |
| `environment-url` | empty | App URL shown on the GitHub deployment |
| `sha` | PR head or pushed commit | Commit the reports attach to |

Outputs: `status` (`succeeded`, `failed`, `timeout`, `accepted`) and `report-warnings`.

A failed report call never changes the deploy result: the deploy stays green, the failure is
printed as a workflow warning and counted in `report-warnings`.

Which GitHub environment name is used, and whether deployments or check runs are posted,
is set per app: Deploys page in the dashboard, or `levelrail-cli apps github-actions set <app>`.
