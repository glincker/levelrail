---
description: Deploy a prebuilt image from a GitHub Actions workflow with the bundled composite action, no Go toolchain needed.
---

# Deploying from GitHub Actions

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

The action installs a prebuilt `levelrail-cli` from GitHub Releases (Linux and macOS, amd64 and arm64), verifies its checksum, and by default picks the newest release, pre-releases included while no stable release exists. Pin one with `cli-version: v0.2.0-beta.15`.

To run other CLI commands in a job, install the CLI on its own with the `setup-cli` action. Its input is `version`:

```yaml
- uses: glincker/levelrail/.github/actions/setup-cli@main
- run: levelrail-cli apps status my-app
  env:
    APP_API_URL: ${{ secrets.LEVELRAIL_API_URL }}
    APP_API_TOKEN: ${{ secrets.LEVELRAIL_API_TOKEN }}
```

## Not tied to GitHub Actions

`apps deploy` and `apps wait` are ordinary `levelrail-cli` commands with real exit codes, so the same pattern works in GitLab CI, CircleCI, Jenkins, or a shell script.

## Getting the API token and URL into GitHub

Set two secrets in your GitHub repository or environment:

- `LEVELRAIL_API_URL`: your control plane's base URL, for example the domain fronting it or `https://host:8080`.
- `LEVELRAIL_API_TOKEN`: a token with at least the `deploy` ability. Create it under **Settings > Tokens** or with `levelrail-cli tokens create --name github-actions --abilities deploy`. Storing it as a secret keeps it out of workflow logs.

## Next steps

<CardGroup :cols="2">
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
