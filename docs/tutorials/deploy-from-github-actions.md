---
description: Build an image in GitHub Actions and deploy it to your own server with Levelrail on every push, failing the pipeline if the rollout fails. About 15 minutes.
---

# Deploy to your own server from GitHub Actions

By the end of this tutorial every push to `main` builds your image, deploys it to your own server, and turns the workflow red if the rollout fails. Levelrail's deploy action does the last step; you keep your existing build step.

## Before you start

- A running Levelrail instance with an existing app to deploy to ([Deploy a Docker app](deploy-a-docker-app.md) creates one).
- A container registry the server can pull from. This example uses GitHub's `ghcr.io`.
- Permission to add secrets to your GitHub repository.

## 1. Create a deploy-only token

CI should not hold your admin credentials. Mint a token that can read and deploy, and nothing else:

```bash
levelrail-cli tokens create --name github-actions --preset deployer --expires-in-days 90
```

The command asks for your admin username and password, because a token can only be created from a live session. The token prints once and cannot be shown again. `deployer` means the `read` and `deploy` abilities. See [Identity and access](../identity-and-access.md) for the full list.

## 2. Store the URL and token as secrets

```bash
gh secret set LEVELRAIL_API_URL --body "https://console.example.com"
gh secret set LEVELRAIL_API_TOKEN
```

The second command prompts for the value, so the token never lands in your shell history.

## 3. Let the server pull your image

If the image is public, skip this. For a private `ghcr.io` package, give Levelrail pull credentials once. See [Registry credentials](../backups-and-storage.md#registry-credentials), and test them with `levelrail-cli registry-credentials test <id>` before the first deploy.

## 4. Add the workflow

Create `.github/workflows/deploy.yml`:

```yaml
name: Deploy

on:
  push:
    branches: [main]

permissions:
  contents: read
  packages: write

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - uses: docker/build-push-action@v6
        with:
          push: true
          tags: ghcr.io/${{ github.repository }}:${{ github.sha }}

      - name: Deploy to Levelrail
        uses: glincker/levelrail/.github/actions/deploy@main
        with:
          api-url: ${{ secrets.LEVELRAIL_API_URL }}
          api-token: ${{ secrets.LEVELRAIL_API_TOKEN }}
          app: my-app
          image: ghcr.io/${{ github.repository }}:${{ github.sha }}
          cli-version: v0.2.0-beta.15
```

The action installs the CLI (with its checksum verified), runs `apps deploy`, then runs `apps wait` and fails the job if the rollout fails or times out. Without `cli-version` it picks the newest release. Pin it, as above, if you want builds that never change underneath you.

Tag the image with the commit SHA, not `latest`. A SHA tag is immutable, so the deploy history tells you exactly which commit each release is, and rollback has a precise target.

## 5. Push and watch

Push to `main`. The workflow's last step prints:

```text
waiting for "my-app" to converge... (succeeded)
"my-app" rolled out
```

and the deploy appears in `levelrail-cli apps deploys list my-app` and on the dashboard's Deployments page.

## Useful inputs

| Input | Default | What it does |
| --- | --- | --- |
| `wait` | `true` | Set `false` to return as soon as the deploy is accepted. |
| `timeout` | `5m` | How long to wait for the rollout to converge. |
| `confirm` | `false` | Required to deploy into a protected environment. |

The action's `status` output is `succeeded`, `failed`, or `skipped`.

## Not on GitHub?

The action is a thin wrapper over two commands, so any CI works. Install the CLI with the [install script](../installing.md#installing-just-the-cli), export `APP_API_URL` and `APP_API_TOKEN`, then run `levelrail-cli apps deploy my-app --image "$IMAGE"` and `levelrail-cli apps wait my-app`. Both exit non-zero on failure.

## Where to go next

- [Deploying from GitHub Actions](../github-actions.md): the full action reference.
- [Zero-downtime deploys with health checks](zero-downtime-deploys-with-health-checks.md): make a bad release fail the job before it replaces a good one.
- [Pipelines](../pipelines.md): run builds and tests on your own nodes instead of GitHub's.
