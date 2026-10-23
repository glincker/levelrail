---
title: "Deploying from GitHub Actions"
description: Deploy a prebuilt image from a GitHub Actions workflow with the bundled composite action, no Go toolchain needed.
"og:url": "https://docs.levelrail.com/github-actions"
---

`.github/actions/deploy` is a composite GitHub Action that deploys an already-built image to a Levelrail app and waits for the rollout to converge before the job finishes. It wraps two CLI commands, `apps deploy` and `apps wait`, so anything it does you can also script in a shell step.

The action's own [README](https://github.com/glincker/levelrail/blob/main/.github/actions/deploy/README.md) has a complete example workflow. The short version:

```yaml
- name: Deploy to Levelrail
  uses: glincker/levelrail/.github/actions/deploy@main
  with:
    api-url: ${{ secrets.LEVELRAIL_API_URL }}
    api-token: ${{ secrets.LEVELRAIL_API_TOKEN }}
    app: my-app
    image: ghcr.io/${{ github.repository }}:${{ github.sha }}
```

The action does not build your image. Build and push it in an earlier step, then pass the reference as `image`.

| Input | Required | Default | Meaning |
| --- | --- | --- | --- |
| `api-url` | yes | none | Control plane base URL |
| `api-token` | yes | none | API token with at least the `deploy` ability |
| `app` | yes | none | Name of the existing app |
| `image` | yes | none | Image reference to deploy |
| `confirm` | no | `false` | Set `true` to deploy into a protected environment (same as `apps deploy --confirm`) |
| `wait` | no | `true` | Set `false` to return as soon as the deploy is accepted |
| `timeout` | no | `5m` | How long to wait for the rollout to converge |
| `cli-version` | no | `latest` | `levelrail-cli` release tag to install |

The `status` output is `succeeded`, `failed`, or `skipped` (when `wait` is `false`).

The action installs a prebuilt `levelrail-cli` from GitHub Releases (Linux and macOS, amd64 and arm64), verifies its checksum, and by default picks the newest release. Pin one with `cli-version: vX.Y.Z`.

To run other CLI commands in a job, install the CLI on its own with the `setup-cli` action. Its input is `version`:

```yaml
- uses: glincker/levelrail/.github/actions/setup-cli@main
- run: levelrail-cli apps status my-app
  env:
    APP_API_URL: ${{ secrets.LEVELRAIL_API_URL }}
    APP_API_TOKEN: ${{ secrets.LEVELRAIL_API_TOKEN }}
```

`LEVELRAIL_TOKEN`, `LEVELRAIL_API_URL` and `LEVELRAIL_PROFILE` work too and win over the `APP_*` names; Actions sets `CI=true`, so the CLI never prompts (see CI mode in the CLI reference).

## Reporting back to the pull request

`integrations/github-action` is the same deploy-and-wait flow plus a report back to GitHub: a deployment status on the commit (the Deployments panel on the PR) and a check run named after the environment. Use it instead of `.github/actions/deploy` when you want the result visible on the PR.

```yaml
permissions:
  contents: read
  deployments: write
  checks: write

steps:
  - uses: glincker/levelrail/integrations/github-action@main
    with:
      api-url: ${{ secrets.LEVELRAIL_API_URL }}
      api-token: ${{ secrets.LEVELRAIL_API_TOKEN }}
      app: my-app
      image: ghcr.io/${{ github.repository }}:${{ github.sha }}
      environment-url: https://my-app.example.com
```

The `status` output is `succeeded`, `failed`, `timeout`, or `accepted` (when `wait` is `false`, the check run ends neutral). A failed report call never fails the deploy: the deploy result stays as is, the job prints a warning, and `report-warnings` counts the failed calls.

Triggers are ordinary GitHub triggers on your workflow: `push`, `workflow_dispatch`, or `repository_dispatch` (for example `curl -X POST -H "Authorization: Bearer $TOKEN" https://api.github.com/repos/OWNER/REPO/dispatches -d '{"event_type":"levelrail-deploy"}'`). Teams that deploy several apps can call the reusable workflow `.github/workflows/levelrail-deploy.yml` with `uses:` and pass `app` and `image`.

### Per-app settings

Each app stores the GitHub environment name used for deployments (default `production`) and whether deployments and check runs are posted. Change them in the dashboard on the app's Deploys page (GitHub Actions card), or with the CLI:

```bash
levelrail-cli apps github-actions get my-app
levelrail-cli apps github-actions set my-app --environment staging --no-checks
```

The same settings are available at `GET/PUT /api/v1/apps/{name}/github-actions`. `apps github-actions start` and `finish` are the commands the action runs; they read `GITHUB_TOKEN`, `GITHUB_REPOSITORY` and `GITHUB_SHA`, so you can call them from any CI that has a GitHub token.

## Deploy without a stored secret (OIDC)

Instead of a long-lived token, trust a repository and let the job swap its GitHub identity for a 15 minute token that can only read and deploy that one app.

1. Set the dashboard address (Settings), it is the audience the identity token is issued for.
2. On the app's Deploys page, open **Deploy from CI**, trust `owner/name` with a ref such as `refs/heads/main` (globs like `refs/tags/v*` work, an environment is optional), and copy the generated workflow. CLI: `apps github-actions trust add <app> --repo owner/name` and `apps github-actions workflow <app>`.
3. The workflow needs `permissions: id-token: write` and sets `oidc: "true"` on the action. It also sets `rollback-on-failure: "true"`, which redeploys the previously running image when the deploy fails or times out.

The generated workflow honors the app's deploy path filters and skips commits containing `[skip ci]`, `[ci skip]`, `[skip deploy]` or `[no deploy]`. Git push webhooks skip the same markers.

Token exchange is `POST /api/v1/ci/oidc-exchange`; `apps github-actions token <app>` calls it from a job.

## Not tied to GitHub Actions

`apps deploy` and `apps wait` are ordinary `levelrail-cli` commands with real exit codes, so the same pattern works in GitLab CI, CircleCI, Jenkins, or a shell script.

## Getting the API token and URL into GitHub

Set two secrets in your GitHub repository or environment:

- `LEVELRAIL_API_URL`: your control plane's base URL, for example the domain fronting it or `https://host:8080`.
- `LEVELRAIL_API_TOKEN`: a token with at least the `deploy` ability. Create it under **Settings > Tokens** or with `levelrail-cli tokens create --name github-actions --abilities deploy`. Storing it as a secret keeps it out of workflow logs.

## Next steps

<CardGroup cols={2}>
<Card title="CLI reference" href="/cli-reference">

`apps deploy` and `apps wait` in full.

</Card>
<Card title="Identity and access" href="/identity-and-access">

Token creation and abilities.

</Card>
<Card title="Git integrations" href="/git-integrations">

Webhook-based deploys as an alternative.

</Card>
<Card title="Deploying apps" href="/deploying-apps">

The full app lifecycle.

</Card>
</CardGroup>
